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
	CurrentTrigger string
	LastStartedAt  time.Time
	LastFinishedAt time.Time
	History        []Run
}

type Run struct {
	Trigger    string
	Tracks     int
	Warnings   []string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
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
	if !m.begin("synchronous") {
		return Report{}, ErrScanInProgress
	}
	report, err := m.runner.Scan(ctx)
	m.finish(report, err)
	return report, err
}

func (m *Manager) Start() bool {
	return m.start("manual")
}

func (m *Manager) start(trigger string) bool {
	if !m.begin(trigger) {
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
				m.start("periodic")
			}
		}
	}()
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.History = append([]Run(nil), m.status.History...)
	for index := range status.History {
		status.History[index].Warnings = append([]string(nil), status.History[index].Warnings...)
	}
	return status
}

func (m *Manager) begin(trigger string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Scanning {
		return false
	}
	m.status.Scanning = true
	m.status.LastError = ""
	m.status.CurrentTrigger = trigger
	m.status.LastStartedAt = time.Now().UTC()
	return true
}

func (m *Manager) finish(report Report, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Scanning = false
	m.status.LastFinishedAt = time.Now().UTC()
	run := Run{
		Trigger: m.status.CurrentTrigger, Tracks: report.Tracks,
		StartedAt: m.status.LastStartedAt, FinishedAt: m.status.LastFinishedAt,
	}
	for _, warning := range report.Warnings {
		if len(run.Warnings) == 20 {
			break
		}
		run.Warnings = append(run.Warnings, warning.Path+": "+warning.Err.Error())
	}
	if err != nil {
		m.status.LastError = err.Error()
		run.Error = err.Error()
	} else {
		m.status.Count = report.Tracks
	}
	m.status.History = append([]Run{run}, m.status.History...)
	if len(m.status.History) > 20 {
		m.status.History = m.status.History[:20]
	}
}
