package session

import "helm-wails/internal/agent"

const (
	EventSessionOutput    = "session:output"
	EventSessionLifecycle = "session:lifecycle"
	EventAppSnapshot      = "app:snapshot"
	EventPeerState        = "peer:state"

	WorktreeModeNewBranch      = "new-branch"
	WorktreeModeExistingBranch = "existing-branch"
)

type AgentDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type SessionDTO struct {
	ID                   int    `json:"id"`
	WorktreeID           int    `json:"worktreeId"`
	AdapterID            string `json:"adapterId"`
	Label                string `json:"label"`
	Title                string `json:"title"`
	Status               string `json:"status"`
	CWDPath              string `json:"cwdPath"`
	PeerID               string `json:"peerId"`
	PeerCapable          bool   `json:"peerCapable"`
	PeerSummary          string `json:"peerSummary"`
	OutstandingPeerCount int    `json:"outstandingPeerCount"`
}

type WorktreeDTO struct {
	ID        int          `json:"id"`
	RepoID    int          `json:"repoId"`
	Name      string       `json:"name"`
	RootPath  string       `json:"rootPath"`
	GitBranch string       `json:"gitBranch"`
	IsPrimary bool         `json:"isPrimary"`
	Sessions  []SessionDTO `json:"sessions"`
}

type RepoDTO struct {
	ID             int           `json:"id"`
	Name           string        `json:"name"`
	RootPath       string        `json:"rootPath"`
	GitCommonDir   string        `json:"gitCommonDir"`
	IsGitRepo      bool          `json:"isGitRepo"`
	PersistenceKey string        `json:"persistenceKey"`
	Worktrees      []WorktreeDTO `json:"worktrees"`
}

type AppSnapshot struct {
	Repos            []RepoDTO  `json:"repos"`
	ActiveRepoID     int        `json:"activeRepoId"`
	ActiveWorktreeID int        `json:"activeWorktreeId"`
	ActiveSessionID  int        `json:"activeSessionId"`
	AvailableAgents  []AgentDTO `json:"availableAgents"`
	LastUsedAgentID  string     `json:"lastUsedAgentId"`
}

type UIStateDTO struct {
	SidebarOpen       bool     `json:"sidebarOpen"`
	SidebarWidth      int      `json:"sidebarWidth"`
	DiffPanelOpen     bool     `json:"diffPanelOpen"`
	DiffPanelWidth    int      `json:"diffPanelWidth"`
	TerminalFontSize  int      `json:"terminalFontSize"`
	UtilityPanelTab   string   `json:"utilityPanelTab"`
	CollapsedRepoKeys []string `json:"collapsedRepoKeys"`
}

type BootstrapResult struct {
	Snapshot      AppSnapshot  `json:"snapshot"`
	UIState       UIStateDTO   `json:"uiState"`
	RestoreNotice string       `json:"restoreNotice"`
	PeerState     PeerStateDTO `json:"peerState"`
}

type PeerDTO struct {
	PeerID              string `json:"peerId"`
	SessionID           int    `json:"sessionId"`
	AdapterID           string `json:"adapterId"`
	AdapterFamily       string `json:"adapterFamily"`
	Label               string `json:"label"`
	Title               string `json:"title"`
	Summary             string `json:"summary"`
	RepoKey             string `json:"repoKey"`
	WorktreeRootPath    string `json:"worktreeRootPath"`
	OutstandingCount    int    `json:"outstandingCount"`
	UnreadCount         int    `json:"unreadCount"`
	IsSelf              bool   `json:"isSelf"`
	LastHeartbeatUnixMs int64  `json:"lastHeartbeatUnixMs"`
}

type PeerMessageDTO struct {
	ID              int64  `json:"id"`
	FromPeerID      string `json:"fromPeerId"`
	ToPeerID        string `json:"toPeerId"`
	FromLabel       string `json:"fromLabel"`
	FromTitle       string `json:"fromTitle"`
	ReplyToID       int64  `json:"replyToId"`
	Body            string `json:"body"`
	Status          string `json:"status"`
	CreatedAtUnixMs int64  `json:"createdAtUnixMs"`
	NoticedAtUnixMs int64  `json:"noticedAtUnixMs"`
	ReadAtUnixMs    int64  `json:"readAtUnixMs"`
	AckedAtUnixMs   int64  `json:"ackedAtUnixMs"`
	FailedAtUnixMs  int64  `json:"failedAtUnixMs"`
	FailureReason   string `json:"failureReason"`
}

type PeerStateDTO struct {
	Peers    []PeerDTO        `json:"peers"`
	Messages []PeerMessageDTO `json:"messages"`
}

type WorkspaceChoice struct {
	RootPath  string `json:"rootPath"`
	Name      string `json:"name"`
	GitBranch string `json:"gitBranch"`
}

type WorktreeCreateRequest struct {
	Mode       string `json:"mode"`
	BranchName string `json:"branchName"`
	SourceRef  string `json:"sourceRef"`
	Path       string `json:"path"`
	AgentID    string `json:"agentId"`
}

type GitFileChange struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

type WorktreeDiff struct {
	WorktreeID    int             `json:"worktreeId"`
	RootPath      string          `json:"rootPath"`
	GitBranch     string          `json:"gitBranch"`
	IsGitRepo     bool            `json:"isGitRepo"`
	Staged        []GitFileChange `json:"staged"`
	Unstaged      []GitFileChange `json:"unstaged"`
	Untracked     []string        `json:"untracked"`
	StagedPatch   string          `json:"stagedPatch"`
	UnstagedPatch string          `json:"unstagedPatch"`
}

type FileDiff struct {
	WorktreeID int    `json:"worktreeId"`
	Path       string `json:"path"`
	Staged     bool   `json:"staged"`
	Patch      string `json:"patch"`
	Message    string `json:"message"`
}

type WorktreeEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Expandable bool   `json:"expandable"`
}

type WorktreeContentMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Preview string `json:"preview"`
}

type WorktreeFile struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	VersionToken string `json:"versionToken"`
}

type SessionOutputEvent struct {
	SessionID int    `json:"sessionId"`
	Data      string `json:"data"`
}

type SessionLifecycleEvent struct {
	SessionID  int    `json:"sessionId"`
	WorktreeID int    `json:"worktreeId"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exitCode"`
	Error      string `json:"error,omitempty"`
}

type EventSink interface {
	Emit(event string, payload any)
}

type EventSinkFunc func(event string, payload any)

func (f EventSinkFunc) Emit(event string, payload any) {
	f(event, payload)
}

type StartMeta struct {
	SessionID  int
	WorktreeID int
}

type ExitInfo struct {
	SessionID  int
	WorktreeID int
	ExitCode   int
	Err        error
}

type ExitHandler func(info ExitInfo)

type Handle interface {
	Write(data string) error
	Resize(cols, rows int) error
	Close() error
}

type Starter interface {
	Start(spec agent.LaunchSpec, meta StartMeta, sink EventSink, onExit ExitHandler) (Handle, error)
}
