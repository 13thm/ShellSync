package service

import (
	"context"
	"errors"

	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/repository"
)

// allowedTaskStatus defines the legal status transitions.
var allowedTaskStatus = map[string]map[string]bool{
	"pending": {"running": true, "done": true},
	"running": {"paused": true, "done": true, "running": true},
	"paused":  {"running": true, "done": true},
	"done":    {"running": true},
}

// TaskService applies task business rules and emits change events.
type TaskService struct {
	repo   *repository.TaskRepo
	bus    *eventbus.Bus
	userID string
}

func (s *TaskService) Create(ctx context.Context, in repository.TaskCreate) (repository.Task, error) {
	in.UserID = s.userID
	t, err := s.repo.Create(ctx, in)
	if err != nil {
		return repository.Task{}, err
	}
	s.bus.Publish(taskEvent("created", t))
	return t, nil
}

func (s *TaskService) Get(ctx context.Context, id string) (repository.Task, error) {
	return s.repo.Get(ctx, id)
}

func (s *TaskService) List(ctx context.Context, f repository.TaskFilter) ([]repository.Task, error) {
	return s.repo.List(ctx, s.userID, f)
}

func (s *TaskService) Update(ctx context.Context, id string, p repository.TaskPatch) (repository.Task, error) {
	if p.Status != nil {
		cur, err := s.repo.Get(ctx, id)
		if err != nil {
			return repository.Task{}, err
		}
		if !allowedTaskStatus[cur.Status][*p.Status] {
			return repository.Task{}, ErrInvalidTransition
		}
	}
	t, err := s.repo.Update(ctx, id, p)
	if err != nil {
		return repository.Task{}, err
	}
	s.bus.Publish(taskEvent("updated", t))
	return t, nil
}

func (s *TaskService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.bus.Publish(deleteEvent("task", id))
	return nil
}

// Count returns the number of tasks for the user.
func (s *TaskService) Count(ctx context.Context) (int, error) {
	return s.repo.Count(ctx, s.userID)
}

// ErrTaskNotFound is returned when a task does not exist.
var ErrTaskNotFound = errors.New("task not found")
