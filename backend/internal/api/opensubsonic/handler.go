package opensubsonic

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
	"github.com/randomtoy/peerphonic/backend/internal/scanner"
)

const (
	apiVersion    = "1.16.1"
	serverVersion = "0.1.0"
	musicFolderID = "music"
)

type Handler struct {
	catalog  ports.Catalog
	streams  *services.StreamingService
	artwork  *services.ArtworkService
	username string
	password string
	scans    scanController
}

type scanController interface {
	Start() bool
	Status() scanner.Status
}

func NewHandler(
	catalog ports.Catalog,
	streams *services.StreamingService,
	artwork *services.ArtworkService,
	username, password string,
	scans ...scanController,
) http.Handler {
	handler := &Handler{
		catalog: catalog, streams: streams, artwork: artwork, username: username, password: password,
	}
	if len(scans) > 0 {
		handler.scans = scans[0]
	}
	return handler
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	endpoint := strings.TrimSuffix(path.Base(request.URL.Path), ".view")
	if err := request.ParseForm(); err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Invalid request parameters")
		return
	}
	if !h.authenticated(request) {
		h.writeError(writer, request, http.StatusUnauthorized, 40, "Wrong username or password")
		return
	}

	switch endpoint {
	case "ping":
		h.write(writer, request, http.StatusOK, response{})
	case "getLicense":
		h.write(writer, request, http.StatusOK, response{License: &license{Valid: true}})
	case "getMusicFolders":
		h.write(writer, request, http.StatusOK, response{MusicFolders: &musicFolders{
			Folders: []musicFolder{{ID: musicFolderID, Name: "Music"}},
		}})
	case "getIndexes":
		h.getIndexes(writer, request)
	case "getMusicDirectory":
		h.getMusicDirectory(writer, request)
	case "getGenres":
		h.write(writer, request, http.StatusOK, response{Genres: &genresResponse{Genres: []genre{}}})
	case "getArtists":
		h.getArtists(writer, request)
	case "getArtist":
		h.getArtist(writer, request)
	case "getAlbumList2":
		h.getAlbumList2(writer, request)
	case "getAlbum":
		h.getAlbum(writer, request)
	case "getSong":
		h.getSong(writer, request)
	case "getCoverArt":
		h.getCoverArt(writer, request)
	case "search3":
		h.search3(writer, request)
	case "getPlaylists":
		h.write(writer, request, http.StatusOK, response{Playlists: &playlists{Items: []playlist{}}})
	case "getOpenSubsonicExtensions":
		h.write(writer, request, http.StatusOK, response{Extensions: &extensions{Items: []extension{}}})
	case "getScanStatus":
		h.getScanStatus(writer, request, false)
	case "startScan":
		h.getScanStatus(writer, request, true)
	case "stream", "download":
		h.stream(writer, request)
	default:
		h.writeError(writer, request, http.StatusNotFound, 0, "Endpoint not implemented")
	}
}

func (h *Handler) getCoverArt(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	if value := request.Form.Get("size"); value != "" {
		size, err := strconv.Atoi(value)
		if err != nil || size <= 0 {
			h.writeError(writer, request, http.StatusBadRequest, 10, "Parameter size must be a positive integer")
			return
		}
	}
	if h.artwork == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Artwork storage is not configured")
		return
	}
	resolved, err := h.artwork.Open(request.Context(), id)
	if errors.Is(err, ports.ErrNotFound) {
		h.writeError(writer, request, http.StatusNotFound, 70, "Cover art not found")
		return
	}
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to open cover art")
		return
	}
	defer resolved.Content.Close()
	writer.Header().Set("Content-Type", resolved.ContentType)
	writer.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(writer, request, resolved.Name, resolved.ModTime, resolved.Content)
}

func (h *Handler) search3(writer http.ResponseWriter, request *http.Request) {
	if !request.Form.Has("query") {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter query is missing")
		return
	}
	payload := &searchResult3{
		Artists: []artistID3{},
		Albums:  []albumID3{},
		Songs:   []child{},
	}
	if folderID := request.Form.Get("musicFolderId"); folderID != "" && folderID != musicFolderID {
		h.write(writer, request, http.StatusOK, response{SearchResult3: payload})
		return
	}

	artistOffset, artistCount, err := searchPageParameters(request, "artist")
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}
	albumOffset, albumCount, err := searchPageParameters(request, "album")
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}
	songOffset, songCount, err := searchPageParameters(request, "song")
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}

	result, err := h.catalog.Search(request.Context(), ports.CatalogSearch{
		Text:         request.Form.Get("query"),
		ArtistOffset: artistOffset, ArtistCount: artistCount,
		AlbumOffset: albumOffset, AlbumCount: albumCount,
		SongOffset: songOffset, SongCount: songCount,
	})
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to search the music catalog")
		return
	}
	for _, artist := range result.Artists {
		payload.Artists = append(payload.Artists, artistID3{
			ID: artist.ID, Name: artist.Name, AlbumCount: artist.AlbumCount,
		})
	}
	for _, album := range result.Albums {
		payload.Albums = append(payload.Albums, makeAlbumID3(album))
	}
	for _, song := range result.Songs {
		payload.Songs = append(payload.Songs, trackChild(song))
	}
	h.write(writer, request, http.StatusOK, response{SearchResult3: payload})
}

func (h *Handler) getScanStatus(writer http.ResponseWriter, request *http.Request, start bool) {
	if h.scans == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Music scanner is not configured")
		return
	}
	if start {
		h.scans.Start()
	}
	status := h.scans.Status()
	h.write(writer, request, http.StatusOK, response{ScanStatus: &scanStatus{
		Scanning: status.Scanning,
		Count:    status.Count,
	}})
}

func (h *Handler) getArtists(writer http.ResponseWriter, request *http.Request) {
	items, err := h.catalog.Artists(request.Context())
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	groups := make(map[string][]artistID3)
	for _, item := range items {
		name := indexName(item.Name)
		groups[name] = append(groups[name], artistID3{
			ID: item.ID, Name: item.Name, AlbumCount: item.AlbumCount,
		})
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	payload := &artistsID3{IgnoredArticles: "The An A", Indexes: []indexID3{}}
	for _, name := range names {
		payload.Indexes = append(payload.Indexes, indexID3{Name: name, Artists: groups[name]})
	}
	h.write(writer, request, http.StatusOK, response{Artists: payload})
}

func (h *Handler) getArtist(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	albums, err := h.catalog.AlbumsByArtist(request.Context(), id)
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	if len(albums) == 0 {
		h.writeError(writer, request, http.StatusNotFound, 70, "Artist not found")
		return
	}
	item := &artistID3{ID: id, Name: albums[0].Artist, AlbumCount: len(albums), Albums: []albumID3{}}
	for _, album := range albums {
		item.Albums = append(item.Albums, makeAlbumID3(album))
	}
	h.write(writer, request, http.StatusOK, response{ArtistDetail: item})
}

func (h *Handler) getAlbumList2(writer http.ResponseWriter, request *http.Request) {
	offset, limit, err := pageParameters(request)
	if err != nil {
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
		return
	}
	albums, err := h.catalog.Albums(request.Context(), offset, limit)
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	payload := &albumList2{Albums: make([]albumID3, 0, len(albums))}
	for _, album := range albums {
		payload.Albums = append(payload.Albums, makeAlbumID3(album))
	}
	h.write(writer, request, http.StatusOK, response{AlbumList2: payload})
}

func (h *Handler) getAlbum(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	tracks, err := h.catalog.TracksByAlbum(request.Context(), id)
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	if len(tracks) == 0 {
		h.writeError(writer, request, http.StatusNotFound, 70, "Album not found")
		return
	}
	album := albumFromTracks(tracks)
	payload := makeAlbumID3(album)
	payload.Songs = make([]child, 0, len(tracks))
	for _, track := range tracks {
		payload.Songs = append(payload.Songs, trackChild(track))
	}
	h.write(writer, request, http.StatusOK, response{Album: &payload})
}

func (h *Handler) getSong(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	track, err := h.catalog.Track(request.Context(), id)
	if errors.Is(err, ports.ErrNotFound) {
		h.writeError(writer, request, http.StatusNotFound, 70, "Song not found")
		return
	}
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	song := trackChild(track)
	h.write(writer, request, http.StatusOK, response{Song: &song})
}

func (h *Handler) getIndexes(writer http.ResponseWriter, request *http.Request) {
	artists, err := h.catalog.Artists(request.Context())
	if err != nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
		return
	}
	groups := make(map[string][]artist)
	for _, item := range artists {
		name := indexName(item.Name)
		groups[name] = append(groups[name], artist{ID: item.ID, Name: item.Name})
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	indexes := &indexesResponse{}
	for _, name := range names {
		indexes.Indexes = append(indexes.Indexes, index{Name: name, Artists: groups[name]})
	}
	h.write(writer, request, http.StatusOK, response{Indexes: indexes})
}

func (h *Handler) getMusicDirectory(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	directory := &musicDirectory{ID: id}
	switch {
	case id == musicFolderID:
		directory.Name = "Music"
		artists, err := h.catalog.Artists(request.Context())
		if err != nil {
			h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
			return
		}
		for _, item := range artists {
			directory.Children = append(directory.Children, child{
				ID: item.ID, Parent: musicFolderID, Title: item.Name, Artist: item.Name, IsDir: true,
			})
		}
	case strings.HasPrefix(id, "artist_"):
		albums, err := h.catalog.AlbumsByArtist(request.Context(), id)
		if err != nil {
			h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
			return
		}
		if len(albums) == 0 {
			h.writeError(writer, request, http.StatusNotFound, 70, "Directory not found")
			return
		}
		directory.Name = albums[0].Artist
		for _, item := range albums {
			directory.Children = append(directory.Children, albumChild(item))
		}
	case strings.HasPrefix(id, "album_"):
		tracks, err := h.catalog.TracksByAlbum(request.Context(), id)
		if err != nil {
			h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to read the music catalog")
			return
		}
		if len(tracks) == 0 {
			h.writeError(writer, request, http.StatusNotFound, 70, "Directory not found")
			return
		}
		directory.Name = tracks[0].Album
		for _, item := range tracks {
			directory.Children = append(directory.Children, trackChild(item))
		}
	default:
		h.writeError(writer, request, http.StatusNotFound, 70, "Directory not found")
		return
	}
	h.write(writer, request, http.StatusOK, response{Directory: directory})
}

func (h *Handler) stream(writer http.ResponseWriter, request *http.Request) {
	id := request.Form.Get("id")
	if id == "" {
		h.writeError(writer, request, http.StatusBadRequest, 10, "Required parameter id is missing")
		return
	}
	resolved, err := h.streams.Open(request.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			h.writeError(writer, request, http.StatusNotFound, 70, "Song not found")
			return
		}
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to open the song")
		return
	}
	defer resolved.Content.Close()
	if resolved.ContentType != "" {
		writer.Header().Set("Content-Type", resolved.ContentType)
	}
	http.ServeContent(writer, request, resolved.Name, resolved.ModTime, resolved.Content)
}

func (h *Handler) authenticated(request *http.Request) bool {
	username := request.Form.Get("u")
	if subtle.ConstantTimeCompare([]byte(username), []byte(h.username)) != 1 {
		return false
	}
	providedPassword := request.Form.Get("p")
	if strings.HasPrefix(providedPassword, "enc:") {
		decoded, err := hex.DecodeString(strings.TrimPrefix(providedPassword, "enc:"))
		if err != nil {
			return false
		}
		providedPassword = string(decoded)
	}
	if providedPassword != "" {
		return subtle.ConstantTimeCompare([]byte(providedPassword), []byte(h.password)) == 1
	}
	salt := request.Form.Get("s")
	token := request.Form.Get("t")
	if salt == "" || token == "" {
		return false
	}
	expected := fmt.Sprintf("%x", md5.Sum([]byte(h.password+salt)))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(token)), []byte(expected)) == 1
}

func (h *Handler) writeError(writer http.ResponseWriter, request *http.Request, status, code int, message string) {
	h.write(writer, request, status, response{Error: &apiError{Code: code, Message: message}})
}

func (h *Handler) write(writer http.ResponseWriter, request *http.Request, status int, payload response) {
	payload.XMLNS = "http://subsonic.org/restapi"
	payload.Status = "ok"
	if payload.Error != nil {
		payload.Status = "failed"
	}
	payload.Version = apiVersion
	payload.Type = "peerphonic"
	payload.ServerVersion = serverVersion
	payload.OpenSubsonic = true

	if strings.EqualFold(request.Form.Get("f"), "json") {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(map[string]response{"subsonic-response": payload})
		return
	}
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(xml.Header))
	_ = xml.NewEncoder(writer).Encode(payload)
}

func indexName(name string) string {
	for _, value := range strings.TrimSpace(name) {
		if unicode.IsLetter(value) {
			return strings.ToUpper(string(value))
		}
		break
	}
	return "#"
}

func albumChild(item domain.Album) child {
	return child{
		ID: item.ID, Parent: item.ArtistID, Title: item.Name, Album: item.Name,
		Artist: item.Artist, IsDir: true, Year: item.Year, SongCount: item.SongCount,
		Duration: int(item.Duration.Seconds()), CoverArt: item.CoverArtID,
	}
}

func trackChild(item domain.Track) child {
	return child{
		ID: item.ID, Parent: item.AlbumID, Title: item.Title, Album: item.Album,
		Artist: item.Artist, IsDir: false, Track: item.TrackNumber, Year: item.Year,
		Duration: int(item.Duration.Seconds()), Size: item.Size, BitRate: item.BitRate,
		Suffix: item.Suffix, ContentType: item.ContentType, Type: "music",
		AlbumID: item.AlbumID, ArtistID: item.ArtistID, DiscNumber: item.DiscNumber,
		CoverArt: item.CoverArtID,
	}
}

func makeAlbumID3(item domain.Album) albumID3 {
	return albumID3{
		ID: item.ID, Parent: item.ArtistID, Name: item.Name, Title: item.Name,
		Album: item.Name, Artist: item.Artist, ArtistID: item.ArtistID, IsDir: true,
		SongCount: item.SongCount, Duration: int(item.Duration.Seconds()), Year: item.Year,
		CoverArt: item.CoverArtID,
	}
}

func albumFromTracks(tracks []domain.Track) domain.Album {
	first := tracks[0]
	album := domain.Album{
		ID: first.AlbumID, Name: first.Album, Artist: first.AlbumArtist,
		ArtistID: first.ArtistID, Year: first.Year, SongCount: len(tracks),
		CoverArtID: first.CoverArtID,
	}
	for _, track := range tracks {
		album.Duration += track.Duration
		if album.Year == 0 && track.Year != 0 {
			album.Year = track.Year
		}
	}
	return album
}

func pageParameters(request *http.Request) (offset, limit int, err error) {
	limit = 10
	if value := request.Form.Get("size"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 0 {
			return 0, 0, errors.New("parameter size must be a non-negative integer")
		}
	}
	if limit > 500 {
		limit = 500
	}
	if value := request.Form.Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("parameter offset must be a non-negative integer")
		}
	}
	return offset, limit, nil
}

func searchPageParameters(request *http.Request, kind string) (offset, count int, err error) {
	count = 20
	countName := kind + "Count"
	if value := request.Form.Get(countName); value != "" {
		count, err = strconv.Atoi(value)
		if err != nil || count < 0 {
			return 0, 0, fmt.Errorf("parameter %s must be a non-negative integer", countName)
		}
	}
	if count > 500 {
		count = 500
	}
	offsetName := kind + "Offset"
	if value := request.Form.Get(offsetName); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("parameter %s must be a non-negative integer", offsetName)
		}
	}
	return offset, count, nil
}
