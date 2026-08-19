package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type downloadControllerStub struct {
	items      []domain.TrackDownload
	cancelID   string
	retryID    string
	ownedID    string
	monitorErr error
}

func (s *downloadControllerStub) TrackDownloads(context.Context) ([]domain.TrackDownload, error) {
	return s.items, s.monitorErr
}

func (s *downloadControllerStub) CancelTrackDownload(_ context.Context, id string) error {
	if id != s.ownedID {
		return ports.ErrNotFound
	}
	s.cancelID = id
	return nil
}

func (s *downloadControllerStub) RetryTrackDownload(_ context.Context, id string) error {
	if id != s.ownedID {
		return ports.ErrNotFound
	}
	s.retryID = id
	return nil
}

func TestDownloadServiceCombinesSortsAndRoutesProviderJobs(t *testing.T) {
	t.Parallel()

	older := &downloadControllerStub{items: []domain.TrackDownload{{
		ID: "torrent-job", Name: "Zulu", StartedAt: time.Unix(1, 0),
	}}}
	newer := &downloadControllerStub{ownedID: "soulseek-job", items: []domain.TrackDownload{{
		ID: "soulseek-job", Name: "Alpha", StartedAt: time.Unix(2, 0),
	}}}
	service := NewDownloadService(older, newer)
	items, err := service.TrackDownloads(context.Background())
	if err != nil || len(items) != 2 || items[0].ID != "soulseek-job" {
		t.Fatalf("TrackDownloads() = %#v, %v", items, err)
	}
	if err := service.CancelTrackDownload(context.Background(), "soulseek-job"); err != nil {
		t.Fatal(err)
	}
	if err := service.RetryTrackDownload(context.Background(), "soulseek-job"); err != nil {
		t.Fatal(err)
	}
	if newer.cancelID != "soulseek-job" || newer.retryID != "soulseek-job" {
		t.Fatalf("cancel = %q, retry = %q", newer.cancelID, newer.retryID)
	}
}

func TestDownloadServiceReportsUnknownAndMonitorErrors(t *testing.T) {
	t.Parallel()

	service := NewDownloadService(&downloadControllerStub{})
	if err := service.CancelTrackDownload(context.Background(), "missing"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("CancelTrackDownload() error = %v", err)
	}
	want := errors.New("unavailable")
	service = NewDownloadService(&downloadControllerStub{monitorErr: want})
	if _, err := service.TrackDownloads(context.Background()); !errors.Is(err, want) {
		t.Fatalf("TrackDownloads() error = %v", err)
	}
}
