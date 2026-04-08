package main

import (
	"context"
	"testing"
	"time"

	"helm-wails/internal/session"
)

func TestNewAppDoesNotInitializeEagerly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	started := make(chan struct{}, 1)
	resolver := func(baseEnv []string) ([]string, error) {
		started <- struct{}{}
		return baseEnv, nil
	}

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	select {
	case <-started:
		t.Fatalf("resolver ran during NewApp(), want deferred startup initialization")
	default:
	}

	app.ctx = context.Background()
	app.startInitialization(resolver)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("resolver did not run after startInitialization()")
	}

	if err := app.waitReady(); err != nil {
		t.Fatalf("waitReady() error = %v", err)
	}
	if app.manager == nil {
		t.Fatalf("manager = nil, want initialized manager")
	}
	if app.registry == nil {
		t.Fatalf("registry = nil, want initialized registry")
	}
	if app.store == nil {
		t.Fatalf("store = nil, want initialized store")
	}

	t.Cleanup(func() {
		app.shutdown(nil)
	})
}

func TestBeforeCloseSkipsConfirmationWithoutActiveAgents(t *testing.T) {
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	app.ctx = context.Background()
	close(app.ready)
	app.activeAgents = func() int { return 0 }
	app.confirmClose = func(context.Context, int) (bool, error) {
		t.Fatalf("confirmClose should not be called without active agent sessions")
		return false, nil
	}

	if prevent := app.beforeClose(context.Background()); prevent {
		t.Fatalf("beforeClose() = true, want false when no agent sessions are active")
	}
}

func TestBeforeClosePreventsQuitWhenUserCancels(t *testing.T) {
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	app.ctx = context.Background()
	close(app.ready)

	calls := 0
	app.activeAgents = func() int { return 3 }
	app.confirmClose = func(_ context.Context, count int) (bool, error) {
		calls++
		if count != 3 {
			t.Fatalf("confirmClose count = %d, want 3", count)
		}
		return false, nil
	}

	if prevent := app.beforeClose(context.Background()); !prevent {
		t.Fatalf("beforeClose() = false, want true when user cancels close")
	}
	if calls != 1 {
		t.Fatalf("confirmClose called %d times, want 1", calls)
	}
}

func TestConfirmClearPeerMessagesUsesInjectedPrompt(t *testing.T) {
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	ctx := context.Background()
	app.ctx = ctx
	app.manager = new(session.Manager)
	close(app.ready)

	calls := 0
	app.confirmClearPeerMessages = func(got context.Context) (bool, error) {
		calls++
		if got != ctx {
			t.Fatalf("confirmClearPeerMessages context = %#v, want app context", got)
		}
		return true, nil
	}

	confirmed, err := app.ConfirmClearPeerMessages()
	if err != nil {
		t.Fatalf("ConfirmClearPeerMessages() error = %v", err)
	}
	if !confirmed {
		t.Fatalf("ConfirmClearPeerMessages() = false, want true")
	}
	if calls != 1 {
		t.Fatalf("confirmClearPeerMessages called %d times, want 1", calls)
	}
}

func TestConfirmDiscardFileChangesUsesInjectedPrompt(t *testing.T) {
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	ctx := context.Background()
	app.ctx = ctx
	app.manager = new(session.Manager)
	close(app.ready)

	calls := 0
	app.confirmDiscardFileChanges = func(got context.Context) (bool, error) {
		calls++
		if got != ctx {
			t.Fatalf("confirmDiscardFileChanges context = %#v, want app context", got)
		}
		return true, nil
	}

	confirmed, err := app.ConfirmDiscardFileChanges()
	if err != nil {
		t.Fatalf("ConfirmDiscardFileChanges() error = %v", err)
	}
	if !confirmed {
		t.Fatalf("ConfirmDiscardFileChanges() = false, want true")
	}
	if calls != 1 {
		t.Fatalf("confirmDiscardFileChanges called %d times, want 1", calls)
	}
}

func TestShowWindowOnlyShowsOnce(t *testing.T) {
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	app.ctx = context.Background()
	calls := 0
	app.showWindow = func(context.Context) {
		calls++
	}

	app.ShowWindow()
	app.ShowWindow()
	app.revealWindow(context.Background())

	if calls != 1 {
		t.Fatalf("showWindow called %d times, want 1", calls)
	}
}
