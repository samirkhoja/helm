package state

import (
	"os"
	"path/filepath"
	"time"
)

const (
	defaultSidebarWidth     = 248
	defaultDiffPanelWidth   = 360
	defaultUtilityPanelTab  = "diff"
	defaultTerminalFontSize = 12
	minTerminalFontSize     = 11
	maxTerminalFontSize     = 24
	PeerLiveWindow          = 30 * time.Second

	PeerScopeWorktree = "worktree"
	PeerScopeRepo     = "repo"
	PeerScopeMachine  = "machine"

	PeerMessageStatusQueued  = "queued"
	PeerMessageStatusNoticed = "noticed"
	PeerMessageStatusRead    = "read"
	PeerMessageStatusAcked   = "acked"
	PeerMessageStatusFailed  = "failed"
)

type SessionRecord struct {
	ID               int64
	WorktreeRootPath string
	AdapterID        string
	Label            string
	Title            string
	CWDPath          string
	CreatedAt        time.Time
	LastActiveAt     time.Time
}

type UIState struct {
	LastUsedAgentID   string
	ActiveSessionID   int64
	SidebarOpen       bool
	SidebarWidth      int
	DiffPanelOpen     bool
	DiffPanelWidth    int
	TerminalFontSize  int
	UtilityPanelTab   string
	CollapsedRepoKeys []string
}

type PeerRegistrationRecord struct {
	PeerID            string
	TokenHash         string
	RuntimeInstanceID string
	SessionID         int
	WorktreeRootPath  string
	RepoKey           string
	AdapterID         string
	AdapterFamily     string
	Label             string
	Title             string
	Summary           string
	CreatedAt         time.Time
	LastHeartbeatAt   time.Time
}

type PeerMessageRecord struct {
	ID            int64
	FromPeerID    string
	ToPeerID      string
	FromLabel     string
	FromTitle     string
	ReplyToID     int64
	Body          string
	Status        string
	CreatedAt     time.Time
	NoticedAt     time.Time
	ReadAt        time.Time
	AckedAt       time.Time
	FailedAt      time.Time
	FailureReason string
}

type PeerListFilter struct {
	Scope         string
	SelfPeerID    string
	RepoKey       string
	WorktreeRoot  string
	IncludeSelf   bool
	OnlyLivePeers bool
}

type PersistedState struct {
	Sessions []SessionRecord
	UIState  UIState
}

type Store interface {
	Load() (PersistedState, error)
	CreateSession(record SessionRecord, lastUsedAgentID string) (int64, error)
	DeleteSession(storageID int64, nextActiveStorageID int64) error
	UpdateSessionMode(storageID int64, adapterID, label string, lastActiveAt time.Time) error
	UpdateSessionCWD(storageID int64, cwdPath string, lastActiveAt time.Time) error
	SetLastUsedAgentID(lastUsedAgentID string) error
	SaveUIPreferences(ui UIState) error
	SetActiveSessionID(storageID int64) error
	DBPath() string
	UpsertPeerRegistration(record PeerRegistrationRecord) error
	RemovePeerRegistration(peerID string) error
	UpdatePeerHeartbeat(peerID string, heartbeatAt time.Time) error
	UpdatePeerSummary(peerID string, summary string, updatedAt time.Time) error
	GetPeerRegistration(peerID string) (PeerRegistrationRecord, error)
	ListPeerRegistrations(filter PeerListFilter) ([]PeerRegistrationRecord, error)
	CreatePeerMessage(record PeerMessageRecord) (int64, error)
	ListPeerInbox(peerID string, messageID int64, limit int, includeRead bool) ([]PeerMessageRecord, error)
	ListRecentPeerMessages(limit int) ([]PeerMessageRecord, error)
	DeletePeerMessage(messageID int64) error
	ClearPeerMessages() error
	ListQueuedPeerMessagesForRuntime(runtimeInstanceID string, limit int) ([]PeerMessageRecord, error)
	MarkPeerMessagesNoticed(peerID string, upToMessageID int64, noticedAt time.Time) error
	MarkPeerMessagesRead(peerID string, messageIDs []int64, readAt time.Time) error
	AckPeerMessage(peerID string, messageID int64, ackedAt time.Time) error
	FailPeerMessages(peerID string, failedAt time.Time, reason string) error
	OutstandingPeerCounts(peerIDs []string) (map[string]int, error)
	UnreadPeerCounts(peerIDs []string) (map[string]int, error)
	ExpireStalePeers(before time.Time, failedAt time.Time, reason string) error
	Close() error
}

func DefaultDBPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "helm", "state.db"), nil
}

func DefaultUIState() UIState {
	return UIState{
		LastUsedAgentID:   "shell",
		ActiveSessionID:   0,
		SidebarOpen:       true,
		SidebarWidth:      defaultSidebarWidth,
		DiffPanelOpen:     false,
		DiffPanelWidth:    defaultDiffPanelWidth,
		TerminalFontSize:  defaultTerminalFontSize,
		UtilityPanelTab:   defaultUtilityPanelTab,
		CollapsedRepoKeys: []string{},
	}
}

func normalizeUtilityPanelTab(value string) string {
	switch value {
	case "diff", "files", "peers", "shell":
		return value
	default:
		return defaultUtilityPanelTab
	}
}

func normalizeTerminalFontSize(value int) int {
	switch {
	case value < minTerminalFontSize:
		return defaultTerminalFontSize
	case value > maxTerminalFontSize:
		return maxTerminalFontSize
	default:
		return value
	}
}
