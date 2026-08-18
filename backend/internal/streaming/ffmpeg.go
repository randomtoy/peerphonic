package streaming

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

const defaultMP3BitRate = 192

type FFmpeg struct {
	path string
}

func NewFFmpeg(path string) (*FFmpeg, error) {
	resolved, err := exec.LookPath(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("find FFmpeg executable: %w", err)
	}
	return &FFmpeg{path: resolved}, nil
}

func (f *FFmpeg) Transcode(
	ctx context.Context, source ports.ResolvedSource, options ports.AudioTranscodeOptions,
) (ports.TranscodedSource, error) {
	if !strings.EqualFold(strings.TrimSpace(options.Format), "mp3") {
		return ports.TranscodedSource{}, fmt.Errorf("unsupported transcode format %q", options.Format)
	}
	bitRate := options.BitRate
	if bitRate == 0 {
		bitRate = defaultMP3BitRate
	}
	if bitRate < 32 || bitRate > 320 {
		return ports.TranscodedSource{}, fmt.Errorf("MP3 bitrate must be between 32 and 320 kbps")
	}

	command := exec.CommandContext(ctx, f.path,
		"-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-map", "0:a:0", "-vn",
		"-codec:a", "libmp3lame", "-b:a", strconv.Itoa(bitRate)+"k", "-f", "mp3", "pipe:1",
	)
	command.Stdin = source.Content
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return ports.TranscodedSource{}, fmt.Errorf("open FFmpeg output: %w", err)
	}
	if err := command.Start(); err != nil {
		_ = stdout.Close()
		return ports.TranscodedSource{}, fmt.Errorf("start FFmpeg: %w", err)
	}
	reader := &processReader{
		stdout: stdout, source: source.Content, command: command, stderr: &stderr,
		done: make(chan struct{}),
	}
	go reader.wait()
	return ports.TranscodedSource{
		Content: reader, Name: replaceExtension(source.Name, ".mp3"), ContentType: "audio/mpeg",
	}, nil
}

type processReader struct {
	stdout  io.ReadCloser
	source  ports.ReadSeekCloser
	command *exec.Cmd
	stderr  *bytes.Buffer
	done    chan struct{}

	closeOnce sync.Once
	mu        sync.Mutex
	waitErr   error
}

func (r *processReader) wait() {
	err := r.command.Wait()
	if err != nil {
		message := strings.TrimSpace(r.stderr.String())
		if message != "" {
			err = fmt.Errorf("%w: %s", err, message)
		}
	}
	r.mu.Lock()
	r.waitErr = err
	r.mu.Unlock()
	close(r.done)
}

func (r *processReader) Read(buffer []byte) (int, error) {
	n, err := r.stdout.Read(buffer)
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	<-r.done
	r.mu.Lock()
	waitErr := r.waitErr
	r.mu.Unlock()
	if waitErr != nil {
		return n, fmt.Errorf("transcode audio with FFmpeg: %w", waitErr)
	}
	return n, io.EOF
}

func (r *processReader) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		stdoutErr := r.stdout.Close()
		if errors.Is(stdoutErr, os.ErrClosed) {
			stdoutErr = nil
		}
		sourceErr := r.source.Close()
		if errors.Is(sourceErr, os.ErrClosed) {
			sourceErr = nil
		}
		select {
		case <-r.done:
		default:
			if r.command.Process != nil {
				if err := r.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					closeErr = errors.Join(closeErr, err)
				}
			}
			<-r.done
		}
		closeErr = errors.Join(closeErr, stdoutErr, sourceErr)
	})
	return closeErr
}

func replaceExtension(name, extension string) string {
	current := filepath.Ext(name)
	if current == "" {
		return name + extension
	}
	return strings.TrimSuffix(name, current) + extension
}
