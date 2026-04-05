package session

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestLaunchCLIRunStartsWatcherAndExecsCommand(t *testing.T) {
	t.Setenv("HELM_APP_PID", "321")

	var watched LaunchWatchRequest
	var execPath string
	var execArgs []string
	var execEnv []string
	execErr := errors.New("stop after exec")

	cli := LaunchCLI{
		SelfPID:   func() int { return 111 },
		ParentPID: func() int { return 222 },
		AppPID:    func() int { return 321 },
		AppStartStamp: func(pid int) (string, error) {
			if pid != 321 {
				t.Fatalf("AppStartStamp pid = %d, want 321", pid)
			}
			return "stamp", nil
		},
		StartWatcher: func(request LaunchWatchRequest) error {
			watched = request
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			execPath = path
			execArgs = append([]string(nil), args...)
			execEnv = append([]string(nil), env...)
			return execErr
		},
	}

	err := cli.Run([]string{"--", "/bin/echo", "hello"})
	if !errors.Is(err, execErr) {
		t.Fatalf("Run() error = %v, want %v", err, execErr)
	}
	if watched.TargetPID != 111 || watched.ParentPID != 222 || watched.AppPID != 321 || watched.AppStartStamp != "stamp" {
		t.Fatalf("watch request = %#v, want populated launch metadata", watched)
	}
	if execPath != "/bin/echo" {
		t.Fatalf("exec path = %q, want /bin/echo", execPath)
	}
	if !reflect.DeepEqual(execArgs, []string{"/bin/echo", "hello"}) {
		t.Fatalf("exec args = %#v, want [/bin/echo hello]", execArgs)
	}
	if len(execEnv) == 0 {
		t.Fatalf("exec env = %#v, want inherited environment", execEnv)
	}
}

func TestLaunchCLIRunRejectsMissingCommand(t *testing.T) {
	cli := LaunchCLI{}
	if err := cli.Run([]string{"--"}); err == nil {
		t.Fatalf("Run() error = nil, want missing command error")
	}
}

func TestShouldTerminateSessionTargetWhenParentChanges(t *testing.T) {
	inspector := &fakeProcessInspector{
		parentPID: 1,
	}

	terminate, err := shouldTerminateSessionTarget(inspector, 100, 200, 0, "")
	if err != nil {
		t.Fatalf("shouldTerminateSessionTarget() error = %v", err)
	}
	if !terminate {
		t.Fatalf("shouldTerminateSessionTarget() = false, want true when parent pid changes")
	}
}

func TestShouldTerminateSessionTargetWhenAppStartChanges(t *testing.T) {
	inspector := &fakeProcessInspector{
		parentPID: 200,
		appStart:  "new-stamp",
	}

	terminate, err := shouldTerminateSessionTarget(inspector, 100, 200, 300, "old-stamp")
	if err != nil {
		t.Fatalf("shouldTerminateSessionTarget() error = %v", err)
	}
	if !terminate {
		t.Fatalf("shouldTerminateSessionTarget() = false, want true when app start stamp changes")
	}
}

func TestSessionWatchCLITerminatesTargetGroup(t *testing.T) {
	inspector := &fakeProcessInspector{
		existsSequence: []bool{true},
		parentPID:      1,
	}

	cli := SessionWatchCLI{
		PollInterval:  time.Millisecond,
		TerminateWait: 0,
		Inspector:     inspector,
	}

	if err := cli.Run([]string{"--target-pid", "100", "--parent-pid", "200"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !inspector.terminated {
		t.Fatalf("TerminateProcessGroup() was not called")
	}
	if inspector.terminatedPID != 100 {
		t.Fatalf("terminated pid = %d, want 100", inspector.terminatedPID)
	}
}

type fakeProcessInspector struct {
	existsSequence []bool
	parentPID      int
	appStart       string
	terminated     bool
	terminatedPID  int
}

func (f *fakeProcessInspector) ProcessExists(pid int) (bool, error) {
	if len(f.existsSequence) == 0 {
		return true, nil
	}
	value := f.existsSequence[0]
	f.existsSequence = f.existsSequence[1:]
	return value, nil
}

func (f *fakeProcessInspector) ProcessParentPID(pid int) (int, error) {
	return f.parentPID, nil
}

func (f *fakeProcessInspector) ProcessStartStamp(pid int) (string, error) {
	return f.appStart, nil
}

func (f *fakeProcessInspector) TerminateProcessGroup(pid int, gracePeriod time.Duration) error {
	f.terminated = true
	f.terminatedPID = pid
	return nil
}
