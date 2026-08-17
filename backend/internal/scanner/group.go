package scanner

import (
	"context"
	"fmt"
)

type Group struct {
	runners []Runner
}

func NewGroup(runners ...Runner) *Group {
	return &Group{runners: runners}
}

func (g *Group) Scan(ctx context.Context) (Report, error) {
	var combined Report
	for _, runner := range g.runners {
		report, err := runner.Scan(ctx)
		if err != nil {
			return Report{}, fmt.Errorf("scan music source: %w", err)
		}
		combined.Tracks += report.Tracks
		combined.Warnings = append(combined.Warnings, report.Warnings...)
	}
	return combined, nil
}
