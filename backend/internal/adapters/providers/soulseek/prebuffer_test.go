package soulseek

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForInitialDataHonorsConfiguredPrebuffer(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "track.part")
	job := &downloadJob{finalPath: path, incompletePath: path + ".missing", done: make(chan struct{})}
	if err := os.WriteFile(path, []byte("12"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err == nil {
			_, _ = file.WriteString("34")
			_ = file.Close()
		}
	}()
	started := time.Now()
	if err := waitForInitialData(context.Background(), job, time.Second, 4); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 75*time.Millisecond {
		t.Fatalf("prebuffer returned after %v before threshold was written", elapsed)
	}
}
