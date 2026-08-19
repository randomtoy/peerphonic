package soulseek

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type growingFile struct {
	job      *downloadJob
	size     int64
	offset   int64
	closed   chan struct{}
	closeOne sync.Once
	mu       sync.Mutex
}

func newGrowingFile(job *downloadJob, size int64) *growingFile {
	return &growingFile{job: job, size: size, closed: make(chan struct{})}
}

func (f *growingFile) Read(buffer []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if f.offset >= f.size {
			return 0, io.EOF
		}
		var openErr error
		for _, path := range []string{f.job.finalPath, f.job.incompletePath} {
			file, err := os.Open(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				openErr = err
				continue
			}
			_, seekErr := file.Seek(f.offset, io.SeekStart)
			readBuffer := buffer
			if remaining := f.size - f.offset; int64(len(readBuffer)) > remaining {
				readBuffer = readBuffer[:remaining]
			}
			n, readErr := file.Read(readBuffer)
			closeErr := file.Close()
			if seekErr != nil {
				return 0, fmt.Errorf("seek growing Soulseek file: %w", seekErr)
			}
			if n > 0 {
				f.offset += int64(n)
				return n, nil
			}
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return 0, fmt.Errorf("read growing Soulseek file: %w", readErr)
			}
			if closeErr != nil {
				return 0, fmt.Errorf("close growing Soulseek file: %w", closeErr)
			}
		}
		select {
		case <-f.closed:
			return 0, os.ErrClosed
		case <-f.job.done:
			if err := f.job.result(); err != nil {
				return 0, err
			}
			if f.offset >= f.size {
				return 0, io.EOF
			}
			if openErr != nil {
				return 0, fmt.Errorf("open completed Soulseek file: %w", openErr)
			}
			if _, err := os.Stat(f.job.finalPath); err != nil {
				return 0, fmt.Errorf("open completed Soulseek file: %w", err)
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (f *growingFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = f.offset + offset
	case io.SeekEnd:
		next = f.size + offset
	default:
		return 0, errors.New("invalid seek origin")
	}
	if next < 0 {
		return 0, errors.New("negative seek position")
	}
	f.offset = next
	return next, nil
}

func (f *growingFile) Close() error {
	f.closeOne.Do(func() { close(f.closed) })
	return nil
}
