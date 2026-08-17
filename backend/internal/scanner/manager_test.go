package scanner

import (
	"context"
	"errors"
	"testing"
	"time"
)

type runnerStub struct {
	started chan struct{}
	release chan struct{}
	report  Report
	err     error
}

func (r *runnerStub) Scan(ctx context.Context) (Report, error) {
	if r.started != nil {
		close(r.started)
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return Report{}, ctx.Err()
		}
	}
	return r.report, r.err
}

func TestManagerScanNowRecordsStatus(t *testing.T) {
	t.Parallel()

	manager := NewManager(context.Background(), &runnerStub{report: Report{Tracks: 42}})
	report, err := manager.ScanNow(context.Background())
	if err != nil || report.Tracks != 42 {
		t.Fatalf("ScanNow() = %#v, %v", report, err)
	}
	status := manager.Status()
	if status.Scanning || status.Count != 42 || status.LastError != "" {
		t.Fatalf("Status() = %#v", status)
	}
}

func TestManagerPreventsConcurrentScans(t *testing.T) {
	t.Parallel()

	runner := &runnerStub{started: make(chan struct{}), release: make(chan struct{}), report: Report{Tracks: 7}}
	manager := NewManager(context.Background(), runner)
	if !manager.Start() {
		t.Fatal("Start() = false, want true")
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("background scan did not start")
	}
	if manager.Start() {
		t.Fatal("second Start() = true, want false")
	}
	if _, err := manager.ScanNow(context.Background()); !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("ScanNow() error = %v, want ErrScanInProgress", err)
	}
	close(runner.release)
	deadline := time.Now().Add(time.Second)
	for manager.Status().Scanning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := manager.Status()
	if status.Scanning || status.Count != 7 {
		t.Fatalf("Status() after completion = %#v", status)
	}
}
