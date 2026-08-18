package scanner

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrScanInProgress = errors.New("music scan is already in progress")

type Runner interface {
	Scan(ctx context.Context) (Report, error)
}

type Status struct {
	Scanning       bool
	Count          int
	LastError      string
	LastStartedAt  time.Time
	LastFinishedAt time.Time
}

type Manager struct {
	ctx    context.Context
	runner Runner
	mu     sync.RWMutex
	status Status
}

func NewManager(ctx context.Context, runner Runner) *Manager {
	return &Manager{ctx: ctx, runner: runner}
}

func (m *Manager) ScanNow(ctx context.Context) (Report, error) {
	if !m.begin() {
		return Report{}, ErrScanInProgress
	}
	report, err := m.runner.Scan(ctx)
	m.finish(report, err)
	return report, err
}

func (m *Manager) Start() bool {
	if !m.begin() {
		return false
	}
	go func() {
		report, err := m.runner.Scan(m.ctx)
		m.finish(report, err)
	}()
	return true
}

// StartPeriodic schedules scans until the manager context is cancelled. A tick
// is skipped when another scan is still running.
func (m *Manager) StartPeriodic(interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.Start()
			}
		}
	}()
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) begin() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Scanning {
		return false
	}
	m.status.Scanning = true
	m.status.LastError = ""
	m.status.LastStartedAt = time.Now().UTC()
	return true
}

func (m *Manager) finish(report Report, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Scanning = false
	m.status.LastFinishedAt = time.Now().UTC()
	if err != nil {
		m.status.LastError = err.Error()
		return
	}
	m.status.Count = report.Tracks
}
