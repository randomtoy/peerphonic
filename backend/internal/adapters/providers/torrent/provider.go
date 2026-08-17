package torrent

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	torrentclient "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const Name = "torrent"

var audioContentTypes = map[string]string{
	".aac": "audio/aac", ".flac": "audio/flac", ".m4a": "audio/mp4",
	".mp3": "audio/mpeg", ".oga": "audio/ogg", ".ogg": "audio/ogg",
	".opus": "audio/ogg", ".wav": "audio/wav",
}

var imageContentTypes = map[string]string{
	".gif": "image/gif", ".jpeg": "image/jpeg", ".jpg": "image/jpeg",
	".png": "image/png", ".webp": "image/webp",
}

const streamReadahead = 8 << 20

type Provider struct {
	metadataRoot string
	dataRoot     string

	clientMu  sync.Mutex
	client    *torrentclient.Client
	operation sync.RWMutex

	cacheMu        sync.Mutex
	cacheFiles     map[string]cacheFile
	active         map[string]int
	lastAccessed   map[string]time.Time
	onCacheChanged func()

	artworkMu sync.RWMutex
	artworks  map[string]domain.SourceRef

	completionMu sync.Mutex
	completionWG sync.WaitGroup
	trackIDs     map[string]string
	onCompleted  func(CompletedFile)
	closing      bool
}

type CompletedFile struct {
	TrackID string
	Path    string
}

type Catalog struct {
	InfoHash string
	Name     string
	Tracks   []domain.TrackSource
}

type catalogFile struct {
	parts       []string
	length      int64
	extension   string
	contentType string
}

type cacheFile struct {
	infoHash    string
	logicalPath string
}

type evictionCandidate struct {
	key          string
	infoHash     string
	path         string
	lastAccessed time.Time
	size         int64
}

func New() *Provider {
	return &Provider{
		artworks:     make(map[string]domain.SourceRef),
		trackIDs:     make(map[string]string),
		cacheFiles:   make(map[string]cacheFile),
		active:       make(map[string]int),
		lastAccessed: make(map[string]time.Time),
	}
}

// NewStreaming configures a provider that joins a swarm only when media or
// artwork is opened. Downloaded pieces are persisted under dataRoot.
func NewStreaming(metadataRoot, dataRoot string) (*Provider, error) {
	metadataRoot, err := filepath.Abs(metadataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve torrent metadata directory: %w", err)
	}
	dataRoot, err = filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve torrent data directory: %w", err)
	}
	provider := New()
	provider.metadataRoot = metadataRoot
	provider.dataRoot = dataRoot
	return provider, nil
}

func (*Provider) Name() string { return Name }

func (*Provider) Search(context.Context, domain.SearchQuery) ([]domain.TrackSource, error) {
	return nil, nil
}

func (p *Provider) Resolve(ctx context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	if ref.Provider != Name {
		return ports.ResolvedSource{}, fmt.Errorf("cannot resolve provider %q", ref.Provider)
	}
	return p.open(ctx, ref)
}

func (p *Provider) OpenArtwork(ctx context.Context, id string) (ports.ResolvedSource, error) {
	p.artworkMu.RLock()
	ref, ok := p.artworks[id]
	p.artworkMu.RUnlock()
	if !ok {
		return ports.ResolvedSource{}, ports.ErrNotFound
	}
	return p.open(ctx, ref)
}

func (p *Provider) SetCompletedHandler(handler func(CompletedFile)) {
	p.completionMu.Lock()
	p.onCompleted = handler
	p.completionMu.Unlock()
}

func (p *Provider) SetCacheChangedHandler(handler func()) {
	p.cacheMu.Lock()
	p.onCacheChanged = handler
	p.cacheMu.Unlock()
}

// CachedPath returns a fully materialized file without starting the torrent
// client. Incomplete files remain provider-owned .part data and are ignored.
func (p *Provider) CachedPath(ref domain.SourceRef, expectedSize int64) (string, bool) {
	if ref.Provider != Name {
		return "", false
	}
	_, logicalPath, err := parseSourceKey(ref.Key)
	if err != nil {
		return "", false
	}
	mediaPath := filepath.Join(p.dataRoot, filepath.FromSlash(logicalPath))
	info, err := os.Stat(mediaPath)
	if err != nil || !info.Mode().IsRegular() || (expectedSize > 0 && info.Size() != expectedSize) {
		return "", false
	}
	return mediaPath, true
}

// CacheUsage reports physical disk allocation rather than logical file sizes,
// because incomplete torrent files can be sparse.
func (p *Provider) CacheUsage(ctx context.Context) (domain.CacheUsage, error) {
	usage := domain.CacheUsage{Name: Name}
	err := filepath.WalkDir(p.dataRoot, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) && filePath == p.dataRoot {
				return filepath.SkipDir
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		usage.Size += allocatedFileSize(info)
		if strings.HasPrefix(entry.Name(), ".torrent.db") {
			return nil
		}
		usage.Entries++
		if strings.HasSuffix(entry.Name(), ".part") {
			usage.PartialEntries++
		}
		return nil
	})
	if err != nil {
		return domain.CacheUsage{}, fmt.Errorf("inspect torrent cache: %w", err)
	}
	return usage, nil
}

// Evict removes least recently accessed torrent files. Active torrents are
// skipped, and an inactive torrent is dropped from the client before any of
// its files are removed so piece completion is re-evaluated on the next open.
func (p *Provider) Evict(ctx context.Context, bytes int64) (int64, error) {
	if bytes <= 0 {
		return 0, nil
	}
	p.operation.Lock()
	defer p.operation.Unlock()

	candidates, err := p.evictionCandidates(ctx)
	if err != nil {
		return 0, err
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].lastAccessed.Before(candidates[right].lastAccessed)
	})
	dropped := make(map[string]bool)
	var freed int64
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return freed, err
		}
		p.cacheMu.Lock()
		active := p.active[candidate.infoHash]
		p.cacheMu.Unlock()
		if active > 0 {
			continue
		}
		if !dropped[candidate.infoHash] {
			p.dropTorrent(candidate.infoHash)
			dropped[candidate.infoHash] = true
		}
		if err := os.Remove(candidate.path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return freed, fmt.Errorf("remove cached torrent file %q: %w", candidate.path, err)
		}
		freed += candidate.size
		p.cacheMu.Lock()
		delete(p.lastAccessed, candidate.key)
		p.cacheMu.Unlock()
		if freed >= bytes {
			break
		}
	}
	return freed, nil
}

func (p *Provider) Close() error {
	p.cacheMu.Lock()
	p.onCacheChanged = nil
	p.cacheMu.Unlock()
	p.operation.Lock()
	defer p.operation.Unlock()
	p.completionMu.Lock()
	p.closing = true
	p.completionMu.Unlock()
	p.clientMu.Lock()
	client := p.client
	p.client = nil
	p.clientMu.Unlock()
	if client == nil {
		p.completionWG.Wait()
		return nil
	}
	err := errors.Join(client.Close()...)
	p.completionWG.Wait()
	return err
}

func (p *Provider) ReadCatalog(reader io.Reader) (Catalog, error) {
	meta, err := metainfo.Load(reader)
	if err != nil {
		return Catalog{}, fmt.Errorf("read torrent metainfo: %w", err)
	}
	info, err := meta.UnmarshalInfo()
	if err != nil {
		return Catalog{}, fmt.Errorf("decode torrent info: %w", err)
	}
	name := strings.TrimSpace(info.BestName())
	if name == "" {
		return Catalog{}, fmt.Errorf("torrent name is empty")
	}
	infoHash := meta.HashInfoBytes().HexString()
	result := Catalog{InfoHash: infoHash, Name: name}
	var audioFiles []catalogFile
	var artworkFiles []catalogFile
	for _, file := range info.UpvertedFiles() {
		parts, err := safePath(name, file.BestPath())
		if err != nil {
			return Catalog{}, err
		}
		extension := strings.ToLower(path.Ext(parts[len(parts)-1]))
		if contentType, supported := audioContentTypes[extension]; supported {
			if detected := mime.TypeByExtension(extension); detected != "" {
				contentType = detected
			}
			audioFiles = append(audioFiles, catalogFile{
				parts: parts, length: file.Length, extension: extension, contentType: contentType,
			})
		} else if contentType, supported := imageContentTypes[extension]; supported {
			artworkFiles = append(artworkFiles, catalogFile{
				parts: parts, length: file.Length, extension: extension, contentType: contentType,
			})
		}
	}
	for _, file := range audioFiles {
		track := makeTrack(infoHash, file.parts, file.length, file.extension, file.contentType)
		p.registerTrack(track.Ref.Key, track.Track.ID)
		p.registerCacheFile(track.Ref.Key, infoHash, strings.Join(file.parts, "/"))
		if artwork, ok := bestArtwork(file.parts, artworkFiles); ok {
			logicalPath := strings.Join(artwork.parts, "/")
			id := domain.StableID("torrentart", infoHash, logicalPath)
			track.Track.CoverArtID = id
			p.registerArtwork(id, domain.SourceRef{Provider: Name, Key: infoHash + "/" + logicalPath})
		}
		result.Tracks = append(result.Tracks, track)
	}
	for _, file := range artworkFiles {
		logicalPath := strings.Join(file.parts, "/")
		p.registerCacheFile(infoHash+"/"+logicalPath, infoHash, logicalPath)
	}
	return result, nil
}

func (p *Provider) open(ctx context.Context, ref domain.SourceRef) (ports.ResolvedSource, error) {
	p.operation.RLock()
	defer p.operation.RUnlock()

	infoHash, logicalPath, err := parseSourceKey(ref.Key)
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	client, err := p.ensureClient()
	if err != nil {
		return ports.ResolvedSource{}, err
	}
	metadataPath := filepath.Join(p.metadataRoot, infoHash+".torrent")
	torrent, err := client.AddTorrentFromFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ports.ResolvedSource{}, fmt.Errorf("%w: torrent metadata is missing", ports.ErrSourceUnavailable)
		}
		return ports.ResolvedSource{}, fmt.Errorf("add torrent %s: %w", infoHash, err)
	}
	select {
	case <-torrent.GotInfo():
	case <-ctx.Done():
		return ports.ResolvedSource{}, ctx.Err()
	}
	for _, file := range torrent.Files() {
		if file.Path() != logicalPath {
			continue
		}
		reader := file.NewReader()
		reader.SetContext(ctx)
		reader.SetReadahead(streamReadahead)
		p.retain(infoHash, ref.Key)
		return ports.ResolvedSource{
			Content: &boundedReader{
				ReadSeekCloser: reader,
				length:         file.Length(),
				contiguous:     true,
				onComplete: func() {
					p.notifyCompleted(ref, logicalPath)
				},
				onClose: func() { p.release(infoHash) },
			},
			Name: path.Base(logicalPath), ContentType: contentType(logicalPath),
			Size: file.Length(), ModTime: time.Time{},
		}, nil
	}
	return ports.ResolvedSource{}, fmt.Errorf("%w: torrent file %q is missing", ports.ErrSourceUnavailable, logicalPath)
}

func (p *Provider) evictionCandidates(ctx context.Context) ([]evictionCandidate, error) {
	p.cacheMu.Lock()
	files := make(map[string]cacheFile, len(p.cacheFiles))
	accessed := make(map[string]time.Time, len(p.lastAccessed))
	for key, file := range p.cacheFiles {
		files[key] = file
	}
	for key, value := range p.lastAccessed {
		accessed[key] = value
	}
	p.cacheMu.Unlock()

	var candidates []evictionCandidate
	for key, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filePath := filepath.Join(p.dataRoot, filepath.FromSlash(file.logicalPath))
		info, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			filePath += ".part"
			info, err = os.Stat(filePath)
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect cached torrent file %q: %w", filePath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		lastAccessed := accessed[key]
		if lastAccessed.IsZero() {
			lastAccessed = info.ModTime()
		}
		candidates = append(candidates, evictionCandidate{
			key: key, infoHash: file.infoHash, path: filePath,
			lastAccessed: lastAccessed, size: allocatedFileSize(info),
		})
	}
	return candidates, nil
}

func (p *Provider) dropTorrent(infoHash string) {
	hash := metainfo.NewHashFromHex(infoHash)
	p.clientMu.Lock()
	client := p.client
	p.clientMu.Unlock()
	if client == nil {
		return
	}
	if torrent, ok := client.Torrent(hash); ok {
		torrent.Drop()
	}
}

func (p *Provider) ensureClient() (*torrentclient.Client, error) {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	if p.client != nil {
		return p.client, nil
	}
	if p.metadataRoot == "" || p.dataRoot == "" {
		return nil, fmt.Errorf("%w: torrent streaming is not configured", ports.ErrSourceUnavailable)
	}
	if err := os.MkdirAll(p.dataRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create torrent data directory: %w", err)
	}
	config := torrentclient.NewDefaultClientConfig()
	config.DataDir = p.dataRoot
	config.ListenPort = 0
	config.NoDefaultPortForwarding = true
	config.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := torrentclient.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("start torrent client: %w", err)
	}
	p.client = client
	return client, nil
}

func parseSourceKey(key string) (infoHash, logicalPath string, err error) {
	infoHash, logicalPath, ok := strings.Cut(key, "/")
	if !ok || logicalPath == "" || len(infoHash) != 40 {
		return "", "", fmt.Errorf("invalid torrent source key")
	}
	if _, err := hex.DecodeString(infoHash); err != nil {
		return "", "", fmt.Errorf("invalid torrent info hash: %w", err)
	}
	if path.Clean(logicalPath) != logicalPath || strings.HasPrefix(logicalPath, "../") {
		return "", "", fmt.Errorf("invalid torrent source path")
	}
	return infoHash, logicalPath, nil
}

func contentType(name string) string {
	extension := strings.ToLower(path.Ext(name))
	if value := audioContentTypes[extension]; value != "" {
		return value
	}
	return imageContentTypes[extension]
}

func (p *Provider) registerArtwork(id string, ref domain.SourceRef) {
	p.artworkMu.Lock()
	p.artworks[id] = ref
	p.artworkMu.Unlock()
}

func (p *Provider) registerTrack(sourceKey, trackID string) {
	p.completionMu.Lock()
	p.trackIDs[sourceKey] = trackID
	p.completionMu.Unlock()
}

func (p *Provider) registerCacheFile(key, infoHash, logicalPath string) {
	p.cacheMu.Lock()
	p.cacheFiles[key] = cacheFile{infoHash: infoHash, logicalPath: logicalPath}
	p.cacheMu.Unlock()
}

func (p *Provider) retain(infoHash, key string) {
	p.cacheMu.Lock()
	p.active[infoHash]++
	p.lastAccessed[key] = time.Now().UTC()
	p.cacheMu.Unlock()
}

func (p *Provider) release(infoHash string) {
	p.cacheMu.Lock()
	if p.active[infoHash] <= 1 {
		delete(p.active, infoHash)
	} else {
		p.active[infoHash]--
	}
	handler := p.onCacheChanged
	p.cacheMu.Unlock()
	if handler != nil {
		handler()
	}
}

func (p *Provider) notifyCompleted(ref domain.SourceRef, logicalPath string) {
	infoHash, _, err := parseSourceKey(ref.Key)
	if err != nil {
		return
	}
	p.completionMu.Lock()
	trackID := p.trackIDs[ref.Key]
	handler := p.onCompleted
	if trackID == "" || handler == nil || p.closing {
		p.completionMu.Unlock()
		return
	}
	p.completionWG.Add(1)
	p.completionMu.Unlock()
	p.retain(infoHash, ref.Key)
	file := CompletedFile{
		TrackID: trackID,
		Path:    filepath.Join(p.dataRoot, filepath.FromSlash(logicalPath)),
	}
	go func() {
		defer p.completionWG.Done()
		defer p.release(infoHash)
		handler(file)
	}()
}

func bestArtwork(audioParts []string, candidates []catalogFile) (catalogFile, bool) {
	albumDirectory := strings.Join(audioParts[:len(audioParts)-1], "/")
	bestScore := int(^uint(0) >> 1)
	var best catalogFile
	found := false
	for _, candidate := range candidates {
		candidateDirectory := strings.Join(candidate.parts[:len(candidate.parts)-1], "/")
		depth := 0
		switch {
		case candidateDirectory == albumDirectory:
		case strings.HasPrefix(candidateDirectory, albumDirectory+"/"):
			depth = strings.Count(strings.TrimPrefix(candidateDirectory, albumDirectory+"/"), "/") + 1
		default:
			continue
		}
		priority := torrentArtworkPriority(candidate.parts[len(candidate.parts)-1])
		score := depth*10 + priority
		logicalPath := strings.Join(candidate.parts, "/")
		if !found || score < bestScore ||
			(score == bestScore && logicalPath < strings.Join(best.parts, "/")) {
			best, bestScore, found = candidate, score, true
		}
	}
	return best, found
}

func torrentArtworkPriority(name string) int {
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	switch stem {
	case "cover", "folder", "front", "albumart":
		return 0
	}
	for _, prefix := range []string{"cover", "folder", "front", "albumart"} {
		if strings.HasPrefix(stem, prefix) {
			return 1
		}
	}
	for _, prefix := range []string{"scan", "img", "image", "artwork", "booklet"} {
		if strings.HasPrefix(stem, prefix) {
			return 30
		}
	}
	for _, prefix := range []string{"cd", "disc", "disk"} {
		if strings.HasPrefix(stem, prefix) {
			return 40
		}
	}
	if strings.HasPrefix(stem, "back") || strings.HasPrefix(stem, "rear") {
		return 80
	}
	return 50
}

// boundedReader prevents a piece shared with the next torrent file from being
// exposed past the selected file's logical end.
type boundedReader struct {
	ports.ReadSeekCloser
	position    int64
	length      int64
	readStarted bool
	contiguous  bool
	completed   bool
	onComplete  func()
	onClose     func()
	closeOnce   sync.Once
}

func (r *boundedReader) Read(buffer []byte) (int, error) {
	if r.position >= r.length {
		return 0, io.EOF
	}
	remaining := r.length - r.position
	if int64(len(buffer)) > remaining {
		buffer = buffer[:remaining]
	}
	read, err := r.ReadSeekCloser.Read(buffer)
	r.readStarted = r.readStarted || read > 0
	r.position += int64(read)
	if r.position >= r.length && r.contiguous && !r.completed && r.onComplete != nil {
		r.completed = true
		r.onComplete()
	}
	if err == nil && r.position >= r.length {
		err = io.EOF
	}
	return read, err
}

func (r *boundedReader) Seek(offset int64, whence int) (int64, error) {
	previous := r.position
	position, err := r.ReadSeekCloser.Seek(offset, whence)
	if err == nil {
		r.position = position
		if !r.readStarted {
			r.contiguous = position == 0
		} else if position != previous {
			r.contiguous = false
		}
	}
	return position, err
}

func (r *boundedReader) Close() error {
	err := r.ReadSeekCloser.Close()
	r.closeOnce.Do(func() {
		if r.onClose != nil {
			r.onClose()
		}
	})
	return err
}

func safePath(root string, fileParts []string) ([]string, error) {
	parts := make([]string, 0, len(fileParts)+1)
	parts = append(parts, root)
	for _, part := range fileParts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\\`) {
			return nil, fmt.Errorf("torrent contains unsafe path component %q", part)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func makeTrack(infoHash string, parts []string, size int64, extension, contentType string) domain.TrackSource {
	fileName := parts[len(parts)-1]
	title, trackNumber := titleAndNumber(strings.TrimSuffix(fileName, path.Ext(fileName)))
	if len(parts) == 1 {
		if _, candidate, ok := strings.Cut(title, " - "); ok && strings.TrimSpace(candidate) != "" {
			title = strings.TrimSpace(candidate)
		}
	}
	artist, album, discNumber := provisionalArtistAlbum(parts)
	artistID := domain.StableID("artist", strings.ToLower(artist))
	albumID := domain.StableID("album", artistID, strings.ToLower(album))
	logicalPath := strings.Join(parts, "/")
	return domain.TrackSource{
		Track: domain.Track{
			ID: domain.StableID("track", Name, infoHash, logicalPath), Title: title,
			Artist: artist, ArtistID: artistID, Album: album, AlbumID: albumID,
			AlbumArtist: artist, AlbumArtistID: artistID, TrackNumber: trackNumber,
			DiscNumber: discNumber, Year: leadingYear(album), Size: size,
			Suffix: strings.TrimPrefix(extension, "."), ContentType: contentType,
		},
		Ref: domain.SourceRef{Provider: Name, Key: infoHash + "/" + logicalPath},
	}
}

func provisionalArtistAlbum(parts []string) (artist, album string, discNumber int) {
	root := parts[0]
	if len(parts) == 1 {
		root = strings.TrimSuffix(root, path.Ext(root))
	}
	rootArtist, rootAlbum, rootSplit := strings.Cut(root, " - ")
	directories := parts[:len(parts)-1]
	if len(parts) == 1 && rootSplit {
		artist, album = strings.TrimSpace(rootArtist), "Unknown Album"
	} else if len(directories) <= 1 && rootSplit {
		artist, album = strings.TrimSpace(rootArtist), strings.TrimSpace(rootAlbum)
	} else if len(directories) <= 1 {
		artist, album = "Unknown Artist", root
	} else {
		albumIndex := len(directories) - 1
		if number, ok := discDirectoryNumber(directories[albumIndex]); ok && albumIndex > 1 {
			discNumber = number
			albumIndex--
		}
		album = directories[albumIndex]
		artistIndex := albumIndex - 1
		for index := 1; index < albumIndex; index++ {
			if collectionDirectory(directories[index]) {
				artistIndex = index - 1
				break
			}
		}
		if artistIndex >= 0 {
			artist = directories[artistIndex]
		}
		if artistIndex == 0 && rootSplit {
			artist = strings.TrimSpace(rootArtist)
		}
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	artist = normalizedArtistDirectory(artist)
	if album == "" {
		album = root
	}
	return artist, album, discNumber
}

func normalizedArtistDirectory(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, prefix := range []string{"группа ", "группа: "} {
		if strings.HasPrefix(lower, prefix) {
			name := strings.TrimSpace(value[len(prefix):])
			if name != "" {
				return name
			}
		}
	}
	return value
}

func collectionDirectory(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimLeftFunc(value, func(character rune) bool {
		return unicode.IsDigit(character) || unicode.IsSpace(character) ||
			character == '.' || character == '-' || character == '_'
	})
	for _, marker := range []string{
		"альбом", "сингл", "сборник", "компиляц", "юбилей", "неофициаль",
		"релиз", "albums", "singles", "compilation", "collection", "bootleg", "releases",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func discDirectoryNumber(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"cd", "disc", "disk", "диск"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		remainder := strings.TrimLeftFunc(strings.TrimPrefix(value, prefix), func(character rune) bool {
			return unicode.IsSpace(character) || character == '.' || character == '-' ||
				character == '_' || character == '№'
		})
		end := 0
		for end < len(remainder) && remainder[end] >= '0' && remainder[end] <= '9' {
			end++
		}
		if end == 0 {
			return 0, false
		}
		number, err := strconv.Atoi(remainder[:end])
		return number, err == nil && number > 0
	}
	return 0, false
}

func leadingYear(value string) int {
	value = strings.TrimLeft(value, " ([{")
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1900 || year > 2100 {
		return 0
	}
	return year
}

func titleAndNumber(value string) (string, int) {
	value = strings.TrimSpace(value)
	index := 0
	for index < len(value) && index < 3 && value[index] >= '0' && value[index] <= '9' {
		index++
	}
	if index == 0 || index == len(value) || !isTrackSeparator(rune(value[index])) {
		return value, 0
	}
	number, err := strconv.Atoi(value[:index])
	if err != nil {
		return value, 0
	}
	title := strings.TrimLeftFunc(value[index:], isTrackSeparator)
	if title == "" {
		return value, 0
	}
	return title, number
}

func isTrackSeparator(value rune) bool {
	return unicode.IsSpace(value) || value == '.' || value == '-' || value == '_'
}
