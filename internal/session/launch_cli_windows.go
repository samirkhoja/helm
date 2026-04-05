//go:build windows

package session

import (
	"errors"
	"io"
	"time"
)

type LaunchWatchRequest struct {
	TargetPID     int
	ParentPID     int
	AppPID        int
	AppStartStamp string
}

type LaunchCLI struct {
	Stderr        io.Writer
	SelfPID       func() int
	ParentPID     func() int
	AppPID        func() int
	AppStartStamp func(int) (string, error)
	StartWatcher  func(LaunchWatchRequest) error
	Exec          func(path string, args []string, env []string) error
}

func (LaunchCLI) Run([]string) error {
	return errors.New("session-launch is not supported on windows")
}

type SessionWatchCLI struct {
	Stderr        io.Writer
	PollInterval  time.Duration
	TerminateWait time.Duration
	Inspector     ProcessInspector
}

type ProcessInspector interface {
	ProcessExists(pid int) (bool, error)
	ProcessParentPID(pid int) (int, error)
	ProcessStartStamp(pid int) (string, error)
	TerminateProcessGroup(pid int, gracePeriod time.Duration) error
}

func (SessionWatchCLI) Run([]string) error {
	return errors.New("session-watch is not supported on windows")
}
