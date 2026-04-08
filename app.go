package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"helm-wails/internal/agent"
	"helm-wails/internal/session"
	persist "helm-wails/internal/state"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	menuActionNewWorkspace         = "new-workspace"
	menuActionNewSession           = "new-session"
	menuActionCloseSession         = "close-session"
	menuActionSaveFileEditor       = "save-file-editor"
	menuActionToggleSidebar        = "toggle-sidebar"
	menuActionToggleDiff           = "toggle-diff"
	menuActionToggleFiles          = "toggle-files"
	menuActionTogglePeers          = "toggle-peers"
	menuActionToggleDiffFullscreen = "toggle-diff-fullscreen"
	menuActionFocusTerminal        = "focus-terminal"
	menuActionFocusFilesPanel      = "focus-files-panel"
	menuActionZoomOutTerminal      = "zoom-out-terminal"
	menuActionResetTerminalZoom    = "reset-terminal-zoom"
	menuActionZoomInTerminal       = "zoom-in-terminal"
	menuActionRefreshDiff          = "refresh-diff"
	menuActionZoomOutDiff          = "zoom-out-diff"
	menuActionResetDiffZoom        = "reset-diff-zoom"
	menuActionZoomInDiff           = "zoom-in-diff"
	menuActionPreviousSession      = "previous-session"
	menuActionNextSession          = "next-session"
	menuActionPreviousSessionAlt   = "previous-session-alt"
	menuActionNextSessionAlt       = "next-session-alt"
	menuActionDismissOverlay       = "dismiss-overlay"
	menuActionCommandPalette       = "command-palette"
)

type App struct {
	ctx                       context.Context
	inheritedEnv              []string
	registry                  *agent.Registry
	manager                   *session.Manager
	confirmClose              func(context.Context, int) (bool, error)
	confirmClearPeerMessages  func(context.Context) (bool, error)
	confirmDiscardFileChanges func(context.Context) (bool, error)
	activeAgents              func() int
	store                     persist.Store
	initErr                   error
	startupNotice             string
	ready                     chan struct{}
	initOnce                  sync.Once
	showOnce                  sync.Once
	showWindow                func(context.Context)
}

func NewApp() (*App, error) {
	return &App{
		ready: make(chan struct{}),
	}, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startInitialization(resolveLoginShellEnv)
}

func (a *App) domReady(ctx context.Context) {
	go func() {
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		<-timer.C
		a.revealWindow(ctx)
	}()
}

func (a *App) ShowWindow() {
	a.revealWindow(a.ctx)
}

func (a *App) revealWindow(ctx context.Context) {
	if ctx == nil {
		ctx = a.ctx
	}
	if ctx == nil {
		return
	}

	showWindow := a.showWindow
	if showWindow == nil {
		showWindow = runtime.WindowShow
	}

	a.showOnce.Do(func() {
		showWindow(ctx)
	})
}

func (a *App) shutdown(ctx context.Context) {
	if a.ctx != nil && a.ready != nil {
		<-a.ready
	}
	if a.manager != nil {
		a.manager.Shutdown()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.ctx != nil && a.ready != nil {
		<-a.ready
	}

	activeAgents := a.runningAgentSessionCount()
	if activeAgents == 0 {
		return false
	}

	confirmClose := a.confirmClose
	if confirmClose == nil {
		confirmClose = confirmTerminateSessionsOnClose
	}

	allowClose, err := confirmClose(ctx, activeAgents)
	if err != nil {
		return false
	}
	return !allowClose
}

func (a *App) emitMenuAction(action string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "menu:action", action)
}

func (a *App) runningAgentSessionCount() int {
	if a.activeAgents != nil {
		return a.activeAgents()
	}
	if a.manager == nil {
		return 0
	}
	return a.manager.ActiveAgentSessionCount()
}

func confirmTerminateSessionsOnClose(ctx context.Context, activeAgents int) (bool, error) {
	if activeAgents <= 0 {
		return true, nil
	}

	noun := "agent sessions"
	if activeAgents == 1 {
		noun = "agent session"
	}

	response, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Quit Helm?",
		Message:       fmt.Sprintf("Helm still has %d active %s. Quitting now will terminate them.", activeAgents, noun),
		Buttons:       []string{"Cancel", "Quit Helm"},
		DefaultButton: "Quit Helm",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, err
	}
	return response == "Quit Helm", nil
}

func confirmClearPeerMessages(ctx context.Context) (bool, error) {
	response, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Clear peer messages?",
		Message:       "Clear all peer messages from the Helm panel?",
		Buttons:       []string{"Cancel", "Clear messages"},
		DefaultButton: "Clear messages",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, err
	}
	return response == "Clear messages", nil
}

func confirmDiscardFileChanges(ctx context.Context) (bool, error) {
	response, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Discard unsaved changes?",
		Message:       "Discard unsaved file changes?",
		Buttons:       []string{"Cancel", "Discard changes"},
		DefaultButton: "Discard changes",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, err
	}
	return response == "Discard changes", nil
}

func (a *App) Bootstrap() (session.BootstrapResult, error) {
	if err := a.waitReady(); err != nil {
		return session.BootstrapResult{}, err
	}
	result, err := a.manager.Bootstrap()
	if err != nil {
		return session.BootstrapResult{}, err
	}
	if a.startupNotice != "" {
		if result.RestoreNotice != "" {
			result.RestoreNotice = a.startupNotice + "\n" + result.RestoreNotice
		} else {
			result.RestoreNotice = a.startupNotice
		}
	}
	return result, nil
}

func (a *App) ChooseWorkspace() (*session.WorkspaceChoice, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	if a.ctx == nil {
		return nil, errors.New("helm runtime is not ready")
	}

	rootPath, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose workspace",
		CanCreateDirectories: true,
		ShowHiddenFiles:      true,
	})
	if err != nil {
		return nil, err
	}
	if rootPath == "" {
		return nil, nil
	}

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}

	choice, err := session.DescribeWorkspace(absRoot)
	if err != nil {
		return nil, err
	}
	return &choice, nil
}

func (a *App) CreateWorkspaceSession(rootPath, agentID string) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.CreateWorkspaceSession(rootPath, agentID)
}

func (a *App) CreateSession(worktreeID int, agentID string) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.CreateSession(worktreeID, agentID)
}

func (a *App) ActivateSession(sessionID int) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.ActivateSession(sessionID)
}

func (a *App) KillSession(sessionID int) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.KillSession(sessionID)
}

func (a *App) SendSessionInput(sessionID int, data string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return a.manager.SendInput(sessionID, data)
}

func (a *App) ResizeSession(sessionID int, cols, rows int) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return a.manager.ResizeSession(sessionID, cols, rows)
}

func (a *App) CreateWorktreeSession(repoID int, request session.WorktreeCreateRequest) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.CreateWorktreeSession(repoID, request)
}

func (a *App) GetWorktreeDiff(worktreeID int) (session.WorktreeDiff, error) {
	if err := a.waitReady(); err != nil {
		return session.WorktreeDiff{}, err
	}
	return a.manager.WorktreeDiff(worktreeID)
}

func (a *App) GetFileDiff(worktreeID int, path string, staged bool) (session.FileDiff, error) {
	if err := a.waitReady(); err != nil {
		return session.FileDiff{}, err
	}
	return a.manager.FileDiff(worktreeID, path, staged)
}

func (a *App) ListWorktreeFiles(worktreeID int) ([]string, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	return a.manager.ListWorktreeFiles(worktreeID)
}

func (a *App) SearchWorktreeContents(worktreeID int, query string, limit int) ([]session.WorktreeContentMatch, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	return a.manager.SearchWorktreeContents(worktreeID, query, limit)
}

func (a *App) ListWorktreeEntries(worktreeID int, relativeDir string) ([]session.WorktreeEntry, error) {
	if err := a.waitReady(); err != nil {
		return nil, err
	}
	return a.manager.ListWorktreeEntries(worktreeID, relativeDir)
}

func (a *App) ReadWorktreeFile(worktreeID int, relativePath string) (session.WorktreeFile, error) {
	if err := a.waitReady(); err != nil {
		return session.WorktreeFile{}, err
	}
	return a.manager.ReadWorktreeFile(worktreeID, relativePath)
}

func (a *App) SaveWorktreeFile(worktreeID int, relativePath, content, expectedVersion string) (session.WorktreeFile, error) {
	if err := a.waitReady(); err != nil {
		return session.WorktreeFile{}, err
	}
	return a.manager.SaveWorktreeFile(worktreeID, relativePath, content, expectedVersion)
}

func (a *App) UpdateSessionCWD(sessionID int, cwdPath string) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return a.manager.UpdateSessionCWD(sessionID, cwdPath)
}

func (a *App) UpdateSessionMode(sessionID int, adapterID string) (session.AppSnapshot, error) {
	if err := a.waitReady(); err != nil {
		return session.AppSnapshot{}, err
	}
	return a.manager.UpdateSessionMode(sessionID, adapterID)
}

func (a *App) SaveUIState(uiState session.UIStateDTO) error {
	if err := a.waitReady(); err != nil {
		return err
	}
	return a.manager.SaveUIState(uiState)
}

func (a *App) DeletePeerMessage(messageID int64) (session.PeerStateDTO, error) {
	if err := a.waitReady(); err != nil {
		return session.PeerStateDTO{}, err
	}
	return a.manager.DeletePeerMessage(messageID)
}

func (a *App) ClearPeerMessages() (session.PeerStateDTO, error) {
	if err := a.waitReady(); err != nil {
		return session.PeerStateDTO{}, err
	}
	return a.manager.ClearPeerMessages()
}

func (a *App) ConfirmClearPeerMessages() (bool, error) {
	if err := a.waitReady(); err != nil {
		return false, err
	}
	if a.ctx == nil {
		return false, errors.New("helm runtime is not ready")
	}

	confirm := a.confirmClearPeerMessages
	if confirm == nil {
		confirm = confirmClearPeerMessages
	}
	return confirm(a.ctx)
}

func (a *App) ConfirmDiscardFileChanges() (bool, error) {
	if err := a.waitReady(); err != nil {
		return false, err
	}
	if a.ctx == nil {
		return false, errors.New("helm runtime is not ready")
	}

	confirm := a.confirmDiscardFileChanges
	if confirm == nil {
		confirm = confirmDiscardFileChanges
	}
	return confirm(a.ctx)
}

func (a *App) startInitialization(resolver loginShellEnvResolver) {
	a.initOnce.Do(func() {
		go func() {
			defer close(a.ready)
			a.initErr = a.initializeWithResolver(resolver)
		}()
	})
}

func (a *App) waitReady() error {
	if a.ctx == nil || a.ready == nil {
		return errors.New("helm runtime is not ready")
	}
	<-a.ready
	if a.initErr != nil {
		return a.initErr
	}
	if a.manager == nil {
		return errors.New("helm runtime is not ready")
	}
	return nil
}

func (a *App) initializeWithResolver(resolver loginShellEnvResolver) error {
	inheritedEnv, startupNotice := normalizeStartupEnv(inheritedEnvironment(), resolver)
	a.inheritedEnv = inheritedEnv
	a.startupNotice = startupNotice

	registry, err := agent.LoadRegistryWithEnv(inheritedEnv)
	if err != nil {
		a.initErr = err
		return err
	}
	a.registry = registry

	storePath, err := persist.DefaultDBPath()
	if err != nil {
		a.initErr = err
		return err
	}
	store, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		a.initErr = err
		return err
	}
	a.store = store
	a.manager = session.NewManager(
		a.registry,
		inheritedEnv,
		session.NewPTYStarter(),
		session.EventSinkFunc(func(event string, payload any) {
			if a.ctx == nil {
				return
			}
			runtime.EventsEmit(a.ctx, event, payload)
		}),
		store,
	)
	executablePath, err := os.Executable()
	if err != nil {
		a.initErr = err
		return err
	}
	if err := a.manager.EnablePeerRuntime(executablePath); err != nil {
		a.initErr = err
		return err
	}
	return nil
}
