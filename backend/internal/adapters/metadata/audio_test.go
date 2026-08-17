package metadata

import (
	"bytes"
	"testing"
	"time"

	"github.com/tcolgate/mp3"
)

func TestInspectMP3(t *testing.T) {
	t.Parallel()

	duration, bitRate, err := inspectMP3(bytes.NewReader(bytes.Repeat(mp3.SilentBytes, 20)))
	if err != nil {
		t.Fatalf("inspectMP3() error = %v", err)
	}
	wantDuration := 20 * mp3.SilentFrame.Duration()
	if difference := duration - wantDuration; difference < -time.Millisecond || difference > time.Millisecond {
		t.Fatalf("duration = %v, want %v", duration, wantDuration)
	}
	wantBitRate := int(mp3.SilentFrame.Header().BitRate()) / 1000
	if bitRate != wantBitRate {
		t.Fatalf("bitRate = %d, want %d", bitRate, wantBitRate)
	}
}

func TestInspectMP3RejectsNonAudio(t *testing.T) {
	t.Parallel()

	if _, _, err := inspectMP3(bytes.NewReader([]byte("not audio"))); err == nil {
		t.Fatal("inspectMP3() error = nil, want invalid audio error")
	}
}
