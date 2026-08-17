package scanner

import (
	"context"
	"errors"
	"sync"
)

var ErrScanInProgress = errors.New("music scan is already in progress")

type Runner interface {
	Scan(ctx context.Context) (Report, error)
}

type Status struct {
	Scanning  bool
	Count     int
	LastError string
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
	return true
}

func (m *Manager) finish(report Report, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Scanning = false
	if err != nil {
		m.status.LastError = err.Error()
		return
	}
	m.status.Count = report.Tracks
}
