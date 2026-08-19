package streaming

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestNewFFmpegRejectsMissingExecutable(t *testing.T) {
	t.Parallel()

	if _, err := NewFFmpeg(filepath.Join(t.TempDir(), "missing-ffmpeg")); err == nil {
		t.Fatal("NewFFmpeg() error = nil")
	}
}

func TestFFmpegStreamsCommandOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ncat\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "source.flac")
	if err := os.WriteFile(inputPath, []byte("audio-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	transcoder, err := NewFFmpeg(helper)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := transcoder.Transcode(context.Background(), ports.ResolvedSource{
		Content: input, Name: "Artist - Song.flac", ContentType: "audio/flac",
	}, ports.AudioTranscodeOptions{Format: "mp3", BitRate: 128})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resolved.Content)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolved.Content.Close(); err != nil {
		t.Fatal(err)
	}
	if string(data) != "audio-data" || resolved.Name != "Artist - Song.mp3" ||
		resolved.ContentType != "audio/mpeg" {
		t.Fatalf("transcoded source = name %q, type %q, data %q",
			resolved.Name, resolved.ContentType, data)
	}
}

func TestFFmpegRejectsUnsupportedOptions(t *testing.T) {
	t.Parallel()

	transcoder := &FFmpeg{path: "ffmpeg"}
	for _, options := range []ports.AudioTranscodeOptions{
		{Format: "flac"},
		{Format: "mp3", BitRate: 16},
		{Format: "mp3", BitRate: 321},
	} {
		source := ports.ResolvedSource{
			Content: &memorySource{Reader: strings.NewReader("audio")}, Name: "song.flac",
		}
		if _, err := transcoder.Transcode(context.Background(), source, options); err == nil {
			t.Fatalf("Transcode(%#v) error = nil", options)
		}
	}
}

type memorySource struct {
	*strings.Reader
}

func (*memorySource) Close() error { return nil }
