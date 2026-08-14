package service

import (
	"context"

	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/repository"
)

// DeviceService manages paired devices.
type DeviceService struct {
	repo *repository.DeviceRepo
	bus  *eventbus.Bus
}

func (s *DeviceService) List(ctx context.Context, userID string) ([]repository.Device, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *DeviceService) Revoke(ctx context.Context, id string) error {
	if err := s.repo.Revoke(ctx, id); err != nil {
		return err
	}
	if d, err := s.repo.Get(ctx, id); err == nil {
		s.bus.Publish(deviceEvent("updated", d))
	}
	return nil
}

// Delete removes the device record entirely (revoking first if needed).
func (s *DeviceService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.bus.Publish(deleteEvent("device", id))
	return nil
}
