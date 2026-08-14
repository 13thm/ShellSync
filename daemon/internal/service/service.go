// Package service implements domain logic on top of the repository.
package service

import (
	"errors"

	"github.com/shellsync/daemon/internal/eventbus"
	"github.com/shellsync/daemon/internal/logstore"
	"github.com/shellsync/daemon/internal/repository"
	"github.com/shellsync/daemon/internal/terminal"
)

// Domain errors.
var (
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrInvalidPairing    = errors.New("invalid or expired pairing code")
)

// Deps bundles the repositories and managers the services need.
type Deps struct {
	UserID       string
	TaskRepo     *repository.TaskRepo
	TerminalRepo *repository.TerminalRepo
	TodoRepo     *repository.TodoRepo
	LogRepo      *repository.LogRepo
	DeviceRepo   *repository.DeviceRepo
	SettingsRepo *repository.SettingsRepo
	TermMgr      *terminal.Manager
	LogMgr       *logstore.Manager
	Bus          *eventbus.Bus
}

// Services is the set of domain services constructed from Deps.
type Services struct {
	Tasks     *TaskService
	Todos     *TodoService
	Terminals *TerminalService
	Pair      *PairService
	Settings  *SettingsService
	Devices   *DeviceService
}

// New builds all services from the given dependencies.
func New(d Deps) *Services {
	return &Services{
		Tasks:     &TaskService{repo: d.TaskRepo, bus: d.Bus, userID: d.UserID},
		Todos:     &TodoService{repo: d.TodoRepo, bus: d.Bus, userID: d.UserID},
		Terminals: &TerminalService{mgr: d.TermMgr, repo: d.TerminalRepo, logMgr: d.LogMgr, bus: d.Bus, userID: d.UserID},
		Pair:      &PairService{deviceRepo: d.DeviceRepo, userID: d.UserID, codes: map[string]pairCode{}},
		Settings:  &SettingsService{repo: d.SettingsRepo},
		Devices:   &DeviceService{repo: d.DeviceRepo, bus: d.Bus},
	}
}

func deviceEvent(action string, d repository.Device) eventbus.Event {
	return eventbus.Event{Type: "device." + action, Entity: "device", Action: action, Payload: d}
}

func taskEvent(action string, t repository.Task) eventbus.Event {
	return eventbus.Event{Type: "task." + action, Entity: "task", Action: action, Payload: t}
}

func todoEvent(action string, t repository.Todo) eventbus.Event {
	return eventbus.Event{Type: "todo." + action, Entity: "todo", Action: action, Payload: t}
}

func terminalEvent(action string, t repository.Terminal) eventbus.Event {
	return eventbus.Event{Type: "terminal." + action, Entity: "terminal", Action: action, Payload: t}
}

func deleteEvent(entity, id string) eventbus.Event {
	return eventbus.Event{
		Type: entity + ".deleted", Entity: entity, Action: "deleted",
		Payload: map[string]string{"id": id},
	}
}
