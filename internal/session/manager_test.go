package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"helm-wails/internal/agent"
	persist "helm-wails/internal/state"
)

type failingStore struct {
	persist.Store
	saveUIPreferencesErr  error
	setActiveSessionErr   error
	deleteSessionErr      error
	upsertPeerRegisterErr error
}

type countingStore struct {
	persist.Store
	mu                 sync.Mutex
	deleteSessionCalls int
}

func (s *failingStore) SaveUIPreferences(ui persist.UIState) error {
	if s.saveUIPreferencesErr != nil {
		return s.saveUIPreferencesErr
	}
	return s.Store.SaveUIPreferences(ui)
}

func (s *failingStore) SetActiveSessionID(storageID int64) error {
	if s.setActiveSessionErr != nil {
		return s.setActiveSessionErr
	}
	return s.Store.SetActiveSessionID(storageID)
}

func (s *failingStore) DeleteSession(storageID int64, nextActiveStorageID int64) error {
	if s.deleteSessionErr != nil {
		return s.deleteSessionErr
	}
	return s.Store.DeleteSession(storageID, nextActiveStorageID)
}

func (s *failingStore) UpsertPeerRegistration(record persist.PeerRegistrationRecord) error {
	if s.upsertPeerRegisterErr != nil {
		return s.upsertPeerRegisterErr
	}
	return s.Store.UpsertPeerRegistration(record)
}

func (s *countingStore) DeleteSession(storageID int64, nextActiveStorageID int64) error {
	s.mu.Lock()
	s.deleteSessionCalls++
	s.mu.Unlock()
	return s.Store.DeleteSession(storageID, nextActiveStorageID)
}

func (s *countingStore) deleteCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteSessionCalls
}

type recordedEvent struct {
	name    string
	payload any
}

type fakeSink struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (s *fakeSink) Emit(event string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, recordedEvent{name: event, payload: payload})
}

func (s *fakeSink) eventsByName(name string) []recordedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]recordedEvent, 0, len(s.events))
	for _, event := range s.events {
		if event.name == name {
			out = append(out, event)
		}
	}
	return out
}

type fakeStarter struct {
	mu      sync.Mutex
	handles map[int]*fakeHandle
	specs   map[int]agent.LaunchSpec
}

func newFakeStarter() *fakeStarter {
	return &fakeStarter{
		handles: make(map[int]*fakeHandle),
		specs:   make(map[int]agent.LaunchSpec),
	}
}

func (s *fakeStarter) Start(spec agent.LaunchSpec, meta StartMeta, sink EventSink, onExit ExitHandler) (Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	handle := &fakeHandle{
		meta:   meta,
		onExit: onExit,
	}
	s.handles[meta.SessionID] = handle
	s.specs[meta.SessionID] = spec
	return handle, nil
}

func (s *fakeStarter) handle(sessionID int) *fakeHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handles[sessionID]
}

func (s *fakeStarter) spec(sessionID int) agent.LaunchSpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.specs[sessionID]
}

type fakeHandle struct {
	meta      StartMeta
	writes    []string
	resizes   [][2]int
	closed    bool
	onExit    ExitHandler
	closeErr  error
	closeHook func()
}

func (h *fakeHandle) Write(data string) error {
	h.writes = append(h.writes, data)
	return nil
}

func (h *fakeHandle) Resize(cols, rows int) error {
	h.resizes = append(h.resizes, [2]int{cols, rows})
	return nil
}

func (h *fakeHandle) Close() error {
	h.closed = true
	if h.closeHook != nil {
		h.closeHook()
	}
	return h.closeErr
}

func (h *fakeHandle) exit(code int, err error) {
	h.onExit(ExitInfo{
		SessionID:  h.meta.SessionID,
		WorktreeID: h.meta.WorktreeID,
		ExitCode:   code,
		Err:        err,
	})
}

func newTestRegistry(t *testing.T) *agent.Registry {
	t.Helper()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:      "codex",
			Label:   "Codex",
			Command: "/bin/sh",
			ResumeArgs: []string{
				"resume",
				"--last",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func newShellOnlyRegistry(t *testing.T) *agent.Registry {
	t.Helper()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func newShellBackedRegistry(t *testing.T) *agent.Registry {
	t.Helper()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:      "codex",
			Label:   "Codex",
			Command: "/usr/bin/env",
			ResumeArgs: []string{
				"resume",
				"--last",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func newPeerEnabledRegistry(t *testing.T) *agent.Registry {
	t.Helper()

	registry, err := agent.NewRegistry([]agent.AdapterConfig{
		{
			ID:      "shell",
			Label:   "Shell",
			Command: "/bin/sh",
		},
		{
			ID:          "codex",
			Label:       "Codex",
			Command:     "/bin/sh",
			Family:      "codex",
			PeerEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func TestManagerCreateWorkspaceSessionDedupesRepoAcrossWorktrees(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, newTestStore(t))

	repoRoot, worktreeRoot := newGitRepoWithWorktree(t, gitPath)

	first, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	second, err := manager.CreateWorkspaceSession(worktreeRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() second error = %v", err)
	}

	if len(first.Repos) != 1 || len(second.Repos) != 1 {
		t.Fatalf("snapshot repos = %#v / %#v, want single repo", first.Repos, second.Repos)
	}
	repo := second.Repos[0]
	if len(repo.Worktrees) != 2 {
		t.Fatalf("repo worktrees = %#v, want 2", repo.Worktrees)
	}
	sessionCount := 0
	for _, worktree := range repo.Worktrees {
		sessionCount += len(worktree.Sessions)
	}
	if sessionCount != 2 {
		t.Fatalf("session count = %d, want 2", sessionCount)
	}
	if second.ActiveSessionID == 0 {
		t.Fatalf("active session not set")
	}
	if second.ActiveWorktreeID == 0 || second.ActiveRepoID != repo.ID {
		t.Fatalf("active repo/worktree = %d/%d, want repo %d and non-zero worktree", second.ActiveRepoID, second.ActiveWorktreeID, repo.ID)
	}
	activeWorktree := findWorktreeDTO(repo.Worktrees, second.ActiveWorktreeID)
	if activeWorktree == nil || !samePath(t, activeWorktree.RootPath, worktreeRoot) {
		t.Fatalf("active worktree = %#v, want %s", activeWorktree, worktreeRoot)
	}
	if second.ActiveSessionID != activeWorktree.Sessions[0].ID {
		t.Fatalf("active session = %d, want newest session", second.ActiveSessionID)
	}
}

func TestManagerCreateWorkspaceSessionNonGitCreatesSyntheticRepo(t *testing.T) {
	t.Parallel()

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, newTestStore(t))

	root := t.TempDir()
	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}

	if len(snapshot.Repos) != 1 {
		t.Fatalf("repos = %#v, want 1", snapshot.Repos)
	}
	repo := snapshot.Repos[0]
	if repo.IsGitRepo {
		t.Fatalf("repo.IsGitRepo = true, want false")
	}
	if len(repo.Worktrees) != 1 {
		t.Fatalf("repo.Worktrees = %#v, want 1", repo.Worktrees)
	}
	if len(repo.Worktrees[0].Sessions) != 1 {
		t.Fatalf("sessions = %#v, want 1", repo.Worktrees[0].Sessions)
	}
	if snapshot.ActiveRepoID != repo.ID || snapshot.ActiveWorktreeID != repo.Worktrees[0].ID {
		t.Fatalf("active repo/worktree = %d/%d, want %d/%d", snapshot.ActiveRepoID, snapshot.ActiveWorktreeID, repo.ID, repo.Worktrees[0].ID)
	}
}

func TestManagerActivateSendResizeAndExit(t *testing.T) {
	t.Parallel()

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, newTestStore(t))

	root := t.TempDir()

	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession(shell) error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]
	snapshot, err = manager.CreateSession(worktree.ID, "codex")
	if err != nil {
		t.Fatalf("CreateSession(codex) error = %v", err)
	}

	worktreeAfterCreate := findWorktreeDTO(snapshot.Repos[0].Worktrees, worktree.ID)
	if worktreeAfterCreate == nil {
		t.Fatalf("worktree %d not found after CreateSession", worktree.ID)
	}
	firstSession := worktreeAfterCreate.Sessions[0]
	secondSession := worktreeAfterCreate.Sessions[1]

	snapshot, err = manager.ActivateSession(firstSession.ID)
	if err != nil {
		t.Fatalf("ActivateSession() error = %v", err)
	}
	if snapshot.ActiveSessionID != firstSession.ID {
		t.Fatalf("snapshot.ActiveSessionID = %d, want %d", snapshot.ActiveSessionID, firstSession.ID)
	}

	if err := manager.SendInput(firstSession.ID, "pwd\n"); err != nil {
		t.Fatalf("SendInput() error = %v", err)
	}
	if err := manager.ResizeSession(firstSession.ID, 132, 42); err != nil {
		t.Fatalf("ResizeSession() error = %v", err)
	}

	firstHandle := starter.handle(firstSession.ID)
	if got := len(firstHandle.writes); got != 1 || firstHandle.writes[0] != "pwd\n" {
		t.Fatalf("writes = %#v", firstHandle.writes)
	}
	if got := len(firstHandle.resizes); got != 1 || firstHandle.resizes[0] != [2]int{132, 42} {
		t.Fatalf("resizes = %#v", firstHandle.resizes)
	}

	starter.handle(secondSession.ID).exit(0, nil)

	snapshot = manager.Snapshot()
	worktree = snapshot.Repos[0].Worktrees[0]
	if len(snapshot.Repos) != 1 || len(worktree.Sessions) != 1 {
		t.Fatalf("snapshot after exit = %#v", snapshot)
	}
	if snapshot.ActiveSessionID != firstSession.ID {
		t.Fatalf("snapshot.ActiveSessionID = %d, want %d", snapshot.ActiveSessionID, firstSession.ID)
	}

	if got := len(sink.eventsByName(EventAppSnapshot)); got == 0 {
		t.Fatalf("expected %s event", EventAppSnapshot)
	}
	if got := len(sink.eventsByName(EventSessionLifecycle)); got == 0 {
		t.Fatalf("expected %s event", EventSessionLifecycle)
	}
}

func TestManagerSaveUIStateDoesNotMutateOnPersistenceFailure(t *testing.T) {
	t.Parallel()

	baseStore := newTestStore(t)
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	manager := NewManager(
		newTestRegistry(t),
		os.Environ(),
		newFakeStarter(),
		&fakeSink{},
		&failingStore{
			Store:                baseStore,
			saveUIPreferencesErr: errors.New("save ui failed"),
		},
	)

	if err := manager.SaveUIState(UIStateDTO{
		SidebarOpen:       false,
		SidebarWidth:      320,
		DiffPanelOpen:     true,
		DiffPanelWidth:    420,
		TerminalFontSize:  18,
		UtilityPanelTab:   "files",
		CollapsedRepoKeys: []string{"repo-a"},
	}); err == nil {
		t.Fatalf("SaveUIState() error = nil, want persistence failure")
	}

	result, err := manager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if !result.UIState.SidebarOpen || result.UIState.DiffPanelOpen {
		t.Fatalf("UI state mutated in memory after failed save: %#v", result.UIState)
	}
	if result.UIState.UtilityPanelTab != "diff" {
		t.Fatalf("UtilityPanelTab = %q, want diff", result.UIState.UtilityPanelTab)
	}
	if result.UIState.TerminalFontSize != 12 {
		t.Fatalf("TerminalFontSize = %d, want 12", result.UIState.TerminalFontSize)
	}
	if len(result.UIState.CollapsedRepoKeys) != 0 {
		t.Fatalf("CollapsedRepoKeys = %#v, want unchanged defaults", result.UIState.CollapsedRepoKeys)
	}
}

func TestManagerActivateSessionDoesNotMutateOnPersistenceFailure(t *testing.T) {
	t.Parallel()

	baseStore := newTestStore(t)
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	starter := newFakeStarter()
	manager := NewManager(
		newTestRegistry(t),
		os.Environ(),
		starter,
		&fakeSink{},
		&failingStore{
			Store:               baseStore,
			setActiveSessionErr: errors.New("set active failed"),
		},
	)

	root := t.TempDir()
	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]
	snapshot, err = manager.CreateSession(worktree.ID, "codex")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	worktreeState := findWorktreeDTO(snapshot.Repos[0].Worktrees, worktree.ID)
	if worktreeState == nil || len(worktreeState.Sessions) != 2 {
		t.Fatalf("sessions = %#v, want 2", worktreeState)
	}
	originalActiveSessionID := snapshot.ActiveSessionID
	targetSessionID := worktreeState.Sessions[0].ID
	if targetSessionID == originalActiveSessionID {
		targetSessionID = worktreeState.Sessions[1].ID
	}

	if _, err := manager.ActivateSession(targetSessionID); err == nil {
		t.Fatalf("ActivateSession() error = nil, want persistence failure")
	}

	current := manager.Snapshot()
	if current.ActiveSessionID != originalActiveSessionID {
		t.Fatalf("ActiveSessionID = %d, want unchanged %d", current.ActiveSessionID, originalActiveSessionID)
	}
}

func TestManagerKillSessionDoesNotRemoveOnPersistenceFailure(t *testing.T) {
	t.Parallel()

	baseStore := newTestStore(t)
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	starter := newFakeStarter()
	manager := NewManager(
		newTestRegistry(t),
		os.Environ(),
		starter,
		&fakeSink{},
		&failingStore{
			Store:            baseStore,
			deleteSessionErr: errors.New("delete failed"),
		},
	)

	root := t.TempDir()
	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	sessionID := snapshot.Repos[0].Worktrees[0].Sessions[0].ID

	if _, err := manager.KillSession(sessionID); err == nil {
		t.Fatalf("KillSession() error = nil, want persistence failure")
	}

	current := manager.Snapshot()
	if len(current.Repos) != 1 || len(current.Repos[0].Worktrees) != 1 || len(current.Repos[0].Worktrees[0].Sessions) != 1 {
		t.Fatalf("snapshot = %#v, want unchanged session after failed delete", current)
	}
	if current.ActiveSessionID != sessionID {
		t.Fatalf("ActiveSessionID = %d, want unchanged %d", current.ActiveSessionID, sessionID)
	}
}

func TestManagerKillSessionRemovesItImmediately(t *testing.T) {
	t.Parallel()

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, newTestStore(t))

	root := t.TempDir()

	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession(shell) error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]
	snapshot, err = manager.CreateSession(worktree.ID, "codex")
	if err != nil {
		t.Fatalf("CreateSession(codex) error = %v", err)
	}

	worktreeAfterCreate := findWorktreeDTO(snapshot.Repos[0].Worktrees, worktree.ID)
	if worktreeAfterCreate == nil {
		t.Fatalf("worktree %d not found after CreateSession", worktree.ID)
	}
	firstSession := worktreeAfterCreate.Sessions[0]
	secondSession := worktreeAfterCreate.Sessions[1]

	nextSnapshot, err := manager.KillSession(secondSession.ID)
	if err != nil {
		t.Fatalf("KillSession() error = %v", err)
	}

	if !starter.handle(secondSession.ID).closed {
		t.Fatalf("session handle was not closed")
	}
	worktree = nextSnapshot.Repos[0].Worktrees[0]
	if len(nextSnapshot.Repos) != 1 || len(worktree.Sessions) != 1 {
		t.Fatalf("snapshot after kill = %#v", nextSnapshot)
	}
	if worktree.Sessions[0].ID != firstSession.ID {
		t.Fatalf("remaining session = %#v, want %d", worktree.Sessions, firstSession.ID)
	}
	if nextSnapshot.ActiveSessionID != firstSession.ID {
		t.Fatalf("active session = %d, want %d", nextSnapshot.ActiveSessionID, firstSession.ID)
	}

	lifecycle := sink.eventsByName(EventSessionLifecycle)
	if len(lifecycle) == 0 {
		t.Fatalf("expected %s event", EventSessionLifecycle)
	}
	lastPayload, ok := lifecycle[len(lifecycle)-1].payload.(SessionLifecycleEvent)
	if !ok {
		t.Fatalf("unexpected lifecycle payload type = %T", lifecycle[len(lifecycle)-1].payload)
	}
	if lastPayload.Status != "killed" {
		t.Fatalf("lifecycle status = %q, want killed", lastPayload.Status)
	}
}

func TestManagerKillSessionSuppressesExitLifecycleDuringIntentionalClose(t *testing.T) {
	t.Parallel()

	baseStore := newTestStore(t)
	counting := &countingStore{Store: baseStore}
	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, counting)

	root := t.TempDir()
	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	sessionID := snapshot.Repos[0].Worktrees[0].Sessions[0].ID
	handle := starter.handle(sessionID)
	handle.closeHook = func() {
		handle.exit(0, nil)
	}

	nextSnapshot, err := manager.KillSession(sessionID)
	if err != nil {
		t.Fatalf("KillSession() error = %v", err)
	}

	if len(nextSnapshot.Repos) != 0 {
		t.Fatalf("snapshot after kill = %#v, want no repos", nextSnapshot)
	}
	if got := counting.deleteCalls(); got != 1 {
		t.Fatalf("DeleteSession() calls = %d, want 1", got)
	}

	lifecycle := sink.eventsByName(EventSessionLifecycle)
	if len(lifecycle) != 1 {
		t.Fatalf("lifecycle events = %#v, want exactly one", lifecycle)
	}
	payload, ok := lifecycle[0].payload.(SessionLifecycleEvent)
	if !ok {
		t.Fatalf("unexpected lifecycle payload type = %T", lifecycle[0].payload)
	}
	if payload.Status != "killed" {
		t.Fatalf("lifecycle status = %q, want killed", payload.Status)
	}
}

func TestManagerUpdateSessionModeKeepsShellBackedSessionAlive(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newShellBackedRegistry(t), os.Environ(), starter, sink, store)

	root := t.TempDir()
	snapshot, err := manager.CreateWorkspaceSession(root, "codex")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession(codex) error = %v", err)
	}

	worktree := snapshot.Repos[0].Worktrees[0]
	if len(worktree.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want 1", worktree.Sessions)
	}
	session := worktree.Sessions[0]
	if session.AdapterID != "codex" {
		t.Fatalf("session.AdapterID = %q, want codex", session.AdapterID)
	}

	spec := starter.spec(session.ID)
	if spec.Command != "/bin/sh" {
		t.Fatalf("starter spec command = %q, want shell", spec.Command)
	}
	handle := starter.handle(session.ID)
	if len(handle.writes) != 1 || !strings.Contains(handle.writes[0], "/usr/bin/env") {
		t.Fatalf("boot writes = %#v, want /usr/bin/env command", handle.writes)
	}

	nextSnapshot, err := manager.UpdateSessionMode(session.ID, "shell")
	if err != nil {
		t.Fatalf("UpdateSessionMode(shell) error = %v", err)
	}
	if len(nextSnapshot.Repos) != 1 || len(nextSnapshot.Repos[0].Worktrees[0].Sessions) != 1 {
		t.Fatalf("snapshot after shell mode = %#v, want same session count", nextSnapshot)
	}
	if nextSnapshot.Repos[0].Worktrees[0].Sessions[0].AdapterID != "shell" {
		t.Fatalf("adapter after shell mode = %q, want shell", nextSnapshot.Repos[0].Worktrees[0].Sessions[0].AdapterID)
	}
	if got := len(sink.eventsByName(EventSessionLifecycle)); got != 0 {
		t.Fatalf("lifecycle events = %#v, want none", sink.eventsByName(EventSessionLifecycle))
	}

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if data.Sessions[0].AdapterID != "shell" {
		t.Fatalf("persisted AdapterID = %q, want shell", data.Sessions[0].AdapterID)
	}
	if data.UIState.LastUsedAgentID != "codex" {
		t.Fatalf("LastUsedAgentID = %q, want codex", data.UIState.LastUsedAgentID)
	}

	nextSnapshot, err = manager.UpdateSessionMode(session.ID, "codex")
	if err != nil {
		t.Fatalf("UpdateSessionMode(codex) error = %v", err)
	}
	if nextSnapshot.Repos[0].Worktrees[0].Sessions[0].AdapterID != "codex" {
		t.Fatalf("adapter after codex mode = %q, want codex", nextSnapshot.Repos[0].Worktrees[0].Sessions[0].AdapterID)
	}
}

func TestManagerCreateWorktreeSessionCreatesCheckoutAndSession(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, newTestStore(t))

	repoRoot := newGitRepo(t, gitPath)
	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}

	repo := snapshot.Repos[0]
	request := WorktreeCreateRequest{
		Mode:       WorktreeModeNewBranch,
		BranchName: "feature/helm",
		AgentID:    "codex",
	}

	nextSnapshot, err := manager.CreateWorktreeSession(repo.ID, request)
	if err != nil {
		t.Fatalf("CreateWorktreeSession() error = %v", err)
	}

	repo = nextSnapshot.Repos[0]
	if len(repo.Worktrees) != 2 {
		t.Fatalf("repo worktrees = %#v, want 2", repo.Worktrees)
	}

	var created *WorktreeDTO
	for i := range repo.Worktrees {
		if repo.Worktrees[i].GitBranch == "feature/helm" {
			created = &repo.Worktrees[i]
			break
		}
	}
	if created == nil {
		t.Fatalf("created worktree not found in %#v", repo.Worktrees)
	}
	if len(created.Sessions) != 1 {
		t.Fatalf("created worktree sessions = %#v, want 1", created.Sessions)
	}
	if nextSnapshot.ActiveWorktreeID != created.ID || nextSnapshot.ActiveSessionID != created.Sessions[0].ID {
		t.Fatalf("active worktree/session = %d/%d, want %d/%d", nextSnapshot.ActiveWorktreeID, nextSnapshot.ActiveSessionID, created.ID, created.Sessions[0].ID)
	}
	if _, err := os.Stat(created.RootPath); err != nil {
		t.Fatalf("created worktree path stat error = %v", err)
	}
}

func TestManagerCreateWorktreeSessionNormalizesDetachedHeadSourceRef(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	starter := newFakeStarter()
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, &fakeSink{}, newTestStore(t))

	repoRoot := newGitRepo(t, gitPath)
	snapshot, err := manager.CreateWorkspaceSession(repoRoot, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}

	repo := snapshot.Repos[0]
	nextSnapshot, err := manager.CreateWorktreeSession(repo.ID, WorktreeCreateRequest{
		Mode:       WorktreeModeNewBranch,
		BranchName: "feature/from-detached",
		SourceRef:  detachedHead,
		AgentID:    "codex",
	})
	if err != nil {
		t.Fatalf("CreateWorktreeSession() error = %v", err)
	}

	var created *WorktreeDTO
	for i := range nextSnapshot.Repos[0].Worktrees {
		if nextSnapshot.Repos[0].Worktrees[i].GitBranch == "feature/from-detached" {
			created = &nextSnapshot.Repos[0].Worktrees[i]
			break
		}
	}
	if created == nil {
		t.Fatalf("created worktree not found in %#v", nextSnapshot.Repos[0].Worktrees)
	}
}

func TestManagerBootstrapRestoresSessionsAndUIState(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	starter := newFakeStarter()
	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), starter, sink, store)

	root := t.TempDir()
	cwd := filepath.Join(root, "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	worktree := snapshot.Repos[0].Worktrees[0]
	snapshot, err = manager.CreateSession(worktree.ID, "codex")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	worktree = snapshot.Repos[0].Worktrees[0]
	shellSession := worktree.Sessions[0]

	if err := manager.UpdateSessionCWD(shellSession.ID, cwd); err != nil {
		t.Fatalf("UpdateSessionCWD() error = %v", err)
	}
	if err := manager.SaveUIState(UIStateDTO{
		SidebarOpen:       false,
		SidebarWidth:      280,
		DiffPanelOpen:     true,
		DiffPanelWidth:    420,
		TerminalFontSize:  19,
		UtilityPanelTab:   "files",
		CollapsedRepoKeys: []string{filepath.Clean(root)},
	}); err != nil {
		t.Fatalf("SaveUIState() error = %v", err)
	}
	snapshot, err = manager.ActivateSession(shellSession.ID)
	if err != nil {
		t.Fatalf("ActivateSession() error = %v", err)
	}
	if snapshot.ActiveSessionID != shellSession.ID {
		t.Fatalf("ActiveSessionID = %d, want %d", snapshot.ActiveSessionID, shellSession.ID)
	}

	manager.Shutdown()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	restoreStore, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(reopen) error = %v", err)
	}
	t.Cleanup(func() {
		_ = restoreStore.Close()
	})

	restoreStarter := newFakeStarter()
	restoreManager := NewManager(newTestRegistry(t), os.Environ(), restoreStarter, &fakeSink{}, restoreStore)
	result, err := restoreManager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if result.RestoreNotice != "" {
		t.Fatalf("RestoreNotice = %q, want empty", result.RestoreNotice)
	}
	if !result.UIState.DiffPanelOpen || result.UIState.SidebarOpen {
		t.Fatalf("UIState = %#v, want saved panel state", result.UIState)
	}
	if result.UIState.UtilityPanelTab != "files" {
		t.Fatalf("UtilityPanelTab = %q, want files", result.UIState.UtilityPanelTab)
	}
	if result.UIState.SidebarWidth != 280 || result.UIState.DiffPanelWidth != 420 {
		t.Fatalf("UIState = %#v, want saved widths", result.UIState)
	}
	if result.UIState.TerminalFontSize != 19 {
		t.Fatalf("TerminalFontSize = %d, want 19", result.UIState.TerminalFontSize)
	}
	if result.Snapshot.LastUsedAgentID != "codex" {
		t.Fatalf("LastUsedAgentID = %q, want codex", result.Snapshot.LastUsedAgentID)
	}

	restoredWorktree := result.Snapshot.Repos[0].Worktrees[0]
	if len(restoredWorktree.Sessions) != 2 {
		t.Fatalf("restored sessions = %#v, want 2", restoredWorktree.Sessions)
	}

	var restoredShell, restoredCodex *SessionDTO
	for i := range restoredWorktree.Sessions {
		switch restoredWorktree.Sessions[i].AdapterID {
		case "shell":
			restoredShell = &restoredWorktree.Sessions[i]
		case "codex":
			restoredCodex = &restoredWorktree.Sessions[i]
		}
	}
	if restoredShell == nil || restoredCodex == nil {
		t.Fatalf("restored sessions = %#v, want shell and codex", restoredWorktree.Sessions)
	}
	if restoredShell.CWDPath != cwd {
		t.Fatalf("restored shell cwd = %q, want %q", restoredShell.CWDPath, cwd)
	}
	if restoreStarter.spec(restoredShell.ID).CWD != cwd {
		t.Fatalf("restore starter cwd = %q, want %q", restoreStarter.spec(restoredShell.ID).CWD, cwd)
	}
	restoreCodexHandle := restoreStarter.handle(restoredCodex.ID)
	if restoreCodexHandle == nil || len(restoreCodexHandle.writes) != 1 {
		t.Fatalf("restored codex boot writes = %#v, want one resume command", restoreCodexHandle)
	}
	if !strings.Contains(restoreCodexHandle.writes[0], "'resume' '--last'") {
		t.Fatalf("restored codex boot write = %q, want resume --last", restoreCodexHandle.writes[0])
	}
	if result.Snapshot.ActiveSessionID != restoredShell.ID {
		t.Fatalf("ActiveSessionID = %d, want restored shell %d", result.Snapshot.ActiveSessionID, restoredShell.ID)
	}
}

func TestManagerBootstrapSkipsMissingWorktreeAndUnavailableAgent(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, store)
	missingRoot := t.TempDir()
	unavailableRoot := t.TempDir()

	if _, err := manager.CreateWorkspaceSession(missingRoot, "shell"); err != nil {
		t.Fatalf("CreateWorkspaceSession(shell) error = %v", err)
	}
	if _, err := manager.CreateWorkspaceSession(unavailableRoot, "codex"); err != nil {
		t.Fatalf("CreateWorkspaceSession(codex) error = %v", err)
	}

	manager.Shutdown()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.RemoveAll(missingRoot); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	restoreStore, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(reopen) error = %v", err)
	}
	t.Cleanup(func() {
		_ = restoreStore.Close()
	})

	restoreManager := NewManager(newShellOnlyRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, restoreStore)
	result, err := restoreManager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if len(result.Snapshot.Repos) != 0 {
		t.Fatalf("repos = %#v, want empty", result.Snapshot.Repos)
	}
	if result.RestoreNotice == "" {
		t.Fatalf("RestoreNotice = empty, want failure summary")
	}
	if !containsAll(result.RestoreNotice, []string{"Shell 1", "Codex 1"}) {
		t.Fatalf("RestoreNotice = %q, want both session names", result.RestoreNotice)
	}
}

func TestManagerBootstrapFallsBackToWorktreeRootWhenSavedCWDIsInvalid(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, store)
	root := t.TempDir()
	cwd := filepath.Join(root, "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	snapshot, err := manager.CreateWorkspaceSession(root, "shell")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession() error = %v", err)
	}
	session := snapshot.Repos[0].Worktrees[0].Sessions[0]
	if err := manager.UpdateSessionCWD(session.ID, cwd); err != nil {
		t.Fatalf("UpdateSessionCWD() error = %v", err)
	}

	manager.Shutdown()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.RemoveAll(cwd); err != nil {
		t.Fatalf("RemoveAll(cwd) error = %v", err)
	}

	restoreStore, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(reopen) error = %v", err)
	}
	t.Cleanup(func() {
		_ = restoreStore.Close()
	})

	restoreStarter := newFakeStarter()
	restoreManager := NewManager(newTestRegistry(t), os.Environ(), restoreStarter, &fakeSink{}, restoreStore)
	result, err := restoreManager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	restoredSession := result.Snapshot.Repos[0].Worktrees[0].Sessions[0]
	if restoredSession.CWDPath != root {
		t.Fatalf("restored cwd = %q, want %q", restoredSession.CWDPath, root)
	}
	if restoreStarter.spec(restoredSession.ID).CWD != root {
		t.Fatalf("restored start cwd = %q, want %q", restoreStarter.spec(restoredSession.ID).CWD, root)
	}
}

func TestManagerBootstrapRemovesRestoreSessionWhenPeerRegistrationFails(t *testing.T) {
	supportRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HELM_PEER_SUPPORT_ROOT", supportRoot)

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	storePath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	manager := NewManager(newPeerEnabledRegistry(t), os.Environ(), newFakeStarter(), &fakeSink{}, store)
	if err := manager.EnablePeerRuntime(executablePath); err != nil {
		t.Fatalf("EnablePeerRuntime() error = %v", err)
	}

	root := t.TempDir()
	if _, err := manager.CreateWorkspaceSession(root, "codex"); err != nil {
		t.Fatalf("CreateWorkspaceSession(codex) error = %v", err)
	}

	manager.Shutdown()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	restoreBaseStore, err := persist.OpenSQLiteStore(storePath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(reopen) error = %v", err)
	}
	t.Cleanup(func() {
		_ = restoreBaseStore.Close()
	})

	restoreStore := &failingStore{
		Store:                 restoreBaseStore,
		upsertPeerRegisterErr: errors.New("peer registration failed"),
	}
	restoreStarter := newFakeStarter()
	restoreManager := NewManager(newPeerEnabledRegistry(t), os.Environ(), restoreStarter, &fakeSink{}, restoreStore)
	if err := restoreManager.EnablePeerRuntime(executablePath); err != nil {
		t.Fatalf("EnablePeerRuntime(restore) error = %v", err)
	}

	result, err := restoreManager.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	if len(result.Snapshot.Repos) != 0 {
		t.Fatalf("repos = %#v, want empty after failed restore registration", result.Snapshot.Repos)
	}
	if result.RestoreNotice == "" || !strings.Contains(result.RestoreNotice, "Codex 1") {
		t.Fatalf("RestoreNotice = %q, want failed codex restore", result.RestoreNotice)
	}
	if len(restoreStarter.handles) != 1 {
		t.Fatalf("restoreStarter handles = %d, want 1 attempted restore", len(restoreStarter.handles))
	}
	for _, handle := range restoreStarter.handles {
		if !handle.closed {
			t.Fatalf("restored handle closed = false, want true")
		}
	}
	if len(restoreManager.Snapshot().Repos) != 0 {
		t.Fatalf("manager snapshot = %#v, want no lingering restored session", restoreManager.Snapshot().Repos)
	}
}

func TestManagerEmitSnapshotAndPeerStateOrdersSnapshotFirst(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	manager := NewManager(newTestRegistry(t), os.Environ(), newFakeStarter(), sink, newTestStore(t))

	manager.emitSnapshotAndPeerState(AppSnapshot{}, PeerStateDTO{}, true)

	if len(sink.events) != 2 {
		t.Fatalf("events = %#v, want two emissions", sink.events)
	}
	if sink.events[0].name != EventAppSnapshot || sink.events[1].name != EventPeerState {
		t.Fatalf("event order = %#v, want app snapshot then peer state", sink.events)
	}
}

func TestManagerDeleteAndClearPeerMessagesInvalidateCachedPeerSnapshot(t *testing.T) {
	supportRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HELM_PEER_SUPPORT_ROOT", supportRoot)

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	sink := &fakeSink{}
	store := newTestStore(t)
	manager := NewManager(newPeerEnabledRegistry(t), os.Environ(), newFakeStarter(), sink, store)
	if err := manager.EnablePeerRuntime(executablePath); err != nil {
		t.Fatalf("EnablePeerRuntime() error = %v", err)
	}
	t.Cleanup(manager.Shutdown)

	root := t.TempDir()
	created, err := manager.CreateWorkspaceSession(root, "codex")
	if err != nil {
		t.Fatalf("CreateWorkspaceSession(codex) error = %v", err)
	}

	var sessionID int
	for _, repo := range created.Repos {
		for _, worktree := range repo.Worktrees {
			for _, session := range worktree.Sessions {
				sessionID = session.ID
				break
			}
		}
	}
	if sessionID == 0 {
		t.Fatalf("created snapshot = %#v, want session", created)
	}

	deadline := time.Now().Add(2 * time.Second)
	peerID := ""
	for time.Now().Before(deadline) {
		registrations, err := store.ListPeerRegistrations(persist.PeerListFilter{
			Scope:       persist.PeerScopeMachine,
			IncludeSelf: true,
		})
		if err != nil {
			t.Fatalf("ListPeerRegistrations() error = %v", err)
		}
		if len(registrations) > 0 {
			peerID = registrations[0].PeerID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if peerID == "" {
		t.Fatalf("peer registration was not created for snapshot %#v", created)
	}

	createMessage := func(body string, createdAt time.Time) int64 {
		messageID, err := store.CreatePeerMessage(persist.PeerMessageRecord{
			FromPeerID: "sender",
			ToPeerID:   peerID,
			FromLabel:  "Sender",
			FromTitle:  "Sender",
			Body:       body,
			Status:     persist.PeerMessageStatusQueued,
			CreatedAt:  createdAt,
		})
		if err != nil {
			t.Fatalf("CreatePeerMessage(%q) error = %v", body, err)
		}
		return messageID
	}

	assertOutstandingCount := func(label string, want int) {
		snapshot := manager.Snapshot()
		for _, repo := range snapshot.Repos {
			for _, worktree := range repo.Worktrees {
				for _, session := range worktree.Sessions {
					if session.ID != sessionID {
						continue
					}
					if session.OutstandingPeerCount != want {
						t.Fatalf("%s outstanding count = %d, want %d", label, session.OutstandingPeerCount, want)
					}
					return
				}
			}
		}
		t.Fatalf("%s session %d not found in snapshot %#v", label, sessionID, snapshot)
	}

	now := time.Unix(1710000000, 0)
	firstID := createMessage("First message", now)
	manager.peerRuntime.invalidateSnapshot()
	warmDelete := manager.PeerState()
	if len(warmDelete.Messages) != 1 || warmDelete.Messages[0].ID != firstID {
		t.Fatalf("warm delete peer state = %#v, want first message cached", warmDelete)
	}

	afterDelete, err := manager.DeletePeerMessage(firstID)
	if err != nil {
		t.Fatalf("DeletePeerMessage() error = %v", err)
	}
	if len(afterDelete.Messages) != 0 {
		t.Fatalf("DeletePeerMessage() peer state = %#v, want no messages", afterDelete)
	}
	assertOutstandingCount("after delete", 0)

	secondID := createMessage("Second message", now.Add(time.Second))
	thirdID := createMessage("Third message", now.Add(2*time.Second))
	manager.peerRuntime.invalidateSnapshot()
	warmClear := manager.PeerState()
	if len(warmClear.Messages) != 2 {
		t.Fatalf("warm clear peer state len = %d, want 2", len(warmClear.Messages))
	}
	if warmClear.Messages[0].ID != thirdID || warmClear.Messages[1].ID != secondID {
		t.Fatalf("warm clear peer state = %#v, want second and third messages cached", warmClear)
	}

	afterClear, err := manager.ClearPeerMessages()
	if err != nil {
		t.Fatalf("ClearPeerMessages() error = %v", err)
	}
	if len(afterClear.Messages) != 0 {
		t.Fatalf("ClearPeerMessages() peer state = %#v, want no messages", afterClear)
	}
	assertOutstandingCount("after clear", 0)
}

func findWorktreeDTO(worktrees []WorktreeDTO, id int) *WorktreeDTO {
	for i := range worktrees {
		if worktrees[i].ID == id {
			return &worktrees[i]
		}
	}
	return nil
}

func newGitRepo(t *testing.T, gitPath string) string {
	t.Helper()

	root := t.TempDir()
	runGit(t, gitPath, root, "init")
	runGit(t, gitPath, root, "config", "user.email", "helm@example.com")
	runGit(t, gitPath, root, "config", "user.name", "Helm Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("helm\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runGit(t, gitPath, root, "add", "README.md")
	runGit(t, gitPath, root, "commit", "-m", "init")
	return root
}

func newGitRepoWithWorktree(t *testing.T, gitPath string) (string, string) {
	t.Helper()

	root := newGitRepo(t, gitPath)
	worktreeRoot := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-feature")
	runGit(t, gitPath, root, "worktree", "add", "-b", "feature/helm", worktreeRoot, "HEAD")
	return root, worktreeRoot
}

func samePath(t *testing.T, left, right string) bool {
	t.Helper()

	canonical := func(value string) string {
		resolved, err := filepath.EvalSymlinks(value)
		if err == nil {
			return filepath.Clean(resolved)
		}
		return filepath.Clean(value)
	}

	return canonical(left) == canonical(right)
}

func containsAll(value string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}

func newTestStore(t *testing.T) persist.Store {
	t.Helper()

	store, err := persist.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
