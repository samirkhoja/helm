//go:build !windows

package session

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultSessionWatchPollInterval = 500 * time.Millisecond
	defaultSessionWatchGracePeriod  = 1 * time.Second
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

func (cli LaunchCLI) Run(args []string) error {
	if cli.Stderr == nil {
		cli.Stderr = os.Stderr
	}
	if len(args) == 0 || isLaunchHelp(args[0]) {
		cli.writeUsage()
		return nil
	}

	command := launchCommandArgs(args)
	if len(command) == 0 {
		return errors.New("session-launch requires a command after --")
	}

	selfPID := os.Getpid
	if cli.SelfPID != nil {
		selfPID = cli.SelfPID
	}
	parentPID := os.Getppid
	if cli.ParentPID != nil {
		parentPID = cli.ParentPID
	}
	appPID := launchAppPIDFromEnv
	if cli.AppPID != nil {
		appPID = cli.AppPID
	}
	appStartStamp := processStartStamp
	if cli.AppStartStamp != nil {
		appStartStamp = cli.AppStartStamp
	}
	startWatcher := startSessionWatcher
	if cli.StartWatcher != nil {
		startWatcher = cli.StartWatcher
	}
	execCommand := execSessionCommand
	if cli.Exec != nil {
		execCommand = cli.Exec
	}

	request := LaunchWatchRequest{
		TargetPID: selfPID(),
		ParentPID: parentPID(),
		AppPID:    appPID(),
	}
	if request.AppPID > 0 {
		stamp, err := appStartStamp(request.AppPID)
		if err == nil {
			request.AppStartStamp = stamp
		}
	}

	if err := startWatcher(request); err != nil {
		return err
	}
	return execCommand(command[0], command, os.Environ())
}

func (cli LaunchCLI) writeUsage() {
	_, _ = fmt.Fprint(cli.Stderr, "Usage:\n")
	_, _ = fmt.Fprint(cli.Stderr, "  helm session-launch -- <command> [args...]\n")
}

func launchCommandArgs(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return append([]string(nil), args[1:]...)
	}
	return append([]string(nil), args...)
}

func isLaunchHelp(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func launchAppPIDFromEnv() int {
	value := strings.TrimSpace(os.Getenv("HELM_APP_PID"))
	if value == "" {
		return 0
	}
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func startSessionWatcher(request LaunchWatchRequest) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve helm executable path: %w", err)
	}

	args := []string{
		"session-watch",
		"--target-pid", strconv.Itoa(request.TargetPID),
		"--parent-pid", strconv.Itoa(request.ParentPID),
	}
	if request.AppPID > 0 && strings.TrimSpace(request.AppStartStamp) != "" {
		args = append(args,
			"--app-pid", strconv.Itoa(request.AppPID),
			"--app-start", request.AppStartStamp,
		)
	}

	cmd := exec.Command(executablePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func execSessionCommand(path string, args []string, env []string) error {
	return syscall.Exec(path, args, env)
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

func (cli SessionWatchCLI) Run(args []string) error {
	if cli.Stderr == nil {
		cli.Stderr = os.Stderr
	}

	fs := flag.NewFlagSet("session-watch", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	targetPID := fs.Int("target-pid", 0, "pid of the launched command")
	parentPID := fs.Int("parent-pid", 0, "expected parent shell pid")
	appPID := fs.Int("app-pid", 0, "helm app pid")
	appStart := fs.String("app-start", "", "expected helm app start stamp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *targetPID <= 0 {
		return errors.New("session-watch requires --target-pid")
	}

	inspector := cli.Inspector
	if inspector == nil {
		inspector = osProcessInspector{}
	}

	pollInterval := cli.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultSessionWatchPollInterval
	}
	terminateWait := cli.TerminateWait
	if terminateWait <= 0 {
		terminateWait = defaultSessionWatchGracePeriod
	}

	for {
		exists, err := inspector.ProcessExists(*targetPID)
		if err != nil || !exists {
			return nil
		}
		terminate, err := shouldTerminateSessionTarget(inspector, *targetPID, *parentPID, *appPID, *appStart)
		if err != nil || terminate {
			_ = inspector.TerminateProcessGroup(*targetPID, terminateWait)
			return nil
		}
		time.Sleep(pollInterval)
	}
}

func shouldTerminateSessionTarget(inspector ProcessInspector, targetPID, expectedParentPID, appPID int, expectedAppStart string) (bool, error) {
	if expectedParentPID > 0 {
		parentPID, err := inspector.ProcessParentPID(targetPID)
		if err != nil {
			return true, err
		}
		if parentPID != expectedParentPID {
			return true, nil
		}
	}

	expectedAppStart = strings.TrimSpace(expectedAppStart)
	if appPID > 0 && expectedAppStart != "" {
		appStart, err := inspector.ProcessStartStamp(appPID)
		if err != nil {
			return true, err
		}
		if strings.TrimSpace(appStart) != expectedAppStart {
			return true, nil
		}
	}

	return false, nil
}

type osProcessInspector struct{}

func (osProcessInspector) ProcessExists(pid int) (bool, error) {
	err := syscall.Kill(pid, syscall.Signal(0))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, err
	}
}

func (osProcessInspector) ProcessParentPID(pid int) (int, error) {
	value, err := psField(pid, "ppid=")
	if err != nil {
		return 0, err
	}
	parentPID, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse parent pid %q: %w", value, err)
	}
	return parentPID, nil
}

func (osProcessInspector) ProcessStartStamp(pid int) (string, error) {
	return processStartStamp(pid)
}

func (osProcessInspector) TerminateProcessGroup(pid int, gracePeriod time.Duration) error {
	if pid <= 0 {
		return nil
	}

	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)

	if gracePeriod > 0 {
		time.Sleep(gracePeriod)
	}

	exists, err := osProcessInspector{}.ProcessExists(pid)
	if err != nil || !exists {
		return nil
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return nil
}

func processStartStamp(pid int) (string, error) {
	return psField(pid, "lstart=")
}

func psField(pid int, field string) (string, error) {
	output, err := exec.Command("ps", "-o", field, "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("ps %s returned no value for pid %d", field, pid)
	}
	return value, nil
}
