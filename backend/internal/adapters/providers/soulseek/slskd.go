package soulseek

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/audioformat"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const Name = "soulseek"

const (
	// slskd passes this value to Soulseek.NET, where the timeout is measured in
	// milliseconds even though the slskd API describes it as seconds.
	searchTimeoutMilliseconds = 5_000
	searchMaxLimit            = 200
	searchMaxWait             = 8 * time.Second
	searchCleanupDelay        = time.Minute
	pollInterval              = 250 * time.Millisecond
	maxResponse               = 8 << 20
)

var artworkExtensions = map[string]struct{}{
	"gif": {}, "jpeg": {}, "jpg": {}, "png": {},
}

type Client struct {
	endpoint   *url.URL
	apiKey     string
	httpClient *http.Client
	downloads  *downloadCoordinator

	completedMu      sync.RWMutex
	completedHandler func(CompletedFile)
	cacheChanged     func()
	searchCleanupCtx context.Context
	searchCleanupEnd context.CancelFunc
	searchCleanupMu  sync.Mutex
	searchClosed     bool
	searchCleanupWG  sync.WaitGroup
	cleanupDelay     time.Duration

	artworkMu   sync.Mutex
	artworkJobs map[string]*artworkDownload
	artworkWG   sync.WaitGroup
}

type CompletedFile struct {
	TrackID string
	Path    string
}

func NewSlskd(endpoint, apiKey string, timeout time.Duration) (*Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse slskd URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("slskd URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("slskd URL must include a host")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	return &Client{
		endpoint: parsed, apiKey: strings.TrimSpace(apiKey), httpClient: &http.Client{Timeout: timeout},
		searchCleanupCtx: cleanupCtx, searchCleanupEnd: cleanupCancel, cleanupDelay: searchCleanupDelay,
	}, nil
}

func (c *Client) Name() string { return Name }

func (c *Client) SetCompletedHandler(handler func(CompletedFile)) {
	c.completedMu.Lock()
	c.completedHandler = handler
	c.completedMu.Unlock()
}

func (c *Client) SetCacheChangedHandler(handler func()) {
	c.completedMu.Lock()
	c.cacheChanged = handler
	c.completedMu.Unlock()
}

func (c *Client) notifyCompleted(file CompletedFile) {
	c.completedMu.RLock()
	handler := c.completedHandler
	c.completedMu.RUnlock()
	if handler != nil {
		handler(file)
	}
	c.notifyCacheChanged()
}

func (c *Client) notifyCacheChanged() {
	c.completedMu.RLock()
	cacheChanged := c.cacheChanged
	c.completedMu.RUnlock()
	if cacheChanged != nil {
		cacheChanged()
	}
}

func (c *Client) SetMediaDirectories(downloadsDir, incompleteDir string) error {
	coordinator, err := newDownloadCoordinator(c, downloadsDir, incompleteDir)
	if err != nil {
		return err
	}
	if c.downloads != nil {
		c.downloads.close()
	}
	c.downloads = coordinator
	return nil
}

func (c *Client) Close() error {
	c.searchCleanupMu.Lock()
	c.searchClosed = true
	c.searchCleanupEnd()
	c.searchCleanupMu.Unlock()
	c.searchCleanupWG.Wait()
	if c.downloads != nil {
		c.downloads.close()
	}
	c.artworkWG.Wait()
	return nil
}

func (c *Client) ProviderStatus(ctx context.Context) domain.ProviderStatus {
	status := domain.ProviderStatus{Provider: Name, Configured: true}
	request, err := c.request(ctx, http.MethodGet, "/api/v0/session", nil)
	if err != nil {
		status.Message = "invalid slskd request"
		return status
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Peerphonic")
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		status.Message = "slskd is unreachable"
		return status
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	status.Reachable = true
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		status.Authenticated = true
		status.Message = "slskd API is ready"
	case http.StatusUnauthorized, http.StatusForbidden:
		status.Message = "slskd rejected the API key"
	default:
		status.Message = fmt.Sprintf("slskd returned HTTP %d", response.StatusCode)
	}
	return status
}

func (c *Client) Search(ctx context.Context, query domain.SearchQuery) ([]domain.TrackSource, error) {
	query.Text = strings.TrimSpace(query.Text)
	if len([]rune(query.Text)) < 3 || len([]rune(query.Text)) > 200 {
		return nil, errors.New("search query must contain between 3 and 200 characters")
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > searchMaxLimit {
		query.Limit = searchMaxLimit
	}

	results, err := c.searchOnce(ctx, query.Text, query.Limit)
	if err != nil || len(results) > 0 {
		return results, err
	}
	fallback := searchFallbackTerm(query.Text)
	if fallback == "" {
		return results, nil
	}
	fallbackLimit := min(max(query.Limit*4, 50), searchMaxLimit)
	fallbackResults, err := c.searchOnce(ctx, fallback, fallbackLimit)
	if err != nil {
		return nil, err
	}
	return filterSearchResults(fallbackResults, query.Text, query.Limit), nil
}

func (c *Client) searchOnce(ctx context.Context, text string, limit int) ([]domain.TrackSource, error) {
	payload := struct {
		SearchText    string `json:"searchText"`
		SearchTimeout int    `json:"searchTimeout"`
		ResponseLimit int    `json:"responseLimit"`
		FileLimit     int    `json:"fileLimit"`
	}{
		SearchText: text, SearchTimeout: searchTimeoutMilliseconds,
		ResponseLimit: limit, FileLimit: limit * 4,
	}
	var search slskdSearch
	if err := c.doJSON(ctx, http.MethodPost, "/api/v0/searches", payload, &search); err != nil {
		return nil, fmt.Errorf("start slskd search: %w", err)
	}
	if search.ID == "" {
		return nil, errors.New("start slskd search: response did not include an id")
	}
	defer c.scheduleDeleteSearch(search.ID)

	deadline := time.NewTimer(searchMaxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if search.IsComplete {
			return mapSearchResults(search.Responses, limit), nil
		}
		if err := c.doJSON(ctx, http.MethodGet,
			"/api/v0/searches/"+url.PathEscape(search.ID)+"?includeResponses=true", nil, &search,
		); err != nil {
			return nil, fmt.Errorf("poll slskd search: %w", err)
		}
		if search.IsComplete {
			return mapSearchResults(search.Responses, limit), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return mapSearchResults(search.Responses, limit), nil
		case <-ticker.C:
		}
	}
}

func searchFallbackTerm(query string) string {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) < 2 {
		return ""
	}
	longest := terms[0]
	for _, term := range terms[1:] {
		if len([]rune(term)) > len([]rune(longest)) {
			longest = term
		}
	}
	return longest
}

func filterSearchResults(results []domain.TrackSource, query string, limit int) []domain.TrackSource {
	terms := strings.Fields(strings.ToLower(query))
	filtered := make([]domain.TrackSource, 0, min(limit, len(results)))
	for _, result := range results {
		haystack := strings.ToLower(strings.Join([]string{
			result.Track.Artist, result.Track.Album, result.Track.Title, result.DisplayPath,
		}, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, result)
			if len(filtered) == limit {
				break
			}
		}
	}
	return filtered
}

type slskdSearch struct {
	ID         string                `json:"id"`
	IsComplete bool                  `json:"isComplete"`
	Responses  []slskdSearchResponse `json:"responses"`
}

type slskdSearchResponse struct {
	Username          string      `json:"username"`
	Files             []slskdFile `json:"files"`
	LockedFiles       []slskdFile `json:"lockedFiles"`
	HasFreeUploadSlot bool        `json:"hasFreeUploadSlot"`
	QueueLength       int64       `json:"queueLength"`
	UploadSpeed       int64       `json:"uploadSpeed"`
}

type slskdFile struct {
	Filename  string `json:"filename"`
	Extension string `json:"extension"`
	Size      int64  `json:"size"`
	Length    *int   `json:"length"`
	BitRate   *int   `json:"bitRate"`
}

type slskdDirectory struct {
	Name  string      `json:"name"`
	Files []slskdFile `json:"files"`
}

func mapSearchResults(responses []slskdSearchResponse, limit int) []domain.TrackSource {
	results := make([]domain.TrackSource, 0, limit)
	discoveredAt := time.Now().UTC()
	for _, response := range responses {
		files := make([]struct {
			file     slskdFile
			requires bool
		}, 0, len(response.Files)+len(response.LockedFiles))
		for _, file := range response.Files {
			files = append(files, struct {
				file     slskdFile
				requires bool
			}{file: file})
		}
		for _, file := range response.LockedFiles {
			files = append(files, struct {
				file     slskdFile
				requires bool
			}{file: file, requires: true})
		}
		for _, candidate := range files {
			file := candidate.file
			extension := fileExtension(file)
			format, ok := audioformat.ByExtension(extension)
			if !ok || strings.TrimSpace(file.Filename) == "" {
				continue
			}
			displayPath := strings.ReplaceAll(file.Filename, "\\", "/")
			title := strings.TrimSuffix(filepath.Base(displayPath), filepath.Ext(displayPath))
			artist, album := provisionalArtistAlbum(displayPath)
			refPayload, _ := json.Marshal(remoteFileRef{
				Peer: response.Username, Path: file.Filename, Size: file.Size,
			})
			track := domain.Track{
				ID:    domain.StableID(Name, response.Username, file.Filename, strconv.FormatInt(file.Size, 10)),
				Title: title, Artist: artist, ArtistID: domain.StableID("artist", artist),
				Album: album, AlbumID: domain.StableID("album", artist, album),
				AlbumArtist: artist, AlbumArtistID: domain.StableID("artist", artist),
				Size: file.Size, Suffix: format.Suffix, ContentType: format.ContentType,
			}
			if file.Length != nil && *file.Length > 0 {
				track.Duration = time.Duration(*file.Length) * time.Second
			}
			if file.BitRate != nil && *file.BitRate > 0 {
				track.BitRate = *file.BitRate
			}
			results = append(results, domain.TrackSource{
				Track:       track,
				Ref:         domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(refPayload)},
				DisplayPath: displayPath,
				Availability: domain.SourceAvailability{
					Peer: response.Username, UploadSpeed: response.UploadSpeed,
					QueueLength: response.QueueLength, FreeUploadSlot: response.HasFreeUploadSlot,
					RequiresApproval: candidate.requires,
				},
				DiscoveredAt: discoveredAt,
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func provisionalArtistAlbum(displayPath string) (string, string) {
	parts := strings.FieldsFunc(displayPath, func(r rune) bool { return r == '/' || r == '\\' })
	artist, album := "Unknown Artist", "Unknown Album"
	if len(parts) >= 2 && strings.TrimSpace(parts[len(parts)-2]) != "" {
		album = strings.TrimSpace(parts[len(parts)-2])
	}
	if len(parts) >= 3 && strings.TrimSpace(parts[len(parts)-3]) != "" {
		artist = strings.TrimSpace(parts[len(parts)-3])
	}
	return artist, album
}

func (c *Client) BrowseCollection(
	ctx context.Context, anchor domain.TrackSource,
) (domain.SourceCollection, error) {
	if anchor.Ref.Provider != Name {
		return domain.SourceCollection{}, fmt.Errorf("unexpected provider %q", anchor.Ref.Provider)
	}
	if anchor.Availability.RequiresApproval {
		return domain.SourceCollection{}, fmt.Errorf("%w: remote directory requires peer approval", ports.ErrSourceUnavailable)
	}
	remote, err := decodeRemoteFileRef(anchor.Ref.Key)
	if err != nil {
		return domain.SourceCollection{}, err
	}
	directory := remoteDirectory(remote.Path)
	if directory == "" {
		return domain.SourceCollection{}, errors.New("Soulseek result does not belong to a directory")
	}
	var directories []slskdDirectory
	if err := c.doJSON(ctx, http.MethodPost,
		"/api/v0/users/"+url.PathEscape(remote.Peer)+"/directory",
		struct {
			Directory string `json:"directory"`
		}{Directory: directory}, &directories,
	); err != nil {
		return domain.SourceCollection{}, fmt.Errorf("browse Soulseek directory: %w", err)
	}
	files := filesForDirectory(directories, directory)
	if len(files) == 0 {
		return domain.SourceCollection{}, errors.New("Soulseek directory is empty or no longer available")
	}
	artist, album := provisionalArtistAlbum(remote.Path)
	coverID := remoteCoverID(remote.Peer, files)
	discoveredAt := time.Now().UTC()
	tracks := make([]domain.TrackSource, 0, len(files))
	for _, file := range files {
		extension := fileExtension(file)
		format, ok := audioformat.ByExtension(extension)
		if !ok || strings.TrimSpace(file.Filename) == "" || file.Size <= 0 {
			continue
		}
		refPayload, _ := json.Marshal(remoteFileRef{Peer: remote.Peer, Path: file.Filename, Size: file.Size})
		displayPath := strings.ReplaceAll(file.Filename, "\\", "/")
		track := domain.Track{
			ID:     domain.StableID(Name, remote.Peer, file.Filename, strconv.FormatInt(file.Size, 10)),
			Title:  strings.TrimSuffix(filepath.Base(displayPath), filepath.Ext(displayPath)),
			Artist: artist, ArtistID: domain.StableID("artist", artist),
			Album: album, AlbumID: domain.StableID("album", artist, album),
			AlbumArtist: artist, AlbumArtistID: domain.StableID("artist", artist),
			Size: file.Size, Suffix: format.Suffix, ContentType: format.ContentType, CoverArtID: coverID,
		}
		if file.Length != nil && *file.Length > 0 {
			track.Duration = time.Duration(*file.Length) * time.Second
		}
		if file.BitRate != nil && *file.BitRate > 0 {
			track.BitRate = *file.BitRate
		}
		track.TrackNumber = provisionalTrackNumber(track.Title)
		tracks = append(tracks, domain.TrackSource{
			Track:       track,
			Ref:         domain.SourceRef{Provider: Name, Key: base64.RawURLEncoding.EncodeToString(refPayload)},
			DisplayPath: displayPath, Availability: anchor.Availability, DiscoveredAt: discoveredAt,
		})
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		left, right := tracks[i].Track, tracks[j].Track
		if left.TrackNumber > 0 && right.TrackNumber > 0 && left.TrackNumber != right.TrackNumber {
			return left.TrackNumber < right.TrackNumber
		}
		return strings.ToLower(tracks[i].DisplayPath) < strings.ToLower(tracks[j].DisplayPath)
	})
	return domain.SourceCollection{Name: album, Artist: artist, CoverArtID: coverID, Tracks: tracks}, nil
}

func fileExtension(file slskdFile) string {
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(file.Extension), "."))
	if extension == "" {
		extension = strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")
	}
	return extension
}

func remoteDirectory(path string) string {
	index := strings.LastIndexAny(strings.TrimSpace(path), `/\\`)
	if index <= 0 {
		return ""
	}
	return path[:index]
}

func filesForDirectory(directories []slskdDirectory, requested string) []slskdFile {
	normalize := func(value string) string {
		return strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
	}
	for _, directory := range directories {
		if normalize(directory.Name) == normalize(requested) {
			return directory.Files
		}
	}
	if len(directories) == 1 {
		return directories[0].Files
	}
	return nil
}

func provisionalTrackNumber(title string) int {
	digits := strings.TrimSpace(title)
	end := 0
	for end < len(digits) && end < 3 && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	number, _ := strconv.Atoi(digits[:end])
	return number
}

func remoteCoverID(peer string, files []slskdFile) string {
	bestScore := 100
	var best slskdFile
	for _, file := range files {
		extension := fileExtension(file)
		if _, ok := artworkExtensions[extension]; !ok || file.Size <= 0 || file.Size > maxRemoteArtworkSize {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.ReplaceAll(file.Filename, "\\", "/")), filepath.Ext(file.Filename)))
		score := 10
		for index, preferred := range []string{"cover", "folder", "front", "album"} {
			if base == preferred {
				score = index
				break
			}
		}
		if score < bestScore {
			bestScore, best = score, file
		}
	}
	if bestScore == 100 {
		return ""
	}
	payload, _ := json.Marshal(remoteFileRef{Peer: peer, Path: best.Filename, Size: best.Size})
	return remoteArtworkPrefix + base64.RawURLEncoding.EncodeToString(payload)
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint := *c.endpoint
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	if index := strings.Index(endpoint.Path, "?"); index >= 0 {
		endpoint.RawQuery = endpoint.Path[index+1:]
		endpoint.Path = endpoint.Path[:index]
	}
	endpoint.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Peerphonic")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	return request, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrSourceUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("%w: slskd rejected the API key", ports.ErrSourceUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		responseErr := fmt.Errorf(
			"slskd returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)),
		)
		if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests ||
			response.StatusCode >= 500 {
			return fmt.Errorf("%w: %v", ports.ErrSourceUnavailable, responseErr)
		}
		return responseErr
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponse))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode slskd response: %w", err)
	}
	return nil
}

func (c *Client) scheduleDeleteSearch(id string) {
	c.searchCleanupMu.Lock()
	if c.searchClosed {
		c.searchCleanupMu.Unlock()
		return
	}
	c.searchCleanupWG.Add(1)
	c.searchCleanupMu.Unlock()
	go func() {
		defer c.searchCleanupWG.Done()
		timer := time.NewTimer(c.cleanupDelay)
		defer timer.Stop()
		select {
		case <-c.searchCleanupCtx.Done():
			return
		case <-timer.C:
		}
		c.deleteSearch(id)
	}()
}

func (c *Client) deleteSearch(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.doJSON(ctx, http.MethodDelete, "/api/v0/searches/"+url.PathEscape(id), nil, nil)
}
