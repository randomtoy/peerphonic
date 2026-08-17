package metadata

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/scanner"
	"github.com/tcolgate/mp3"
)

func populateAudioProperties(file *os.File, extension string, result *scanner.Metadata) error {
	if extension != ".mp3" {
		return nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind for audio inspection: %w", err)
	}
	duration, bitRate, err := inspectMP3(file)
	if err != nil {
		return fmt.Errorf("inspect mp3: %w", err)
	}
	result.Duration = duration
	result.BitRate = bitRate
	return nil
}

func inspectMP3(source io.Reader) (time.Duration, int, error) {
	decoder := mp3.NewDecoder(source)
	var frame mp3.Frame
	var duration time.Duration
	var weightedBitRate int64
	frames := 0

	for {
		var skipped int
		err := decoder.Decode(&frame, &skipped)
		if err != nil {
			if errors.Is(err, io.EOF) || frames > 0 {
				break
			}
			return 0, 0, err
		}
		frameDuration := frame.Duration()
		bitRate := int(frame.Header().BitRate())
		if frameDuration <= 0 || bitRate <= 0 {
			continue
		}
		duration += frameDuration
		weightedBitRate += int64(bitRate) * frameDuration.Nanoseconds()
		frames++
	}
	if frames == 0 || duration <= 0 {
		return 0, 0, errors.New("no MPEG audio frames found")
	}
	averageKbps := weightedBitRate / duration.Nanoseconds() / 1000
	return duration, int(averageKbps), nil
}
