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
	"strings"
	"unicode"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

const (
	apiVersion    = "1.16.1"
	serverVersion = "0.1.0"
	musicFolderID = "music"
)

type Handler struct {
	catalog  ports.Catalog
	streams  *services.StreamingService
	username string
	password string
}

func NewHandler(catalog ports.Catalog, streams *services.StreamingService, username, password string) http.Handler {
	return &Handler{catalog: catalog, streams: streams, username: username, password: password}
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
	case "stream", "download":
		h.stream(writer, request)
	default:
		h.writeError(writer, request, http.StatusNotFound, 0, "Endpoint not implemented")
	}
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
		Duration: int(item.Duration.Seconds()),
	}
}

func trackChild(item domain.Track) child {
	return child{
		ID: item.ID, Parent: item.AlbumID, Title: item.Title, Album: item.Album,
		Artist: item.Artist, IsDir: false, Track: item.TrackNumber, Year: item.Year,
		Duration: int(item.Duration.Seconds()), Size: item.Size, BitRate: item.BitRate,
		Suffix: item.Suffix, ContentType: item.ContentType, Type: "music",
	}
}
