package services

import (
	"context"
	"errors"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type transferSettingsStub struct {
	stored    domain.TransferLimits
	current   domain.TransferLimits
	loadErr   error
	saveErr   error
	applyErr  error
	storeMiss bool
}

func (s *transferSettingsStub) TransferLimits(context.Context) (domain.TransferLimits, error) {
	if s.storeMiss {
		return domain.TransferLimits{}, ports.ErrNotFound
	}
	return s.stored, s.loadErr
}

func (s *transferSettingsStub) SaveTransferLimits(_ context.Context, limits domain.TransferLimits) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.stored = limits
	return nil
}

type transferControllerStub struct{ settings *transferSettingsStub }

func (s transferControllerStub) TransferLimits(context.Context) (domain.TransferLimits, error) {
	return s.settings.current, nil
}

func (s transferControllerStub) SetTransferLimits(_ context.Context, limits domain.TransferLimits) error {
	if s.settings.applyErr != nil {
		return s.settings.applyErr
	}
	s.settings.current = limits
	return nil
}

func TestTransferSettingsInitializeAndUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := &transferSettingsStub{
		stored:  domain.TransferLimits{UploadBytesPerSecond: 1024, DownloadBytesPerSecond: 2048},
		current: domain.TransferLimits{UploadBytesPerSecond: 1, DownloadBytesPerSecond: 2},
	}
	service := NewTransferSettingsService(stub, transferControllerStub{settings: stub})
	if err := service.Initialize(ctx); err != nil || stub.current != stub.stored {
		t.Fatalf("Initialize() current = %#v, error = %v", stub.current, err)
	}
	admin := domain.User{Username: "admin", Role: domain.UserRoleAdmin}
	want := domain.TransferLimits{UploadBytesPerSecond: 4096, DownloadBytesPerSecond: 8192}
	got, err := service.UpdateLimits(ctx, admin, want)
	if err != nil || got != want || stub.current != want || stub.stored != want {
		t.Fatalf("UpdateLimits() = %#v, current = %#v, stored = %#v, error = %v", got, stub.current, stub.stored, err)
	}
}

func TestTransferSettingsProtectAndValidateUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := &transferSettingsStub{storeMiss: true}
	service := NewTransferSettingsService(stub, transferControllerStub{settings: stub})
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	user := domain.User{Username: "listener", Role: domain.UserRoleUser}
	if _, err := service.UpdateLimits(ctx, user, domain.TransferLimits{}); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("unauthorized UpdateLimits() error = %v", err)
	}
	admin := domain.User{Username: "admin", Role: domain.UserRoleAdmin}
	if _, err := service.UpdateLimits(ctx, admin, domain.TransferLimits{UploadBytesPerSecond: -1}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("invalid UpdateLimits() error = %v", err)
	}
}

func TestTransferSettingsRollBackRuntimeLimitWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	previous := domain.TransferLimits{UploadBytesPerSecond: 100, DownloadBytesPerSecond: 200}
	stub := &transferSettingsStub{current: previous, saveErr: errors.New("disk full")}
	service := NewTransferSettingsService(stub, transferControllerStub{settings: stub})
	admin := domain.User{Username: "admin", Role: domain.UserRoleAdmin}
	if _, err := service.UpdateLimits(ctx, admin, domain.TransferLimits{
		UploadBytesPerSecond: 300, DownloadBytesPerSecond: 400,
	}); err == nil {
		t.Fatal("UpdateLimits() error = nil")
	}
	if stub.current != previous {
		t.Fatalf("runtime settings after failed save = %#v, want %#v", stub.current, previous)
	}
}
