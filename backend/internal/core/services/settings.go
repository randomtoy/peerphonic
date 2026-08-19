package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrInvalidSettings = errors.New("invalid settings")

type TransferSettingsService struct {
	store      ports.TransferLimitStore
	controller ports.TransferLimitController
}

func NewTransferSettingsService(
	store ports.TransferLimitStore, controller ports.TransferLimitController,
) *TransferSettingsService {
	return &TransferSettingsService{store: store, controller: controller}
}

func (s *TransferSettingsService) Initialize(ctx context.Context) error {
	limits, err := s.store.TransferLimits(ctx)
	if errors.Is(err, ports.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load transfer settings: %w", err)
	}
	if err := s.controller.SetTransferLimits(ctx, limits); err != nil {
		return fmt.Errorf("apply stored transfer settings: %w", err)
	}
	return nil
}

func (s *TransferSettingsService) Limits(
	ctx context.Context, actor domain.User,
) (domain.TransferLimits, error) {
	if !actor.HasPermission(domain.PermissionSourcesManage) {
		return domain.TransferLimits{}, ports.ErrForbidden
	}
	return s.controller.TransferLimits(ctx)
}

func (s *TransferSettingsService) UpdateLimits(
	ctx context.Context, actor domain.User, limits domain.TransferLimits,
) (domain.TransferLimits, error) {
	if !actor.HasPermission(domain.PermissionSourcesManage) {
		return domain.TransferLimits{}, ports.ErrForbidden
	}
	if limits.UploadBytesPerSecond < 0 || limits.DownloadBytesPerSecond < 0 {
		return domain.TransferLimits{}, fmt.Errorf("%w: transfer limits must be non-negative", ErrInvalidSettings)
	}
	previous, err := s.controller.TransferLimits(ctx)
	if err != nil {
		return domain.TransferLimits{}, fmt.Errorf("read current transfer limits: %w", err)
	}
	if err := s.controller.SetTransferLimits(ctx, limits); err != nil {
		return domain.TransferLimits{}, fmt.Errorf("apply transfer limits: %w", err)
	}
	if err := s.store.SaveTransferLimits(ctx, limits); err != nil {
		_ = s.controller.SetTransferLimits(context.WithoutCancel(ctx), previous)
		return domain.TransferLimits{}, fmt.Errorf("save transfer limits: %w", err)
	}
	return limits, nil
}
