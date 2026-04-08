package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"helm-wails/internal/agent"
	"helm-wails/internal/peer"
	persist "helm-wails/internal/state"
)

type repoState struct {
	ID           int
	Name         string
	RootPath     string
	GitCommonDir string
	IsGitRepo    bool
}

type worktreeState struct {
	ID        int
	RepoID    int
	Name      string
	RootPath  string
	GitBranch string
	IsPrimary bool
}

type sessionState struct {
	ID           int
	StorageID    int64
	storageReady chan struct{} // closed when async DB write completes; nil for restored sessions
	WorktreeID   int
	AdapterID    string
	Label        string
	Title        string
	Status       string
	CWDPath      string
	Handle       Handle
	Closing      bool
	PeerLaunch   *peerLaunchState
	SupportDir   string
}

type managedSessionStartRequest struct {
	Plan                   shellLaunchPlan
	Repo                   *repoState
	SelectedWorktree       *worktreeState
	SessionID              int
	AdapterID              string
	Label                  string
	Title                  string
	StartPath              string
	StorageID              int64
	PersistLastUsedAgentID string
	Activate               bool
}

type Manager struct {
	mu sync.RWMutex

	registry     *agent.Registry
	inheritedEnv []string
	starter      Starter
	sink         EventSink
	store        persist.Store
	peerRuntime  *peerRuntime

	repoSeq     int
	worktreeSeq int
	sessionSeq  int

	repos     []*repoState
	worktrees []*worktreeState
	sessions  []*sessionState

	activeRepoID     int
	activeWorktreeID int
	activeSessionID  int
	lastUsedAgentID  string
	shuttingDown     bool
	bootstrapped     bool
	uiState          UIStateDTO

	availableAgents   []AgentDTO
	availableAgentIDs map[string]struct{}
	nextCreatedAt     time.Time

	shellCacheMu sync.RWMutex
	shellCache   map[shellAdapterCacheKey]cachedShellAdapterDefinitions
}

func NewManager(registry *agent.Registry, inheritedEnv []string, starter Starter, sink EventSink, store persist.Store) *Manager {
	if starter == nil {
		starter = NewPTYStarter()
	}
	if sink == nil {
		sink = EventSinkFunc(func(string, any) {})
	}

	availableAgents := availableAgentDTOs(registry)
	availableAgentIDs := make(map[string]struct{}, len(availableAgents))
	for _, item := range availableAgents {
		availableAgentIDs[item.ID] = struct{}{}
	}

	return &Manager{
		registry:          registry,
		inheritedEnv:      append([]string(nil), inheritedEnv...),
		starter:           starter,
		sink:              sink,
		store:             store,
		lastUsedAgentID:   "shell",
		uiState:           toUIStateDTO(persist.DefaultUIState()),
		availableAgents:   availableAgents,
		availableAgentIDs: availableAgentIDs,
		shellCache:        make(map[shellAdapterCacheKey]cachedShellAdapterDefinitions),
	}
}

func (m *Manager) EnablePeerRuntime(executablePath string) error {
	if m.store == nil {
		return fmt.Errorf("state store is not configured")
	}

	support, err := peer.NewSupportManager(executablePath)
	if err != nil {
		return err
	}

	var runtime *peerRuntime
	runtime = newPeerRuntime(
		m.store,
		support,
		func(sessionID int, data string) error {
			m.mu.RLock()
			session := m.findSessionByIDLocked(sessionID)
			m.mu.RUnlock()
			if session == nil {
				return fmt.Errorf("session %d not found", sessionID)
			}
			return session.Handle.Write(data)
		},
		func() {
			m.mu.RLock()
			shuttingDown := m.shuttingDown
			m.mu.RUnlock()
			if shuttingDown {
				return
			}
			m.emitSnapshotAndPeerState(m.Snapshot(), runtime.snapshot(), true)
		},
	)

	m.mu.Lock()
	m.peerRuntime = runtime
	m.mu.Unlock()

	runtime.start()
	return nil
}

func (m *Manager) PeerState() PeerStateDTO {
	m.mu.RLock()
	runtime := m.peerRuntime
	m.mu.RUnlock()
	if runtime == nil {
		return PeerStateDTO{}
	}
	return runtime.snapshot()
}

func (m *Manager) Snapshot() AppSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshotLocked()
}

func (m *Manager) ActiveAgentSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, session := range m.sessions {
		if session == nil || session.Closing {
			continue
		}
		if session.Status != "running" {
			continue
		}
		if session.AdapterID == "shell" {
			continue
		}
		count++
	}
	return count
}

func (m *Manager) Bootstrap() (BootstrapResult, error) {
	if m.store == nil {
		return BootstrapResult{}, fmt.Errorf("state store is not configured")
	}

	m.mu.RLock()
	if m.bootstrapped {
		result := BootstrapResult{
			Snapshot:  m.snapshotLocked(),
			UIState:   m.uiState,
			PeerState: m.peerStateLocked(),
		}
		m.mu.RUnlock()
		return result, nil
	}
	m.mu.RUnlock()

	persisted, err := m.store.Load()
	if err != nil {
		return BootstrapResult{}, err
	}

	restoreNotice := m.restorePersistedSessions(persisted)

	m.mu.Lock()
	m.uiState = toUIStateDTO(persisted.UIState)
	m.bootstrapped = true
	result := BootstrapResult{
		Snapshot:      m.snapshotLocked(),
		UIState:       m.uiState,
		RestoreNotice: restoreNotice,
		PeerState:     m.peerStateLocked(),
	}
	m.mu.Unlock()
	return result, nil
}

func (m *Manager) SaveUIState(uiState UIStateDTO) error {
	if m.store == nil {
		return fmt.Errorf("state store is not configured")
	}

	uiState.UtilityPanelTab = normalizeUtilityPanelTab(uiState.UtilityPanelTab)
	uiState.TerminalFontSize = normalizeTerminalFontSize(uiState.TerminalFontSize)

	if err := m.store.SaveUIPreferences(persist.UIState{
		SidebarOpen:       uiState.SidebarOpen,
		SidebarWidth:      uiState.SidebarWidth,
		DiffPanelOpen:     uiState.DiffPanelOpen,
		DiffPanelWidth:    uiState.DiffPanelWidth,
		TerminalFontSize:  uiState.TerminalFontSize,
		UtilityPanelTab:   uiState.UtilityPanelTab,
		CollapsedRepoKeys: append([]string(nil), uiState.CollapsedRepoKeys...),
	}); err != nil {
		return err
	}

	m.mu.Lock()
	m.uiState = uiState
	m.mu.Unlock()
	return nil
}

func (m *Manager) DeletePeerMessage(messageID int64) (PeerStateDTO, error) {
	if m.store == nil {
		return PeerStateDTO{}, fmt.Errorf("state store is not configured")
	}
	if messageID <= 0 {
		return PeerStateDTO{}, fmt.Errorf("message id must be greater than zero")
	}
	if err := m.store.DeletePeerMessage(messageID); err != nil {
		return PeerStateDTO{}, err
	}
	if m.peerRuntime != nil {
		m.peerRuntime.invalidateSnapshot()
	}
	return m.emitPeerStateRefresh(), nil
}

func (m *Manager) ClearPeerMessages() (PeerStateDTO, error) {
	if m.store == nil {
		return PeerStateDTO{}, fmt.Errorf("state store is not configured")
	}
	if err := m.store.ClearPeerMessages(); err != nil {
		return PeerStateDTO{}, err
	}
	if m.peerRuntime != nil {
		m.peerRuntime.invalidateSnapshot()
	}
	return m.emitPeerStateRefresh(), nil
}

func (m *Manager) UpdateSessionCWD(sessionID int, cwdPath string) error {
	if m.store == nil {
		return fmt.Errorf("state store is not configured")
	}

	m.awaitSessionStorage(sessionID)

	m.mu.Lock()
	session := m.findSessionByIDLocked(sessionID)
	if session == nil {
		m.mu.Unlock()
		return fmt.Errorf("session %d not found", sessionID)
	}
	worktree := m.findWorktreeByIDLocked(session.WorktreeID)
	if worktree == nil {
		m.mu.Unlock()
		return fmt.Errorf("worktree %d not found", session.WorktreeID)
	}

	normalizedCWD, _ := normalizeSessionStartPath(worktree.RootPath, cwdPath)
	session.CWDPath = normalizedCWD
	storageID := session.StorageID
	m.mu.Unlock()

	return m.store.UpdateSessionCWD(storageID, normalizedCWD, time.Now())
}

func (m *Manager) UpdateSessionMode(sessionID int, adapterID string) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	m.awaitSessionStorage(sessionID)

	adapterID = strings.TrimSpace(adapterID)
	if adapterID == "" {
		return AppSnapshot{}, fmt.Errorf("adapter id is required")
	}

	label, family, peerEnabled, err := m.sessionModeMetadata(adapterID)
	if err != nil {
		return AppSnapshot{}, err
	}

	m.mu.RLock()
	session := m.findSessionByIDLocked(sessionID)
	if session == nil {
		m.mu.RUnlock()
		return AppSnapshot{}, fmt.Errorf("session %d not found", sessionID)
	}
	if session.AdapterID == adapterID {
		snapshot := m.snapshotLocked()
		m.mu.RUnlock()
		return snapshot, nil
	}
	worktree := m.findWorktreeByIDLocked(session.WorktreeID)
	if worktree == nil {
		m.mu.RUnlock()
		return AppSnapshot{}, fmt.Errorf("worktree %d not found", session.WorktreeID)
	}
	repo := m.findRepoByIDLocked(worktree.RepoID)
	storageID := session.StorageID
	title := session.Title
	prevAdapterID := session.AdapterID
	prevLabel := session.Label
	prevLastUsed := m.lastUsedAgentID
	peerLaunch := session.PeerLaunch
	rootPath := worktree.RootPath
	repoKey := repoPersistenceKey(repo)
	m.mu.RUnlock()

	now := time.Now()
	if adapterID != "shell" {
		if err := m.store.SetLastUsedAgentID(adapterID); err != nil {
			return AppSnapshot{}, err
		}
	}
	if err := m.store.UpdateSessionMode(storageID, adapterID, label, now); err != nil {
		if adapterID != "shell" {
			_ = m.store.SetLastUsedAgentID(prevLastUsed)
		}
		return AppSnapshot{}, err
	}

	m.mu.Lock()
	session = m.findSessionByIDLocked(sessionID)
	if session == nil {
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		return snapshot, nil
	}
	session.AdapterID = adapterID
	session.Label = label
	if adapterID != "shell" {
		m.lastUsedAgentID = adapterID
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()

	if m.peerRuntime != nil {
		if peerEnabled && peerLaunch != nil {
			if err := m.peerRuntime.registerSession(
				sessionID,
				peerLaunch,
				rootPath,
				repoKey,
				adapterID,
				family,
				label,
				title,
			); err != nil {
				_ = m.store.UpdateSessionMode(storageID, prevAdapterID, prevLabel, now)
				if adapterID != "shell" {
					_ = m.store.SetLastUsedAgentID(prevLastUsed)
				}

				m.mu.Lock()
				if current := m.findSessionByIDLocked(sessionID); current != nil {
					current.AdapterID = prevAdapterID
					current.Label = prevLabel
					m.lastUsedAgentID = prevLastUsed
					snapshot = m.snapshotLocked()
				}
				m.mu.Unlock()
				return snapshot, err
			}
		} else {
			m.peerRuntime.unregisterSession(sessionID, "peer session is idle")
		}

		m.mu.RLock()
		snapshot = m.snapshotLocked()
		m.mu.RUnlock()
	}

	m.emitSnapshotAndPeerState(snapshot, m.PeerState(), m.peerRuntime != nil)

	return snapshot, nil
}

func (m *Manager) reserveSessionSlot(selection repoSelection, label, desiredTitle string) (*repoState, *worktreeState, int, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	repo := m.upsertRepoLocked(selection.Repo)
	selectedWorktree := m.syncWorktreesLocked(repo.ID, selection.Repo.Worktrees, selection.SelectedWorktree.RootPath)
	m.sessionSeq++
	sessionID := m.sessionSeq
	title := strings.TrimSpace(desiredTitle)
	if title == "" {
		title = m.nextSessionTitleLocked(selectedWorktree.ID, label)
	}

	return repo, selectedWorktree, sessionID, title
}

func (m *Manager) startShellBackedHandle(plan shellLaunchPlan, sessionID, worktreeID int) (Handle, error) {
	handle, err := m.starter.Start(plan.ShellSpec, StartMeta{
		SessionID:  sessionID,
		WorktreeID: worktreeID,
	}, m.sink, m.handleExit)
	if err != nil {
		cleanupSessionSupportDir(plan.SupportDir)
		return nil, err
	}
	if !plan.ModeTracking && plan.ManualBootCommand != "" {
		if err := handle.Write(plan.ManualBootCommand + "\n"); err != nil {
			_ = handle.Close()
			cleanupSessionSupportDir(plan.SupportDir)
			return nil, err
		}
	}
	return handle, nil
}

func (m *Manager) startManagedSession(request managedSessionStartRequest) (*sessionState, AppSnapshot, error) {
	handle, err := m.startShellBackedHandle(request.Plan, request.SessionID, request.SelectedWorktree.ID)
	if err != nil {
		return nil, AppSnapshot{}, err
	}

	session := &sessionState{
		ID:         request.SessionID,
		StorageID:  request.StorageID,
		WorktreeID: request.SelectedWorktree.ID,
		AdapterID:  request.AdapterID,
		Label:      request.Label,
		Title:      request.Title,
		Status:     "running",
		CWDPath:    request.StartPath,
		Handle:     handle,
		PeerLaunch: request.Plan.PeerLaunch,
		SupportDir: request.Plan.SupportDir,
	}

	// Restored session (StorageID already set): synchronous peer registration.
	if request.StorageID != 0 {
		if m.peerRuntime != nil && request.Plan.PeerLaunch != nil && request.Plan.RequestedSpec.PeerEnabled {
			if err := m.peerRuntime.registerSession(
				request.SessionID,
				request.Plan.PeerLaunch,
				request.SelectedWorktree.RootPath,
				repoPersistenceKey(request.Repo),
				request.AdapterID,
				request.Plan.RequestedSpec.Family,
				request.Label,
				request.Title,
			); err != nil {
				_ = handle.Close()
				cleanupSessionSupportDir(request.Plan.SupportDir)
				return nil, AppSnapshot{}, err
			}
		}

		snapshot := m.insertSessionLocked(session, request)
		return session, snapshot, nil
	}

	// New session: add to memory immediately, defer DB write + peer registration.
	storageReady := make(chan struct{})
	session.storageReady = storageReady

	var snapshot AppSnapshot
	var finalize deferredStorageWork
	m.mu.Lock()
	finalize = newDeferredStorageWork(request, m.nextSessionCreatedAtLocked())
	m.sessions = append(m.sessions, session)
	if request.Activate {
		m.activeRepoID = request.Repo.ID
		m.activeWorktreeID = request.SelectedWorktree.ID
		m.activeSessionID = request.SessionID
		if request.PersistLastUsedAgentID != "" {
			m.lastUsedAgentID = request.PersistLastUsedAgentID
		}
		snapshot = m.snapshotLocked()
	}
	m.mu.Unlock()

	go m.finalizeSessionStorage(session, finalize, storageReady)

	return session, snapshot, nil
}

func (m *Manager) insertSessionLocked(session *sessionState, request managedSessionStartRequest) AppSnapshot {
	var snapshot AppSnapshot
	m.mu.Lock()
	m.sessions = append(m.sessions, session)
	if request.Activate {
		m.activeRepoID = request.Repo.ID
		m.activeWorktreeID = request.SelectedWorktree.ID
		m.activeSessionID = request.SessionID
		if request.PersistLastUsedAgentID != "" {
			m.lastUsedAgentID = request.PersistLastUsedAgentID
		}
		snapshot = m.snapshotLocked()
	}
	m.mu.Unlock()
	return snapshot
}

type deferredStorageWork struct {
	sessionID        int
	worktreeRootPath string
	adapterID        string
	label            string
	title            string
	startPath        string
	lastUsedAgentID  string
	peerLaunch       *peerLaunchState
	peerEnabled      bool
	peerFamily       string
	repoKey          string
	createdAt        time.Time
}

func newDeferredStorageWork(request managedSessionStartRequest, createdAt time.Time) deferredStorageWork {
	return deferredStorageWork{
		sessionID:        request.SessionID,
		worktreeRootPath: request.SelectedWorktree.RootPath,
		adapterID:        request.AdapterID,
		label:            request.Label,
		title:            request.Title,
		startPath:        request.StartPath,
		lastUsedAgentID:  request.PersistLastUsedAgentID,
		peerLaunch:       request.Plan.PeerLaunch,
		peerEnabled:      request.Plan.RequestedSpec.PeerEnabled,
		peerFamily:       request.Plan.RequestedSpec.Family,
		repoKey:          repoPersistenceKey(request.Repo),
		createdAt:        createdAt,
	}
}

func (m *Manager) finalizeSessionStorage(session *sessionState, work deferredStorageWork, ready chan struct{}) {
	defer close(ready)
	storageID, err := m.store.CreateSession(persist.SessionRecord{
		WorktreeRootPath: work.worktreeRootPath,
		AdapterID:        work.adapterID,
		Label:            work.label,
		Title:            work.title,
		CWDPath:          work.startPath,
		CreatedAt:        work.createdAt,
		LastActiveAt:     work.createdAt,
	}, work.lastUsedAgentID)
	if err != nil {
		return
	}

	m.mu.Lock()
	session.StorageID = storageID
	isStillActive := m.activeSessionID == work.sessionID
	m.mu.Unlock()

	if isStillActive {
		_ = m.store.SetActiveSessionID(storageID)
	}

	if m.peerRuntime != nil && work.peerLaunch != nil && work.peerEnabled {
		_ = m.peerRuntime.registerSession(
			work.sessionID,
			work.peerLaunch,
			work.worktreeRootPath,
			work.repoKey,
			work.adapterID,
			work.peerFamily,
			work.label,
			work.title,
		)
	}
}

// awaitSessionStorage blocks until the background DB write for the given session
// has completed. This is a no-op for restored sessions (storageReady is nil).
func (m *Manager) awaitSessionStorage(sessionID int) {
	m.mu.RLock()
	session := m.findSessionByIDLocked(sessionID)
	var ready chan struct{}
	if session != nil {
		ready = session.storageReady
	}
	m.mu.RUnlock()
	if ready != nil {
		<-ready
	}
}

func (m *Manager) CreateWorkspaceSession(rootPath, agentID string) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return AppSnapshot{}, fmt.Errorf("workspace path is required")
	}

	selection, err := describeRepoSelection(rootPath)
	if err != nil {
		return AppSnapshot{}, err
	}

	requestedAgentID, err := m.resolveRequestedAgent(agentID)
	if err != nil {
		return AppSnapshot{}, err
	}

	startPath, err := normalizeSessionStartPath(selection.SelectedWorktree.RootPath, selection.SelectedWorktree.RootPath)
	if err != nil {
		return AppSnapshot{}, err
	}

	plan, err := m.prepareShellBackedLaunch(selection.SelectedWorktree.RootPath, startPath, requestedAgentID, false)
	if err != nil {
		return AppSnapshot{}, err
	}

	repo, selectedWorktree, sessionID, title := m.reserveSessionSlot(selection, plan.RequestedSpec.Label, "")
	_, snapshot, err := m.startManagedSession(managedSessionStartRequest{
		Plan:                   plan,
		Repo:                   repo,
		SelectedWorktree:       selectedWorktree,
		SessionID:              sessionID,
		AdapterID:              requestedAgentID,
		Label:                  plan.RequestedSpec.Label,
		Title:                  title,
		StartPath:              startPath,
		PersistLastUsedAgentID: persistedLastUsedAgentID(requestedAgentID),
		Activate:               true,
	})
	if err != nil {
		return AppSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) CreateSession(worktreeID int, agentID string) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	requestedAgentID, err := m.resolveRequestedAgent(agentID)
	if err != nil {
		return AppSnapshot{}, err
	}

	m.mu.RLock()
	selection, err := m.repoSelectionFromMemoryLocked(worktreeID)
	m.mu.RUnlock()
	if err != nil {
		return AppSnapshot{}, err
	}

	startPath, err := normalizeSessionStartPath(selection.SelectedWorktree.RootPath, selection.SelectedWorktree.RootPath)
	if err != nil {
		return AppSnapshot{}, err
	}

	plan, err := m.prepareShellBackedLaunch(selection.SelectedWorktree.RootPath, startPath, requestedAgentID, false)
	if err != nil {
		return AppSnapshot{}, err
	}

	repo, selectedWorktree, sessionID, title := m.reserveSessionSlot(selection, plan.RequestedSpec.Label, "")
	_, snapshot, err := m.startManagedSession(managedSessionStartRequest{
		Plan:                   plan,
		Repo:                   repo,
		SelectedWorktree:       selectedWorktree,
		SessionID:              sessionID,
		AdapterID:              requestedAgentID,
		Label:                  plan.RequestedSpec.Label,
		Title:                  title,
		StartPath:              startPath,
		PersistLastUsedAgentID: persistedLastUsedAgentID(requestedAgentID),
		Activate:               true,
	})
	if err != nil {
		return AppSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) repoSelectionFromMemoryLocked(worktreeID int) (repoSelection, error) {
	worktree := m.findWorktreeByIDLocked(worktreeID)
	if worktree == nil {
		return repoSelection{}, fmt.Errorf("worktree %d not found", worktreeID)
	}

	repo := m.findRepoByIDLocked(worktree.RepoID)
	if repo == nil {
		return repoSelection{}, fmt.Errorf("repo for worktree %d not found", worktreeID)
	}

	var worktreeDescs []worktreeDescriptor
	for _, wt := range m.worktrees {
		if wt.RepoID == repo.ID {
			worktreeDescs = append(worktreeDescs, worktreeDescriptor{
				Name:      wt.Name,
				RootPath:  wt.RootPath,
				GitBranch: wt.GitBranch,
				IsPrimary: wt.IsPrimary,
			})
		}
	}

	return repoSelection{
		Repo: repoDescriptor{
			Name:         repo.Name,
			RootPath:     repo.RootPath,
			GitCommonDir: repo.GitCommonDir,
			IsGitRepo:    repo.IsGitRepo,
			Worktrees:    worktreeDescs,
		},
		SelectedWorktree: worktreeDescriptor{
			Name:      worktree.Name,
			RootPath:  worktree.RootPath,
			GitBranch: worktree.GitBranch,
			IsPrimary: worktree.IsPrimary,
		},
	}, nil
}

func (m *Manager) CreateWorktreeSession(repoID int, request WorktreeCreateRequest) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	requestedAgentID, err := m.resolveRequestedAgent(request.AgentID)
	if err != nil {
		return AppSnapshot{}, err
	}

	m.mu.RLock()
	repo := m.findRepoByIDLocked(repoID)
	activeWorktree := m.findWorktreeByIDLocked(m.activeWorktreeID)
	m.mu.RUnlock()
	if repo == nil {
		return AppSnapshot{}, fmt.Errorf("repo %d not found", repoID)
	}
	if !repo.IsGitRepo {
		return AppSnapshot{}, fmt.Errorf("worktrees are only available for git repositories")
	}

	createRequest := request
	createRequest.AgentID = requestedAgentID
	if strings.TrimSpace(createRequest.SourceRef) == "" {
		if activeWorktree != nil && activeWorktree.RepoID == repo.ID {
			createRequest.SourceRef = activeWorktree.GitBranch
			if createRequest.SourceRef == noGitBranch || createRequest.SourceRef == detachedHead {
				createRequest.SourceRef = defaultSource
			}
		}
		if strings.TrimSpace(createRequest.SourceRef) == "" {
			createRequest.SourceRef = defaultSource
		}
	}
	createRequest.SourceRef = normalizeWorktreeSourceRef(createRequest.SourceRef)

	createdPath, err := CreateWorktree(repo.RootPath, createRequest)
	if err != nil {
		return AppSnapshot{}, err
	}

	selection, err := describeRepoSelection(createdPath)
	if err != nil {
		return AppSnapshot{}, err
	}

	startPath, err := normalizeSessionStartPath(selection.SelectedWorktree.RootPath, selection.SelectedWorktree.RootPath)
	if err != nil {
		return AppSnapshot{}, err
	}

	plan, err := m.prepareShellBackedLaunch(selection.SelectedWorktree.RootPath, startPath, requestedAgentID, false)
	if err != nil {
		return AppSnapshot{}, err
	}

	repo, selectedWorktree, sessionID, title := m.reserveSessionSlot(selection, plan.RequestedSpec.Label, "")
	_, snapshot, err := m.startManagedSession(managedSessionStartRequest{
		Plan:                   plan,
		Repo:                   repo,
		SelectedWorktree:       selectedWorktree,
		SessionID:              sessionID,
		AdapterID:              requestedAgentID,
		Label:                  plan.RequestedSpec.Label,
		Title:                  title,
		StartPath:              startPath,
		PersistLastUsedAgentID: persistedLastUsedAgentID(requestedAgentID),
		Activate:               true,
	})
	if err != nil {
		return AppSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) ActivateSession(sessionID int) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	m.awaitSessionStorage(sessionID)

	m.mu.RLock()
	session := m.findSessionByIDLocked(sessionID)
	if session == nil {
		m.mu.RUnlock()
		return AppSnapshot{}, fmt.Errorf("session %d not found", sessionID)
	}

	worktree := m.findWorktreeByIDLocked(session.WorktreeID)
	if worktree == nil {
		m.mu.RUnlock()
		return AppSnapshot{}, fmt.Errorf("worktree %d not found", session.WorktreeID)
	}
	storageID := session.StorageID
	m.mu.RUnlock()

	if err := m.store.SetActiveSessionID(storageID); err != nil {
		return AppSnapshot{}, err
	}

	m.mu.Lock()
	session = m.findSessionByIDLocked(sessionID)
	if session == nil {
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		return snapshot, nil
	}
	worktree = m.findWorktreeByIDLocked(session.WorktreeID)
	if worktree == nil {
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		return snapshot, nil
	}

	m.activeSessionID = sessionID
	m.activeWorktreeID = worktree.ID
	m.activeRepoID = worktree.RepoID
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	return snapshot, nil
}

func (m *Manager) KillSession(sessionID int) (AppSnapshot, error) {
	if m.store == nil {
		return AppSnapshot{}, fmt.Errorf("state store is not configured")
	}

	m.awaitSessionStorage(sessionID)

	m.mu.Lock()
	session := m.findSessionByIDLocked(sessionID)
	if session == nil {
		m.mu.Unlock()
		return AppSnapshot{}, fmt.Errorf("session %d not found", sessionID)
	}
	session.Closing = true
	worktree := m.findWorktreeByIDLocked(session.WorktreeID)
	worktreeID := session.WorktreeID
	storageID := session.StorageID
	repoID := 0
	if worktree != nil {
		repoID = worktree.RepoID
	}
	nextActiveStorageID := m.projectActiveSessionStorageIDAfterRemovalLocked(worktreeID, repoID, sessionID)
	handle := session.Handle
	m.mu.Unlock()

	if err := handle.Close(); err != nil {
		m.mu.Lock()
		if current := m.findSessionByIDLocked(sessionID); current != nil {
			current.Closing = false
		}
		m.mu.Unlock()
		return AppSnapshot{}, err
	}

	if err := m.store.DeleteSession(storageID, nextActiveStorageID); err != nil {
		m.mu.Lock()
		if current := m.findSessionByIDLocked(sessionID); current != nil {
			current.Closing = false
		}
		m.mu.Unlock()
		return AppSnapshot{}, err
	}
	if m.peerRuntime != nil {
		m.peerRuntime.unregisterSession(sessionID, "peer session was closed")
	}

	m.mu.Lock()
	session = m.findSessionByIDLocked(sessionID)
	if session == nil {
		snapshot := m.snapshotLocked()
		m.mu.Unlock()
		return snapshot, nil
	}

	m.removeSessionLocked(sessionID)
	m.removeRepoIfEmptyLocked(repoID)
	m.rebalanceActiveLocked(worktreeID, repoID, sessionID)

	snapshot := m.snapshotLocked()
	m.mu.Unlock()

	m.sink.Emit(EventSessionLifecycle, SessionLifecycleEvent{
		SessionID:  sessionID,
		WorktreeID: worktreeID,
		Status:     "killed",
		ExitCode:   -1,
	})
	m.sink.Emit(EventAppSnapshot, snapshot)

	return snapshot, nil
}

func (m *Manager) SendInput(sessionID int, data string) error {
	m.mu.RLock()
	session := m.findSessionByIDLocked(sessionID)
	m.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("session %d not found", sessionID)
	}
	return session.Handle.Write(data)
}

func (m *Manager) ResizeSession(sessionID int, cols, rows int) error {
	m.mu.RLock()
	session := m.findSessionByIDLocked(sessionID)
	m.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("session %d not found", sessionID)
	}
	return session.Handle.Resize(cols, rows)
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return
	}
	m.shuttingDown = true
	handles := make([]Handle, 0, len(m.sessions))
	sessionIDs := make([]int, 0, len(m.sessions))
	supportDirs := make([]string, 0, len(m.sessions))
	pendingStorage := make([]chan struct{}, 0)
	for _, session := range m.sessions {
		if session.Handle != nil {
			handles = append(handles, session.Handle)
		}
		sessionIDs = append(sessionIDs, session.ID)
		if session.SupportDir != "" {
			supportDirs = append(supportDirs, session.SupportDir)
		}
		if session.storageReady != nil {
			pendingStorage = append(pendingStorage, session.storageReady)
		}
	}
	m.repos = nil
	m.worktrees = nil
	m.sessions = nil
	m.activeRepoID = 0
	m.activeWorktreeID = 0
	m.activeSessionID = 0
	m.mu.Unlock()

	for _, ready := range pendingStorage {
		<-ready
	}

	if m.peerRuntime != nil {
		for _, sessionID := range sessionIDs {
			m.peerRuntime.unregisterSession(sessionID, "peer session is offline")
		}
		m.peerRuntime.stop()
	}

	for _, handle := range handles {
		_ = handle.Close()
	}
	for _, supportDir := range supportDirs {
		cleanupSessionSupportDir(supportDir)
	}
}

func (m *Manager) handleExit(info ExitInfo) {
	m.awaitSessionStorage(info.SessionID)

	m.mu.Lock()
	if m.shuttingDown {
		worktree := m.findWorktreeByIDLocked(info.WorktreeID)
		repoID := 0
		if worktree != nil {
			repoID = worktree.RepoID
		}
		m.removeSessionLocked(info.SessionID)
		m.removeRepoIfEmptyLocked(repoID)
		m.mu.Unlock()
		return
	}

	session := m.findSessionByIDLocked(info.SessionID)
	if session == nil {
		m.mu.Unlock()
		return
	}
	if session.Closing {
		m.mu.Unlock()
		return
	}

	worktree := m.findWorktreeByIDLocked(info.WorktreeID)
	repoID := 0
	storageID := session.StorageID
	if worktree != nil {
		repoID = worktree.RepoID
	}

	m.removeSessionLocked(info.SessionID)
	m.removeRepoIfEmptyLocked(repoID)
	m.rebalanceActiveLocked(info.WorktreeID, repoID, info.SessionID)
	nextActiveStorageID := m.activeSessionStorageIDLocked()

	snapshot := m.snapshotLocked()
	m.mu.Unlock()

	status := "exited"
	errText := ""
	if info.Err != nil {
		status = "error"
		errText = info.Err.Error()
	}
	if err := m.store.DeleteSession(storageID, nextActiveStorageID); err != nil {
		if errText != "" {
			errText += "; "
		}
		errText += err.Error()
	}
	if m.peerRuntime != nil {
		m.peerRuntime.unregisterSession(info.SessionID, "peer session exited")
	}

	m.sink.Emit(EventSessionLifecycle, SessionLifecycleEvent{
		SessionID:  info.SessionID,
		WorktreeID: info.WorktreeID,
		Status:     status,
		ExitCode:   info.ExitCode,
		Error:      errText,
	})
	m.sink.Emit(EventAppSnapshot, snapshot)
}

func (m *Manager) snapshotLocked() AppSnapshot {
	peerDecorations := m.peerStateDecorationsLocked()
	worktreesByID := make(map[int]*worktreeState, len(m.worktrees))
	worktreesByRepo := make(map[int][]*worktreeState, len(m.worktrees))
	for _, worktree := range m.worktrees {
		worktreesByID[worktree.ID] = worktree
		worktreesByRepo[worktree.RepoID] = append(worktreesByRepo[worktree.RepoID], worktree)
	}

	repoHasSession := make(map[int]bool)
	sessionsByWorktree := make(map[int][]SessionDTO, len(worktreesByID))
	for _, session := range m.sessions {
		worktree := worktreesByID[session.WorktreeID]
		if worktree == nil {
			continue
		}
		repoHasSession[worktree.RepoID] = true
		decoration := peerDecorations[session.ID]
		sessionsByWorktree[worktree.ID] = append(sessionsByWorktree[worktree.ID], SessionDTO{
			ID:                   session.ID,
			WorktreeID:           session.WorktreeID,
			AdapterID:            session.AdapterID,
			Label:                session.Label,
			Title:                session.Title,
			Status:               session.Status,
			CWDPath:              session.CWDPath,
			PeerID:               decoration.PeerID,
			PeerCapable:          decoration.PeerCapable,
			PeerSummary:          decoration.PeerSummary,
			OutstandingPeerCount: decoration.OutstandingPeerCount,
		})
	}

	repoStates := make([]*repoState, 0, len(m.repos))
	for _, repo := range m.repos {
		if repoHasSession[repo.ID] {
			repoStates = append(repoStates, repo)
		}
	}

	sort.SliceStable(repoStates, func(i, j int) bool {
		return repoStates[i].Name < repoStates[j].Name
	})

	repos := make([]RepoDTO, 0, len(repoStates))
	for _, repo := range repoStates {
		worktreeStates := append([]*worktreeState(nil), worktreesByRepo[repo.ID]...)
		sort.SliceStable(worktreeStates, func(i, j int) bool {
			if worktreeStates[i].IsPrimary != worktreeStates[j].IsPrimary {
				return worktreeStates[i].IsPrimary
			}
			return worktreeStates[i].RootPath < worktreeStates[j].RootPath
		})
		worktrees := make([]WorktreeDTO, 0, len(worktreeStates))
		for _, worktree := range worktreeStates {
			sessionItems := sessionsByWorktree[worktree.ID]
			if sessionItems == nil {
				sessionItems = []SessionDTO{}
			}
			worktrees = append(worktrees, WorktreeDTO{
				ID:        worktree.ID,
				RepoID:    worktree.RepoID,
				Name:      worktree.Name,
				RootPath:  worktree.RootPath,
				GitBranch: worktree.GitBranch,
				IsPrimary: worktree.IsPrimary,
				Sessions:  sessionItems,
			})
		}
		repos = append(repos, RepoDTO{
			ID:             repo.ID,
			Name:           repo.Name,
			RootPath:       repo.RootPath,
			GitCommonDir:   repo.GitCommonDir,
			IsGitRepo:      repo.IsGitRepo,
			PersistenceKey: repoPersistenceKey(repo),
			Worktrees:      worktrees,
		})
	}

	return AppSnapshot{
		Repos:            repos,
		ActiveRepoID:     m.activeRepoID,
		ActiveWorktreeID: m.activeWorktreeID,
		ActiveSessionID:  m.activeSessionID,
		AvailableAgents:  append([]AgentDTO(nil), m.availableAgents...),
		LastUsedAgentID:  m.lastUsedAgentID,
	}
}

func (m *Manager) peerStateLocked() PeerStateDTO {
	if m.peerRuntime == nil {
		return PeerStateDTO{}
	}
	return m.peerRuntime.snapshot()
}

func (m *Manager) emitSnapshotAndPeerState(snapshot AppSnapshot, peerState PeerStateDTO, includePeerState bool) {
	if m.sink == nil {
		return
	}
	m.sink.Emit(EventAppSnapshot, snapshot)
	if includePeerState {
		m.sink.Emit(EventPeerState, peerState)
	}
}

func (m *Manager) emitPeerStateRefresh() PeerStateDTO {
	m.mu.RLock()
	peerState := m.peerStateLocked()
	snapshot := m.snapshotLocked()
	m.mu.RUnlock()

	m.emitSnapshotAndPeerState(snapshot, peerState, true)
	return peerState
}

func (m *Manager) peerStateDecorationsLocked() map[int]sessionPeerDecoration {
	if m.peerRuntime == nil {
		return map[int]sessionPeerDecoration{}
	}
	return m.peerRuntime.sessionDecorations()
}

func (m *Manager) sessionDTOsForWorktreeLocked(worktreeID int, peerDecorations map[int]sessionPeerDecoration) []SessionDTO {
	items := make([]SessionDTO, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session.WorktreeID != worktreeID {
			continue
		}
		decoration := peerDecorations[session.ID]
		items = append(items, SessionDTO{
			ID:                   session.ID,
			WorktreeID:           session.WorktreeID,
			AdapterID:            session.AdapterID,
			Label:                session.Label,
			Title:                session.Title,
			Status:               session.Status,
			CWDPath:              session.CWDPath,
			PeerID:               decoration.PeerID,
			PeerCapable:          decoration.PeerCapable,
			PeerSummary:          decoration.PeerSummary,
			OutstandingPeerCount: decoration.OutstandingPeerCount,
		})
	}
	return items
}

func (m *Manager) activeSessionStorageIDLocked() int64 {
	activeSession := m.findSessionByIDLocked(m.activeSessionID)
	if activeSession == nil {
		return 0
	}
	return activeSession.StorageID
}

func (m *Manager) projectActiveSessionStorageIDAfterRemovalLocked(worktreeID, repoID, sessionID int) int64 {
	if m.activeSessionID != sessionID {
		return m.activeSessionStorageIDLocked()
	}
	if next := m.lastSessionForWorktreeExceptLocked(worktreeID, sessionID); next != nil {
		return next.StorageID
	}
	if next := m.lastSessionForRepoExceptLocked(repoID, sessionID); next != nil {
		return next.StorageID
	}
	if next := m.lastSessionExceptLocked(sessionID); next != nil {
		return next.StorageID
	}
	return 0
}

func (m *Manager) resolveRequestedAgent(agentID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if agentID != "" {
		if _, ok := m.availableAgentIDs[agentID]; !ok {
			return "", fmt.Errorf("agent %q is not installed", agentID)
		}
		return agentID, nil
	}

	if m.lastUsedAgentID != "" {
		if _, ok := m.availableAgentIDs[m.lastUsedAgentID]; ok {
			return m.lastUsedAgentID, nil
		}
	}
	if _, ok := m.availableAgentIDs["shell"]; ok {
		return "shell", nil
	}
	for id := range m.availableAgentIDs {
		return id, nil
	}
	return "", fmt.Errorf("no installed agents found")
}

func (m *Manager) upsertRepoLocked(desc repoDescriptor) *repoState {
	existing := m.findRepoByDescriptorLocked(desc)
	if existing == nil {
		m.repoSeq++
		existing = &repoState{ID: m.repoSeq}
		m.repos = append(m.repos, existing)
	}
	existing.Name = desc.Name
	existing.RootPath = desc.RootPath
	existing.GitCommonDir = desc.GitCommonDir
	existing.IsGitRepo = desc.IsGitRepo
	return existing
}

func (m *Manager) syncWorktreesLocked(repoID int, descriptors []worktreeDescriptor, selectedRoot string) *worktreeState {
	seen := make(map[string]struct{}, len(descriptors))
	var selected *worktreeState

	for _, descriptor := range descriptors {
		root := filepath.Clean(descriptor.RootPath)
		seen[root] = struct{}{}

		worktree := m.findWorktreeByRootLocked(repoID, root)
		if worktree == nil {
			m.worktreeSeq++
			worktree = &worktreeState{
				ID:     m.worktreeSeq,
				RepoID: repoID,
			}
			m.worktrees = append(m.worktrees, worktree)
		}

		worktree.Name = descriptor.Name
		worktree.RootPath = root
		worktree.GitBranch = descriptor.GitBranch
		worktree.IsPrimary = descriptor.IsPrimary

		if root == filepath.Clean(selectedRoot) {
			selected = worktree
		}
	}

	filtered := m.worktrees[:0]
	for _, worktree := range m.worktrees {
		if worktree.RepoID == repoID {
			if _, ok := seen[filepath.Clean(worktree.RootPath)]; !ok && !m.worktreeHasSessionsLocked(worktree.ID) {
				continue
			}
		}
		filtered = append(filtered, worktree)
	}
	m.worktrees = filtered

	if selected != nil {
		return selected
	}
	for _, worktree := range m.worktrees {
		if worktree.RepoID == repoID {
			return worktree
		}
	}
	return nil
}

func (m *Manager) nextSessionTitleLocked(worktreeID int, label string) string {
	count := 0
	prefix := label + " "
	for _, session := range m.sessions {
		if session.WorktreeID == worktreeID && strings.HasPrefix(session.Title, prefix) {
			count++
		}
	}
	return fmt.Sprintf("%s %d", label, count+1)
}

func (m *Manager) nextSessionCreatedAtLocked() time.Time {
	createdAt := time.Now()
	if !m.nextCreatedAt.IsZero() && !createdAt.After(m.nextCreatedAt) {
		createdAt = m.nextCreatedAt.Add(time.Millisecond)
	}
	m.nextCreatedAt = createdAt
	return createdAt
}

func (m *Manager) rebalanceActiveLocked(worktreeID, repoID, sessionID int) {
	if m.activeSessionID != sessionID {
		return
	}
	if next := m.lastSessionForWorktreeLocked(worktreeID); next != nil {
		m.activeSessionID = next.ID
		m.activeWorktreeID = next.WorktreeID
		if worktree := m.findWorktreeByIDLocked(next.WorktreeID); worktree != nil {
			m.activeRepoID = worktree.RepoID
		}
		return
	}
	if next := m.lastSessionForRepoLocked(repoID); next != nil {
		m.activeSessionID = next.ID
		m.activeWorktreeID = next.WorktreeID
		if worktree := m.findWorktreeByIDLocked(next.WorktreeID); worktree != nil {
			m.activeRepoID = worktree.RepoID
		}
		return
	}
	if next := m.lastSessionLocked(); next != nil {
		m.activeSessionID = next.ID
		m.activeWorktreeID = next.WorktreeID
		if worktree := m.findWorktreeByIDLocked(next.WorktreeID); worktree != nil {
			m.activeRepoID = worktree.RepoID
		}
		return
	}
	m.activeRepoID = 0
	m.activeWorktreeID = 0
	m.activeSessionID = 0
}

func (m *Manager) removeSessionLocked(sessionID int) {
	for index, session := range m.sessions {
		if session.ID != sessionID {
			continue
		}
		cleanupSessionSupportDir(session.SupportDir)
		m.sessions = append(m.sessions[:index], m.sessions[index+1:]...)
		return
	}
}

func (m *Manager) removeRepoIfEmptyLocked(repoID int) {
	if repoID == 0 {
		return
	}
	for _, worktree := range m.worktrees {
		if worktree.RepoID != repoID {
			continue
		}
		if m.worktreeHasSessionsLocked(worktree.ID) {
			return
		}
	}

	filteredWorktrees := m.worktrees[:0]
	for _, worktree := range m.worktrees {
		if worktree.RepoID == repoID {
			continue
		}
		filteredWorktrees = append(filteredWorktrees, worktree)
	}
	m.worktrees = filteredWorktrees

	for index, repo := range m.repos {
		if repo.ID != repoID {
			continue
		}
		m.repos = append(m.repos[:index], m.repos[index+1:]...)
		return
	}
}

func (m *Manager) worktreeHasSessionsLocked(worktreeID int) bool {
	for _, session := range m.sessions {
		if session.WorktreeID == worktreeID {
			return true
		}
	}
	return false
}

func (m *Manager) findRepoByDescriptorLocked(desc repoDescriptor) *repoState {
	for _, repo := range m.repos {
		if desc.IsGitRepo {
			if repo.IsGitRepo && filepath.Clean(repo.GitCommonDir) == filepath.Clean(desc.GitCommonDir) {
				return repo
			}
			continue
		}
		if !repo.IsGitRepo && filepath.Clean(repo.RootPath) == filepath.Clean(desc.RootPath) {
			return repo
		}
	}
	return nil
}

func (m *Manager) findRepoByIDLocked(repoID int) *repoState {
	for _, repo := range m.repos {
		if repo.ID == repoID {
			return repo
		}
	}
	return nil
}

func (m *Manager) findWorktreeByIDLocked(worktreeID int) *worktreeState {
	for _, worktree := range m.worktrees {
		if worktree.ID == worktreeID {
			return worktree
		}
	}
	return nil
}

func (m *Manager) findWorktreeByRootLocked(repoID int, rootPath string) *worktreeState {
	for _, worktree := range m.worktrees {
		if worktree.RepoID == repoID && filepath.Clean(worktree.RootPath) == filepath.Clean(rootPath) {
			return worktree
		}
	}
	return nil
}

func (m *Manager) findSessionByIDLocked(sessionID int) *sessionState {
	for _, session := range m.sessions {
		if session.ID == sessionID {
			return session
		}
	}
	return nil
}

func (m *Manager) worktreesForRepoLocked(repoID int) []*worktreeState {
	items := make([]*worktreeState, 0)
	for _, worktree := range m.worktrees {
		if worktree.RepoID == repoID {
			items = append(items, worktree)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsPrimary != items[j].IsPrimary {
			return items[i].IsPrimary
		}
		return items[i].RootPath < items[j].RootPath
	})
	return items
}

func (m *Manager) lastSessionForWorktreeLocked(worktreeID int) *sessionState {
	for i := len(m.sessions) - 1; i >= 0; i-- {
		if m.sessions[i].WorktreeID == worktreeID {
			return m.sessions[i]
		}
	}
	return nil
}

func (m *Manager) lastSessionForWorktreeExceptLocked(worktreeID int, skipSessionID int) *sessionState {
	for i := len(m.sessions) - 1; i >= 0; i-- {
		if m.sessions[i].ID == skipSessionID {
			continue
		}
		if m.sessions[i].WorktreeID == worktreeID {
			return m.sessions[i]
		}
	}
	return nil
}

func (m *Manager) lastSessionForRepoLocked(repoID int) *sessionState {
	for i := len(m.sessions) - 1; i >= 0; i-- {
		worktree := m.findWorktreeByIDLocked(m.sessions[i].WorktreeID)
		if worktree != nil && worktree.RepoID == repoID {
			return m.sessions[i]
		}
	}
	return nil
}

func (m *Manager) lastSessionForRepoExceptLocked(repoID int, skipSessionID int) *sessionState {
	for i := len(m.sessions) - 1; i >= 0; i-- {
		if m.sessions[i].ID == skipSessionID {
			continue
		}
		worktree := m.findWorktreeByIDLocked(m.sessions[i].WorktreeID)
		if worktree != nil && worktree.RepoID == repoID {
			return m.sessions[i]
		}
	}
	return nil
}

func (m *Manager) lastSessionLocked() *sessionState {
	if len(m.sessions) == 0 {
		return nil
	}
	return m.sessions[len(m.sessions)-1]
}

func (m *Manager) lastSessionExceptLocked(skipSessionID int) *sessionState {
	for i := len(m.sessions) - 1; i >= 0; i-- {
		if m.sessions[i].ID != skipSessionID {
			return m.sessions[i]
		}
	}
	return nil
}

func availableAgentDTOs(registry *agent.Registry) []AgentDTO {
	if registry == nil {
		return nil
	}
	items := registry.AvailableList()
	out := make([]AgentDTO, 0, len(items))
	for _, item := range items {
		out = append(out, AgentDTO{
			ID:    item.ID,
			Label: item.Label,
		})
	}
	return out
}

func persistedLastUsedAgentID(adapterID string) string {
	if strings.TrimSpace(adapterID) == "" || adapterID == "shell" {
		return ""
	}
	return adapterID
}

func repoPersistenceKey(repo *repoState) string {
	if repo == nil {
		return ""
	}
	if repo.IsGitRepo {
		return filepath.Clean(repo.GitCommonDir)
	}
	return filepath.Clean(repo.RootPath)
}

func toUIStateDTO(ui persist.UIState) UIStateDTO {
	collapsedRepoKeys := append([]string{}, ui.CollapsedRepoKeys...)
	return UIStateDTO{
		SidebarOpen:       ui.SidebarOpen,
		SidebarWidth:      ui.SidebarWidth,
		DiffPanelOpen:     ui.DiffPanelOpen,
		DiffPanelWidth:    ui.DiffPanelWidth,
		TerminalFontSize:  normalizeTerminalFontSize(ui.TerminalFontSize),
		UtilityPanelTab:   normalizeUtilityPanelTab(ui.UtilityPanelTab),
		CollapsedRepoKeys: collapsedRepoKeys,
	}
}

func normalizeTerminalFontSize(value int) int {
	switch {
	case value < 11:
		return 12
	case value > 24:
		return 24
	default:
		return value
	}
}

func normalizeUtilityPanelTab(value string) string {
	switch value {
	case "diff", "files", "peers":
		return value
	default:
		return "diff"
	}
}

func (m *Manager) restorePersistedSessions(saved persist.PersistedState) string {
	if saved.UIState.LastUsedAgentID != "" {
		m.mu.Lock()
		m.lastUsedAgentID = saved.UIState.LastUsedAgentID
		m.mu.Unlock()
	}

	var (
		restoreErrors  []string
		firstRestored  *sessionState
		activeRestored *sessionState
	)

	for _, record := range saved.Sessions {
		session, err := m.restoreSession(record)
		if err != nil {
			restoreErrors = append(restoreErrors, fmt.Sprintf("%s: %v", restoredSessionName(record), err))
			continue
		}
		if firstRestored == nil {
			firstRestored = session
		}
		if record.ID == saved.UIState.ActiveSessionID {
			activeRestored = session
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if activeRestored != nil {
		m.activeSessionID = activeRestored.ID
		m.activeWorktreeID = activeRestored.WorktreeID
		if worktree := m.findWorktreeByIDLocked(activeRestored.WorktreeID); worktree != nil {
			m.activeRepoID = worktree.RepoID
		}
	} else if firstRestored != nil {
		m.activeSessionID = firstRestored.ID
		m.activeWorktreeID = firstRestored.WorktreeID
		if worktree := m.findWorktreeByIDLocked(firstRestored.WorktreeID); worktree != nil {
			m.activeRepoID = worktree.RepoID
		}
	}

	if activeStorageID := m.activeSessionStorageIDLocked(); activeStorageID != saved.UIState.ActiveSessionID {
		_ = m.store.SetActiveSessionID(activeStorageID)
	}

	if len(restoreErrors) == 0 {
		return ""
	}
	return "Some sessions could not be restored: " + strings.Join(restoreErrors, "; ")
}

func (m *Manager) restoreSession(record persist.SessionRecord) (*sessionState, error) {
	rootPath, err := normalizeRootPath(record.WorktreeRootPath)
	if err != nil {
		return nil, err
	}

	requestedAgentID, err := m.resolveRequestedAgent(record.AdapterID)
	if err != nil {
		return nil, err
	}

	selection, err := describeRepoSelection(rootPath)
	if err != nil {
		return nil, err
	}

	startPath, err := normalizeSessionStartPath(selection.SelectedWorktree.RootPath, record.CWDPath)
	if err != nil {
		return nil, err
	}

	plan, err := m.prepareShellBackedLaunch(selection.SelectedWorktree.RootPath, startPath, requestedAgentID, true)
	if err != nil {
		return nil, err
	}

	label := record.Label
	if label == "" {
		label = plan.RequestedSpec.Label
	}
	repo, selectedWorktree, sessionID, title := m.reserveSessionSlot(selection, label, record.Title)
	session, _, err := m.startManagedSession(managedSessionStartRequest{
		Plan:             plan,
		Repo:             repo,
		SelectedWorktree: selectedWorktree,
		SessionID:        sessionID,
		AdapterID:        requestedAgentID,
		Label:            label,
		Title:            title,
		StartPath:        startPath,
		StorageID:        record.ID,
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

func restoredSessionName(record persist.SessionRecord) string {
	if strings.TrimSpace(record.Title) != "" {
		return record.Title
	}
	if strings.TrimSpace(record.Label) != "" {
		return record.Label
	}
	return record.AdapterID
}

func (m *Manager) sessionModeMetadata(adapterID string) (label string, family string, peerEnabled bool, err error) {
	if adapterID == "shell" {
		if cfg, ok := m.registry.Get("shell"); ok {
			config := cfg.Config()
			return config.Label, config.Family, config.PeerEnabled, nil
		}
		return "Shell", peer.FamilyGeneric, false, nil
	}

	cfg, ok := m.registry.Get(adapterID)
	if !ok {
		return "", "", false, fmt.Errorf("unknown adapter %q", adapterID)
	}
	config := cfg.Config()
	return config.Label, config.Family, config.PeerEnabled, nil
}

func cleanupSessionSupportDir(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.RemoveAll(path)
}

func normalizeSessionStartPath(worktreeRoot, cwdPath string) (string, error) {
	rootPath, err := normalizeRootPath(worktreeRoot)
	if err != nil {
		return "", err
	}

	cwdPath = strings.TrimSpace(cwdPath)
	if cwdPath == "" {
		return rootPath, nil
	}

	absCWD, err := filepath.Abs(cwdPath)
	if err != nil {
		return rootPath, nil
	}
	info, err := os.Stat(absCWD)
	if err != nil || !info.IsDir() {
		return rootPath, nil
	}

	relative, err := filepath.Rel(rootPath, absCWD)
	if err != nil {
		return rootPath, nil
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return rootPath, nil
	}
	return absCWD, nil
}

func normalizeRootPath(rootPath string) (string, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absRoot)
	}
	return absRoot, nil
}
