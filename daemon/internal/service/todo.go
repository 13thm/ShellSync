package service

import (
	"context"

	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/repository"
)

// TodoService applies todo business rules and emits change events.
type TodoService struct {
	repo   *repository.TodoRepo
	bus    *eventbus.Bus
	userID string
}

func (s *TodoService) Create(ctx context.Context, in repository.TodoCreate) (repository.Todo, error) {
	in.UserID = s.userID
	t, err := s.repo.Create(ctx, in)
	if err != nil {
		return repository.Todo{}, err
	}
	s.bus.Publish(todoEvent("created", t))
	return t, nil
}

func (s *TodoService) Get(ctx context.Context, id string) (repository.Todo, error) {
	return s.repo.Get(ctx, id)
}

func (s *TodoService) List(ctx context.Context, f repository.TodoFilter) ([]repository.Todo, error) {
	return s.repo.List(ctx, s.userID, f)
}

func (s *TodoService) Update(ctx context.Context, id string, p repository.TodoPatch) (repository.Todo, error) {
	t, err := s.repo.Update(ctx, id, p)
	if err != nil {
		return repository.Todo{}, err
	}
	s.bus.Publish(todoEvent("updated", t))
	return t, nil
}

func (s *TodoService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.bus.Publish(deleteEvent("todo", id))
	return nil
}
