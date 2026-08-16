package opensubsonic

import "encoding/xml"

type response struct {
	XMLName       xml.Name         `xml:"subsonic-response" json:"-"`
	XMLNS         string           `xml:"xmlns,attr" json:"-"`
	Status        string           `xml:"status,attr" json:"status"`
	Version       string           `xml:"version,attr" json:"version"`
	Type          string           `xml:"type,attr" json:"type"`
	ServerVersion string           `xml:"serverVersion,attr" json:"serverVersion"`
	OpenSubsonic  bool             `xml:"openSubsonic,attr" json:"openSubsonic"`
	Error         *apiError        `xml:"error,omitempty" json:"error,omitempty"`
	License       *license         `xml:"license,omitempty" json:"license,omitempty"`
	MusicFolders  *musicFolders    `xml:"musicFolders,omitempty" json:"musicFolders,omitempty"`
	Indexes       *indexesResponse `xml:"indexes,omitempty" json:"indexes,omitempty"`
	Directory     *musicDirectory  `xml:"directory,omitempty" json:"directory,omitempty"`
	Genres        *genresResponse  `xml:"genres,omitempty" json:"genres,omitempty"`
	Artists       *artistsID3      `xml:"artists,omitempty" json:"artists,omitempty"`
	ArtistDetail  *artistID3       `xml:"artist,omitempty" json:"artist,omitempty"`
	Album         *albumID3        `xml:"album,omitempty" json:"album,omitempty"`
	AlbumList2    *albumList2      `xml:"albumList2,omitempty" json:"albumList2,omitempty"`
	Song          *child           `xml:"song,omitempty" json:"song,omitempty"`
	Playlists     *playlists       `xml:"playlists,omitempty" json:"playlists,omitempty"`
	Extensions    *extensions      `xml:"openSubsonicExtensions,omitempty" json:"openSubsonicExtensions,omitempty"`
}

type apiError struct {
	Code    int    `xml:"code,attr" json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

type license struct {
	Valid bool `xml:"valid,attr" json:"valid"`
}

type musicFolders struct {
	Folders []musicFolder `xml:"musicFolder" json:"musicFolder"`
}

type musicFolder struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

type indexesResponse struct {
	Indexes []index `xml:"index" json:"index"`
}

type index struct {
	Name    string   `xml:"name,attr" json:"name"`
	Artists []artist `xml:"artist" json:"artist"`
}

type artist struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

type musicDirectory struct {
	ID       string  `xml:"id,attr" json:"id"`
	Name     string  `xml:"name,attr" json:"name"`
	Children []child `xml:"child" json:"child,omitempty"`
}

type child struct {
	ID          string `xml:"id,attr" json:"id"`
	Parent      string `xml:"parent,attr" json:"parent"`
	Title       string `xml:"title,attr" json:"title"`
	Album       string `xml:"album,attr,omitempty" json:"album,omitempty"`
	Artist      string `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	IsDir       bool   `xml:"isDir,attr" json:"isDir"`
	Track       int    `xml:"track,attr,omitempty" json:"track,omitempty"`
	Year        int    `xml:"year,attr,omitempty" json:"year,omitempty"`
	Duration    int    `xml:"duration,attr,omitempty" json:"duration,omitempty"`
	Size        int64  `xml:"size,attr,omitempty" json:"size,omitempty"`
	BitRate     int    `xml:"bitRate,attr,omitempty" json:"bitRate,omitempty"`
	Suffix      string `xml:"suffix,attr,omitempty" json:"suffix,omitempty"`
	ContentType string `xml:"contentType,attr,omitempty" json:"contentType,omitempty"`
	Type        string `xml:"type,attr,omitempty" json:"type,omitempty"`
	SongCount   int    `xml:"songCount,attr,omitempty" json:"songCount,omitempty"`
	AlbumID     string `xml:"albumId,attr,omitempty" json:"albumId,omitempty"`
	ArtistID    string `xml:"artistId,attr,omitempty" json:"artistId,omitempty"`
	DiscNumber  int    `xml:"discNumber,attr,omitempty" json:"discNumber,omitempty"`
	IsVideo     bool   `xml:"isVideo,attr" json:"isVideo"`
}

type genresResponse struct {
	Genres []genre `xml:"genre" json:"genre"`
}

type genre struct {
	SongCount  int    `xml:"songCount,attr" json:"songCount"`
	AlbumCount int    `xml:"albumCount,attr" json:"albumCount"`
	Value      string `xml:",chardata" json:"value"`
}

type artistsID3 struct {
	IgnoredArticles string     `xml:"ignoredArticles,attr" json:"ignoredArticles"`
	Indexes         []indexID3 `xml:"index" json:"index"`
}

type indexID3 struct {
	Name    string      `xml:"name,attr" json:"name"`
	Artists []artistID3 `xml:"artist" json:"artist"`
}

type artistID3 struct {
	ID         string     `xml:"id,attr" json:"id"`
	Name       string     `xml:"name,attr" json:"name"`
	AlbumCount int        `xml:"albumCount,attr" json:"albumCount"`
	Albums     []albumID3 `xml:"album" json:"album,omitempty"`
}

type albumID3 struct {
	ID        string  `xml:"id,attr" json:"id"`
	Parent    string  `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	Name      string  `xml:"name,attr" json:"name"`
	Title     string  `xml:"title,attr" json:"title"`
	Album     string  `xml:"album,attr" json:"album"`
	Artist    string  `xml:"artist,attr" json:"artist"`
	ArtistID  string  `xml:"artistId,attr" json:"artistId"`
	IsDir     bool    `xml:"isDir,attr" json:"isDir"`
	SongCount int     `xml:"songCount,attr" json:"songCount"`
	Duration  int     `xml:"duration,attr" json:"duration"`
	Year      int     `xml:"year,attr,omitempty" json:"year,omitempty"`
	Songs     []child `xml:"song" json:"song,omitempty"`
}

type albumList2 struct {
	Albums []albumID3 `xml:"album" json:"album"`
}

type playlists struct {
	Items []playlist `xml:"playlist" json:"playlist"`
}

type playlist struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

type extensions struct {
	Items []extension `xml:"openSubsonicExtension" json:"openSubsonicExtension"`
}

type extension struct {
	Name     string `xml:"name,attr" json:"name"`
	Versions []int  `xml:"versions" json:"versions"`
}
