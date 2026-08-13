package service

import (
	"context"

	"github.com/shellsync/daemon/internal/repository"
)

// SettingsService wraps the settings KV store.
type SettingsService struct {
	repo *repository.SettingsRepo
}

func (s *SettingsService) GetAll(ctx context.Context) (map[string]string, error) {
	return s.repo.GetAll(ctx)
}

func (s *SettingsService) Get(ctx context.Context, key string) (string, bool, error) {
	return s.repo.Get(ctx, key)
}

func (s *SettingsService) Set(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
}

func (s *SettingsService) Delete(ctx context.Context, key string) error {
	return s.repo.Delete(ctx, key)
}

// Patch applies a partial settings update.
func (s *SettingsService) Patch(ctx context.Context, kv map[string]string) error {
	for k, v := range kv {
		if err := s.repo.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}
