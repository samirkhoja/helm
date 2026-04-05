package session

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"helm-wails/internal/agent"
)

type eventCollector struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (c *eventCollector) Emit(event string, payload any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, recordedEvent{name: event, payload: payload})
}

func (c *eventCollector) outputContains(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, event := range c.events {
		if event.name != EventSessionOutput {
			continue
		}
		payload, ok := event.payload.(SessionOutputEvent)
		if ok && strings.Contains(payload.Data, substr) {
			return true
		}
	}
	return false
}

func TestPTYStarterStreamsOutputAndExits(t *testing.T) {
	t.Parallel()

	starter := NewPTYStarter()
	sink := &eventCollector{}
	exitCh := make(chan ExitInfo, 1)

	handle, err := starter.Start(agent.LaunchSpec{
		ID:      "shell",
		Label:   "Shell",
		Command: "/bin/sh",
		Args:    []string{"-c", "printf 'hello from helm'"},
		Env:     os.Environ(),
		CWD:     t.TempDir(),
	}, StartMeta{
		SessionID:  1,
		WorktreeID: 1,
	}, sink, func(info ExitInfo) {
		exitCh <- info
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer handle.Close()

	select {
	case info := <-exitCh:
		if info.ExitCode != 0 {
			t.Fatalf("exit code = %d, want 0", info.ExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for process exit")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if sink.outputContains("hello from helm") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected session output event")
}
