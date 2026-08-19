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
	User          *user            `xml:"user,omitempty" json:"user,omitempty"`
	MusicFolders  *musicFolders    `xml:"musicFolders,omitempty" json:"musicFolders,omitempty"`
	Indexes       *indexesResponse `xml:"indexes,omitempty" json:"indexes,omitempty"`
	Directory     *musicDirectory  `xml:"directory,omitempty" json:"directory,omitempty"`
	Genres        *genresResponse  `xml:"genres,omitempty" json:"genres,omitempty"`
	Artists       *artistsID3      `xml:"artists,omitempty" json:"artists,omitempty"`
	ArtistDetail  *artistID3       `xml:"artist,omitempty" json:"artist,omitempty"`
	Album         *albumID3        `xml:"album,omitempty" json:"album,omitempty"`
	AlbumList     *albumList       `xml:"albumList,omitempty" json:"albumList,omitempty"`
	AlbumList2    *albumList2      `xml:"albumList2,omitempty" json:"albumList2,omitempty"`
	SongsByGenre  *songs           `xml:"songsByGenre,omitempty" json:"songsByGenre,omitempty"`
	RandomSongs   *songs           `xml:"randomSongs,omitempty" json:"randomSongs,omitempty"`
	Song          *child           `xml:"song,omitempty" json:"song,omitempty"`
	SearchResult2 *searchResult2   `xml:"searchResult2,omitempty" json:"searchResult2,omitempty"`
	SearchResult3 *searchResult3   `xml:"searchResult3,omitempty" json:"searchResult3,omitempty"`
	Playlists     *playlists       `xml:"playlists,omitempty" json:"playlists,omitempty"`
	Playlist      *playlist        `xml:"playlist,omitempty" json:"playlist,omitempty"`
	Starred       *starred         `xml:"starred,omitempty" json:"starred,omitempty"`
	Starred2      *starredLibrary  `xml:"starred2,omitempty" json:"starred2,omitempty"`
	PlayQueue     *playQueue       `xml:"playQueue,omitempty" json:"playQueue,omitempty"`
	Extensions    *extensions      `xml:"openSubsonicExtensions,omitempty" json:"openSubsonicExtensions,omitempty"`
	ScanStatus    *scanStatus      `xml:"scanStatus,omitempty" json:"scanStatus,omitempty"`
}

type apiError struct {
	Code    int    `xml:"code,attr" json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

type license struct {
	Valid bool `xml:"valid,attr" json:"valid"`
}

type user struct {
	Username          string   `xml:"username,attr" json:"username"`
	ScrobblingEnabled bool     `xml:"scrobblingEnabled,attr" json:"scrobblingEnabled"`
	AdminRole         bool     `xml:"adminRole,attr" json:"adminRole"`
	SettingsRole      bool     `xml:"settingsRole,attr" json:"settingsRole"`
	DownloadRole      bool     `xml:"downloadRole,attr" json:"downloadRole"`
	UploadRole        bool     `xml:"uploadRole,attr" json:"uploadRole"`
	PlaylistRole      bool     `xml:"playlistRole,attr" json:"playlistRole"`
	CoverArtRole      bool     `xml:"coverArtRole,attr" json:"coverArtRole"`
	CommentRole       bool     `xml:"commentRole,attr" json:"commentRole"`
	PodcastRole       bool     `xml:"podcastRole,attr" json:"podcastRole"`
	StreamRole        bool     `xml:"streamRole,attr" json:"streamRole"`
	JukeboxRole       bool     `xml:"jukeboxRole,attr" json:"jukeboxRole"`
	ShareRole         bool     `xml:"shareRole,attr" json:"shareRole"`
	Folders           []string `xml:"folder" json:"folder"`
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
	ID         string `xml:"id,attr" json:"id"`
	Name       string `xml:"name,attr" json:"name"`
	Starred    string `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	UserRating int    `xml:"userRating,attr,omitempty" json:"userRating,omitempty"`
	PlayCount  int64  `xml:"playCount,attr,omitempty" json:"playCount,omitempty"`
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
	Genre       string `xml:"genre,attr,omitempty" json:"genre,omitempty"`
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
	CoverArt    string `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	Starred     string `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	UserRating  int    `xml:"userRating,attr,omitempty" json:"userRating,omitempty"`
	PlayCount   int64  `xml:"playCount,attr,omitempty" json:"playCount,omitempty"`
	Played      string `xml:"played,attr,omitempty" json:"played,omitempty"`
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
	Starred    string     `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	UserRating int        `xml:"userRating,attr,omitempty" json:"userRating,omitempty"`
	PlayCount  int64      `xml:"playCount,attr,omitempty" json:"playCount,omitempty"`
	Albums     []albumID3 `xml:"album" json:"album,omitempty"`
}

type albumID3 struct {
	ID         string  `xml:"id,attr" json:"id"`
	Parent     string  `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	Name       string  `xml:"name,attr" json:"name"`
	Title      string  `xml:"title,attr" json:"title"`
	Album      string  `xml:"album,attr" json:"album"`
	Artist     string  `xml:"artist,attr" json:"artist"`
	ArtistID   string  `xml:"artistId,attr" json:"artistId"`
	IsDir      bool    `xml:"isDir,attr" json:"isDir"`
	SongCount  int     `xml:"songCount,attr" json:"songCount"`
	Duration   int     `xml:"duration,attr" json:"duration"`
	Year       int     `xml:"year,attr,omitempty" json:"year,omitempty"`
	Genre      string  `xml:"genre,attr,omitempty" json:"genre,omitempty"`
	CoverArt   string  `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	Starred    string  `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	UserRating int     `xml:"userRating,attr,omitempty" json:"userRating,omitempty"`
	PlayCount  int64   `xml:"playCount,attr,omitempty" json:"playCount,omitempty"`
	Played     string  `xml:"played,attr,omitempty" json:"played,omitempty"`
	Songs      []child `xml:"song" json:"song,omitempty"`
}

type albumList2 struct {
	Albums []albumID3 `xml:"album" json:"album"`
}

type albumList struct {
	Albums []child `xml:"album" json:"album"`
}

type songs struct {
	Songs []child `xml:"song" json:"song"`
}

type searchResult3 struct {
	Artists []artistID3 `xml:"artist" json:"artist"`
	Albums  []albumID3  `xml:"album" json:"album"`
	Songs   []child     `xml:"song" json:"song"`
}

type searchResult2 struct {
	Artists []artist `xml:"artist" json:"artist"`
	Albums  []child  `xml:"album" json:"album"`
	Songs   []child  `xml:"song" json:"song"`
}

type playlists struct {
	Items []playlist `xml:"playlist" json:"playlist"`
}

type playlist struct {
	ID        string  `xml:"id,attr" json:"id"`
	Name      string  `xml:"name,attr" json:"name"`
	Comment   string  `xml:"comment,attr,omitempty" json:"comment,omitempty"`
	Owner     string  `xml:"owner,attr" json:"owner"`
	Public    bool    `xml:"public,attr" json:"public"`
	Created   string  `xml:"created,attr" json:"created"`
	Changed   string  `xml:"changed,attr" json:"changed"`
	SongCount int     `xml:"songCount,attr" json:"songCount"`
	Duration  int     `xml:"duration,attr" json:"duration"`
	Entries   []child `xml:"entry" json:"entry,omitempty"`
}

type starredLibrary struct {
	Artists []artistID3 `xml:"artist" json:"artist"`
	Albums  []albumID3  `xml:"album" json:"album"`
	Songs   []child     `xml:"song" json:"song"`
}

type starred struct {
	Artists []artist `xml:"artist" json:"artist"`
	Albums  []child  `xml:"album" json:"album"`
	Songs   []child  `xml:"song" json:"song"`
}

type playQueue struct {
	Current   string  `xml:"current,attr,omitempty" json:"current,omitempty"`
	Position  int64   `xml:"position,attr,omitempty" json:"position,omitempty"`
	Username  string  `xml:"username,attr" json:"username"`
	Changed   string  `xml:"changed,attr" json:"changed"`
	ChangedBy string  `xml:"changedBy,attr" json:"changedBy"`
	Entries   []child `xml:"entry" json:"entry"`
}

type extensions struct {
	Items []extension `xml:"openSubsonicExtension" json:"openSubsonicExtension"`
}

type extension struct {
	Name     string `xml:"name,attr" json:"name"`
	Versions []int  `xml:"versions" json:"versions"`
}

type scanStatus struct {
	Scanning bool `xml:"scanning,attr" json:"scanning"`
	Count    int  `xml:"count,attr" json:"count"`
}
