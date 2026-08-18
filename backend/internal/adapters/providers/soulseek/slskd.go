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
	"strconv"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const Name = "soulseek"

const (
	searchTimeout  = 5
	searchMaxLimit = 200
	searchMaxWait  = 8 * time.Second
	pollInterval   = 250 * time.Millisecond
	maxResponse    = 8 << 20
)

var audioExtensions = map[string]struct{}{
	"aac": {}, "aiff": {}, "alac": {}, "flac": {}, "m4a": {}, "mp3": {},
	"ogg": {}, "opus": {}, "wav": {}, "wma": {},
}

var audioContentTypes = map[string]string{
	"aac": "audio/aac", "aiff": "audio/aiff", "alac": "audio/mp4", "flac": "audio/flac",
	"m4a": "audio/mp4", "mp3": "audio/mpeg", "ogg": "audio/ogg", "opus": "audio/ogg",
	"wav": "audio/wav", "wma": "audio/x-ms-wma",
}

type Client struct {
	endpoint   *url.URL
	apiKey     string
	httpClient *http.Client
	downloads  *downloadCoordinator
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
	return &Client{
		endpoint: parsed, apiKey: strings.TrimSpace(apiKey), httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Name() string { return Name }

func (c *Client) SetMediaDirectories(downloadsDir, incompleteDir string) error {
	coordinator, err := newDownloadCoordinator(c, downloadsDir, incompleteDir)
	if err != nil {
		return err
	}
	c.downloads = coordinator
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

	payload := struct {
		SearchText    string `json:"searchText"`
		SearchTimeout int    `json:"searchTimeout"`
		ResponseLimit int    `json:"responseLimit"`
		FileLimit     int    `json:"fileLimit"`
	}{
		SearchText: query.Text, SearchTimeout: searchTimeout,
		ResponseLimit: query.Limit, FileLimit: query.Limit * 4,
	}
	var search slskdSearch
	if err := c.doJSON(ctx, http.MethodPost, "/api/v0/searches", payload, &search); err != nil {
		return nil, fmt.Errorf("start slskd search: %w", err)
	}
	if search.ID == "" {
		return nil, errors.New("start slskd search: response did not include an id")
	}
	defer c.deleteSearch(search.ID)

	deadline := time.NewTimer(searchMaxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if search.IsComplete {
			return mapSearchResults(search.Responses, query.Limit), nil
		}
		if err := c.doJSON(ctx, http.MethodGet,
			"/api/v0/searches/"+url.PathEscape(search.ID)+"?includeResponses=true", nil, &search,
		); err != nil {
			return nil, fmt.Errorf("poll slskd search: %w", err)
		}
		if search.IsComplete {
			return mapSearchResults(search.Responses, query.Limit), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return mapSearchResults(search.Responses, query.Limit), nil
		case <-ticker.C:
		}
	}
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
			extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(file.Extension), "."))
			if extension == "" {
				extension = strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")
			}
			if _, ok := audioExtensions[extension]; !ok || strings.TrimSpace(file.Filename) == "" {
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
				Size: file.Size, Suffix: extension, ContentType: audioContentTypes[extension],
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
		return fmt.Errorf("slskd returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
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

func (c *Client) deleteSearch(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.doJSON(ctx, http.MethodDelete, "/api/v0/searches/"+url.PathEscape(id), nil, nil)
}
