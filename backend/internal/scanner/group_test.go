package scanner

import (
	"context"
	"testing"
)

func TestGroupCombinesSourceReports(t *testing.T) {
	t.Parallel()

	group := NewGroup(
		&runnerStub{report: Report{Tracks: 2, Warnings: []Warning{{Path: "local"}}}},
		&runnerStub{report: Report{Tracks: 3, Warnings: []Warning{{Path: "torrent"}}}},
	)
	report, err := group.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Tracks != 5 || len(report.Warnings) != 2 {
		t.Fatalf("Scan() = %#v", report)
	}
}
