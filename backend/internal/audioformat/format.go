package audioformat

import (
	"path/filepath"
	"strings"
)

type Format struct {
	Suffix      string
	ContentType string
}

var supported = map[string]Format{
	".aac":  {Suffix: "aac", ContentType: "audio/aac"},
	".aif":  {Suffix: "aif", ContentType: "audio/aiff"},
	".aiff": {Suffix: "aiff", ContentType: "audio/aiff"},
	".alac": {Suffix: "alac", ContentType: "audio/mp4"},
	".ape":  {Suffix: "ape", ContentType: "audio/ape"},
	".flac": {Suffix: "flac", ContentType: "audio/flac"},
	".m4a":  {Suffix: "m4a", ContentType: "audio/mp4"},
	".m4b":  {Suffix: "m4b", ContentType: "audio/mp4"},
	".mp3":  {Suffix: "mp3", ContentType: "audio/mpeg"},
	".mpc":  {Suffix: "mpc", ContentType: "audio/x-musepack"},
	".oga":  {Suffix: "oga", ContentType: "audio/ogg"},
	".ogg":  {Suffix: "ogg", ContentType: "audio/ogg"},
	".opus": {Suffix: "opus", ContentType: "audio/ogg"},
	".wav":  {Suffix: "wav", ContentType: "audio/wav"},
	".wma":  {Suffix: "wma", ContentType: "audio/x-ms-wma"},
	".wv":   {Suffix: "wv", ContentType: "audio/wavpack"},
}

func ByExtension(extension string) (Format, bool) {
	extension = strings.ToLower(strings.TrimSpace(extension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	format, ok := supported[extension]
	return format, ok
}

func FromPath(name string) (Format, bool) {
	return ByExtension(filepath.Ext(name))
}
