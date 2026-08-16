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
}
