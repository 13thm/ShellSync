package service

import (
	"context"

	"github.com/shellsync/daemon/internal/repository"
)

// DeviceService manages paired devices.
type DeviceService struct {
	repo *repository.DeviceRepo
}

func (s *DeviceService) List(ctx context.Context, userID string) ([]repository.Device, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *DeviceService) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id)
}
