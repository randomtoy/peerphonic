package audioformat

import "testing"

func TestSupportedAudioFormats(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"track.MP3": "audio/mpeg", "track.flac": "audio/flac", "track.ogg": "audio/ogg",
		"track.opus": "audio/ogg", "track.m4a": "audio/mp4", "track.alac": "audio/mp4",
		"track.aac": "audio/aac", "track.wav": "audio/wav", "track.aiff": "audio/aiff",
		"track.wma": "audio/x-ms-wma", "track.ape": "audio/ape", "track.wv": "audio/wavpack",
	}
	for path, contentType := range tests {
		format, ok := FromPath(path)
		if !ok || format.ContentType != contentType {
			t.Errorf("FromPath(%q) = %#v, %v", path, format, ok)
		}
	}
	if _, ok := FromPath("cover.jpg"); ok {
		t.Fatal("cover image detected as audio")
	}
}
