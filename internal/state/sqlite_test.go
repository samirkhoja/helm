package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreLoadDefaults(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(data.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want empty", data.Sessions)
	}
	if !data.UIState.SidebarOpen || data.UIState.SidebarWidth != defaultSidebarWidth {
		t.Fatalf("ui state = %#v, want default sidebar state", data.UIState)
	}
	if data.UIState.DiffPanelOpen || data.UIState.DiffPanelWidth != defaultDiffPanelWidth {
		t.Fatalf("ui state = %#v, want default diff state", data.UIState)
	}
	if data.UIState.TerminalFontSize != defaultTerminalFontSize {
		t.Fatalf("TerminalFontSize = %d, want %d", data.UIState.TerminalFontSize, defaultTerminalFontSize)
	}
	if data.UIState.UtilityPanelTab != defaultUtilityPanelTab {
		t.Fatalf("UtilityPanelTab = %q, want %q", data.UIState.UtilityPanelTab, defaultUtilityPanelTab)
	}
	if data.UIState.LastUsedAgentID != "shell" {
		t.Fatalf("LastUsedAgentID = %q, want shell", data.UIState.LastUsedAgentID)
	}
}

func TestSQLiteStoreSessionAndUIRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	createdAt := time.Unix(1710000000, 0)
	recordID, err := store.CreateSession(SessionRecord{
		WorktreeRootPath: "/tmp/repo",
		AdapterID:        "shell",
		Label:            "Shell",
		Title:            "Shell 1",
		CWDPath:          "/tmp/repo/subdir",
		CreatedAt:        createdAt,
		LastActiveAt:     createdAt,
	}, "shell")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := store.UpdateSessionCWD(recordID, "/tmp/repo/next", createdAt.Add(time.Minute)); err != nil {
		t.Fatalf("UpdateSessionCWD() error = %v", err)
	}
	if err := store.SaveUIPreferences(UIState{
		SidebarOpen:       false,
		SidebarWidth:      280,
		DiffPanelOpen:     true,
		DiffPanelWidth:    420,
		TerminalFontSize:  18,
		UtilityPanelTab:   "files",
		CollapsedRepoKeys: []string{"repo-a", "repo-b"},
	}); err != nil {
		t.Fatalf("SaveUIPreferences() error = %v", err)
	}
	if err := store.SetActiveSessionID(recordID); err != nil {
		t.Fatalf("SetActiveSessionID() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore(reopen) error = %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.Close()
	})

	data, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load() after reopen error = %v", err)
	}

	if len(data.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want 1", data.Sessions)
	}
	record := data.Sessions[0]
	if record.ID != recordID || record.CWDPath != "/tmp/repo/next" {
		t.Fatalf("record = %#v, want persisted id/cwd", record)
	}
	if !record.LastActiveAt.Equal(createdAt.Add(time.Minute)) {
		t.Fatalf("LastActiveAt = %v, want %v", record.LastActiveAt, createdAt.Add(time.Minute))
	}
	if data.UIState.ActiveSessionID != recordID {
		t.Fatalf("ActiveSessionID = %d, want %d", data.UIState.ActiveSessionID, recordID)
	}
	if data.UIState.SidebarOpen || !data.UIState.DiffPanelOpen {
		t.Fatalf("ui state = %#v, want saved panel state", data.UIState)
	}
	if data.UIState.TerminalFontSize != 18 {
		t.Fatalf("TerminalFontSize = %d, want 18", data.UIState.TerminalFontSize)
	}
	if data.UIState.UtilityPanelTab != "files" {
		t.Fatalf("UtilityPanelTab = %q, want files", data.UIState.UtilityPanelTab)
	}
	if got := len(data.UIState.CollapsedRepoKeys); got != 2 {
		t.Fatalf("CollapsedRepoKeys = %#v, want 2 items", data.UIState.CollapsedRepoKeys)
	}
}

func TestSQLiteStoreDeleteSessionClearsActive(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	recordID, err := store.CreateSession(SessionRecord{
		WorktreeRootPath: "/tmp/repo",
		AdapterID:        "shell",
		Label:            "Shell",
		Title:            "Shell 1",
	}, "shell")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := store.DeleteSession(recordID, 0); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(data.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want empty", data.Sessions)
	}
	if data.UIState.ActiveSessionID != 0 {
		t.Fatalf("ActiveSessionID = %d, want 0", data.UIState.ActiveSessionID)
	}
}

func TestSQLiteStoreUpdateSessionModeAndLastUsedAgent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	createdAt := time.Unix(1710000000, 0)
	recordID, err := store.CreateSession(SessionRecord{
		WorktreeRootPath: "/tmp/repo",
		AdapterID:        "shell",
		Label:            "Shell",
		Title:            "Codex 1",
		CWDPath:          "/tmp/repo",
		CreatedAt:        createdAt,
		LastActiveAt:     createdAt,
	}, "shell")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := store.SetLastUsedAgentID("codex"); err != nil {
		t.Fatalf("SetLastUsedAgentID() error = %v", err)
	}
	if err := store.UpdateSessionMode(recordID, "codex", "Codex", createdAt.Add(time.Minute)); err != nil {
		t.Fatalf("UpdateSessionMode() error = %v", err)
	}

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if data.UIState.LastUsedAgentID != "codex" {
		t.Fatalf("LastUsedAgentID = %q, want codex", data.UIState.LastUsedAgentID)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want 1", data.Sessions)
	}
	if data.Sessions[0].AdapterID != "codex" || data.Sessions[0].Label != "Codex" {
		t.Fatalf("session = %#v, want codex/Codex", data.Sessions[0])
	}
	if !data.Sessions[0].LastActiveAt.Equal(createdAt.Add(time.Minute)) {
		t.Fatalf("LastActiveAt = %v, want %v", data.Sessions[0].LastActiveAt, createdAt.Add(time.Minute))
	}
}

func TestSQLiteStoreNormalizesNilCollapsedRepoKeys(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	if err := store.SaveUIPreferences(UIState{
		SidebarOpen:       true,
		SidebarWidth:      defaultSidebarWidth,
		DiffPanelOpen:     false,
		DiffPanelWidth:    defaultDiffPanelWidth,
		TerminalFontSize:  0,
		UtilityPanelTab:   "",
		CollapsedRepoKeys: nil,
	}); err != nil {
		t.Fatalf("SaveUIPreferences() error = %v", err)
	}

	if _, err := store.db.ExecContext(context.Background(), `
		UPDATE ui_state
		SET collapsed_repo_keys_json = 'null'
		WHERE id = 1
	`); err != nil {
		t.Fatalf("update ui_state collapsed_repo_keys_json: %v", err)
	}

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if data.UIState.CollapsedRepoKeys == nil {
		t.Fatalf("CollapsedRepoKeys = nil, want empty slice")
	}
	if len(data.UIState.CollapsedRepoKeys) != 0 {
		t.Fatalf("CollapsedRepoKeys = %#v, want empty slice", data.UIState.CollapsedRepoKeys)
	}
	if data.UIState.UtilityPanelTab != defaultUtilityPanelTab {
		t.Fatalf("UtilityPanelTab = %q, want %q", data.UIState.UtilityPanelTab, defaultUtilityPanelTab)
	}
	if data.UIState.TerminalFontSize != defaultTerminalFontSize {
		t.Fatalf("TerminalFontSize = %d, want %d", data.UIState.TerminalFontSize, defaultTerminalFontSize)
	}
}

func TestSQLiteStoreMigratesToV1(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	defer func() {
		_ = store.Close()
	}()

	var version int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version = %d, want %d", version, schemaVersion)
	}
}

func TestSQLiteStoreMigratesV2UtilityPanelTab(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state-v2.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if _, err := db.Exec(`
		CREATE TABLE sessions (
			id INTEGER PRIMARY KEY,
			worktree_root_path TEXT NOT NULL,
			adapter_id TEXT NOT NULL,
			label TEXT NOT NULL,
			title TEXT NOT NULL,
			cwd_path TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_active_at INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE ui_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_used_agent_id TEXT NOT NULL,
			active_session_id INTEGER,
			sidebar_open INTEGER NOT NULL,
			sidebar_width INTEGER NOT NULL,
			diff_panel_open INTEGER NOT NULL,
			diff_panel_width INTEGER NOT NULL,
			collapsed_repo_keys_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create ui_state table: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO ui_state (
			id,
			last_used_agent_id,
			active_session_id,
			sidebar_open,
			sidebar_width,
			diff_panel_open,
			diff_panel_width,
			collapsed_repo_keys_json
		) VALUES (1, 'shell', NULL, 1, 248, 1, 420, '[]')
	`); err != nil {
		t.Fatalf("insert ui_state row: %v", err)
	}

	if _, err := db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if data.UIState.UtilityPanelTab != defaultUtilityPanelTab {
		t.Fatalf("UtilityPanelTab = %q, want %q", data.UIState.UtilityPanelTab, defaultUtilityPanelTab)
	}
	if data.UIState.TerminalFontSize != defaultTerminalFontSize {
		t.Fatalf("TerminalFontSize = %d, want %d", data.UIState.TerminalFontSize, defaultTerminalFontSize)
	}
}

func TestSQLiteStoreMigratesV3TerminalFontSize(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state-v3.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if _, err := db.Exec(`
		CREATE TABLE sessions (
			id INTEGER PRIMARY KEY,
			worktree_root_path TEXT NOT NULL,
			adapter_id TEXT NOT NULL,
			label TEXT NOT NULL,
			title TEXT NOT NULL,
			cwd_path TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_active_at INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE ui_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_used_agent_id TEXT NOT NULL,
			active_session_id INTEGER,
			sidebar_open INTEGER NOT NULL,
			sidebar_width INTEGER NOT NULL,
			diff_panel_open INTEGER NOT NULL,
			diff_panel_width INTEGER NOT NULL,
			utility_panel_tab TEXT NOT NULL DEFAULT 'diff',
			collapsed_repo_keys_json TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create ui_state table: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO ui_state (
			id,
			last_used_agent_id,
			active_session_id,
			sidebar_open,
			sidebar_width,
			diff_panel_open,
			diff_panel_width,
			utility_panel_tab,
			collapsed_repo_keys_json
		) VALUES (1, 'shell', NULL, 1, 248, 0, 360, 'diff', '[]')
	`); err != nil {
		t.Fatalf("insert ui_state row: %v", err)
	}

	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	data, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if data.UIState.TerminalFontSize != defaultTerminalFontSize {
		t.Fatalf("TerminalFontSize = %d, want %d", data.UIState.TerminalFontSize, defaultTerminalFontSize)
	}
}

func TestSQLiteStorePeerMessageLifecycle(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	if err := store.UpsertPeerRegistration(PeerRegistrationRecord{
		PeerID:            "peer-a",
		TokenHash:         "token-a",
		RuntimeInstanceID: "runtime-a",
		SessionID:         1,
		WorktreeRootPath:  "/tmp/repo",
		RepoKey:           "repo-key",
		AdapterID:         "codex",
		AdapterFamily:     "codex",
		Label:             "Peer A",
		Title:             "Peer A",
		CreatedAt:         now,
		LastHeartbeatAt:   now,
	}); err != nil {
		t.Fatalf("UpsertPeerRegistration(peer-a) error = %v", err)
	}
	if err := store.UpsertPeerRegistration(PeerRegistrationRecord{
		PeerID:            "peer-b",
		TokenHash:         "token-b",
		RuntimeInstanceID: "runtime-b",
		SessionID:         2,
		WorktreeRootPath:  "/tmp/repo",
		RepoKey:           "repo-key",
		AdapterID:         "claude-code",
		AdapterFamily:     "claude",
		Label:             "Peer B",
		Title:             "Peer B",
		CreatedAt:         now,
		LastHeartbeatAt:   now,
	}); err != nil {
		t.Fatalf("UpsertPeerRegistration(peer-b) error = %v", err)
	}

	firstID, err := store.CreatePeerMessage(PeerMessageRecord{
		FromPeerID: "peer-a",
		ToPeerID:   "peer-b",
		FromLabel:  "Peer A",
		FromTitle:  "Peer A",
		Body:       "First message",
		Status:     PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(first) error = %v", err)
	}
	secondID, err := store.CreatePeerMessage(PeerMessageRecord{
		FromPeerID: "peer-a",
		ToPeerID:   "peer-b",
		FromLabel:  "Peer A",
		FromTitle:  "Peer A",
		Body:       "Second message",
		Status:     PeerMessageStatusQueued,
		CreatedAt:  now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(second) error = %v", err)
	}

	outstanding, err := store.OutstandingPeerCounts([]string{"peer-a", "peer-b"})
	if err != nil {
		t.Fatalf("OutstandingPeerCounts() error = %v", err)
	}
	if outstanding["peer-b"] != 2 {
		t.Fatalf("OutstandingPeerCounts(peer-b) = %d, want 2", outstanding["peer-b"])
	}

	unread, err := store.UnreadPeerCounts([]string{"peer-b"})
	if err != nil {
		t.Fatalf("UnreadPeerCounts() error = %v", err)
	}
	if unread["peer-b"] != 2 {
		t.Fatalf("UnreadPeerCounts(peer-b) = %d, want 2", unread["peer-b"])
	}

	if err := store.MarkPeerMessagesNoticed("peer-b", secondID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("MarkPeerMessagesNoticed() error = %v", err)
	}

	inbox, err := store.ListPeerInbox("peer-b", 0, 10, false)
	if err != nil {
		t.Fatalf("ListPeerInbox() error = %v", err)
	}
	if len(inbox) != 2 {
		t.Fatalf("ListPeerInbox() len = %d, want 2", len(inbox))
	}
	if inbox[0].ID != secondID || inbox[0].Status != PeerMessageStatusNoticed {
		t.Fatalf("latest inbox message = %#v, want noticed second message", inbox[0])
	}

	if err := store.MarkPeerMessagesRead("peer-b", []int64{firstID, secondID}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("MarkPeerMessagesRead() error = %v", err)
	}

	unread, err = store.UnreadPeerCounts([]string{"peer-b"})
	if err != nil {
		t.Fatalf("UnreadPeerCounts(after read) error = %v", err)
	}
	if unread["peer-b"] != 0 {
		t.Fatalf("UnreadPeerCounts(peer-b) after read = %d, want 0", unread["peer-b"])
	}

	if err := store.AckPeerMessage("peer-b", firstID, now.Add(4*time.Second)); err != nil {
		t.Fatalf("AckPeerMessage() error = %v", err)
	}

	outstanding, err = store.OutstandingPeerCounts([]string{"peer-b"})
	if err != nil {
		t.Fatalf("OutstandingPeerCounts(after ack) error = %v", err)
	}
	if outstanding["peer-b"] != 1 {
		t.Fatalf("OutstandingPeerCounts(peer-b) after ack = %d, want 1", outstanding["peer-b"])
	}

	if err := store.FailPeerMessages("peer-b", now.Add(5*time.Second), "peer offline"); err != nil {
		t.Fatalf("FailPeerMessages() error = %v", err)
	}

	outstanding, err = store.OutstandingPeerCounts([]string{"peer-b"})
	if err != nil {
		t.Fatalf("OutstandingPeerCounts(after fail) error = %v", err)
	}
	if outstanding["peer-b"] != 0 {
		t.Fatalf("OutstandingPeerCounts(peer-b) after fail = %d, want 0", outstanding["peer-b"])
	}
}

func TestSQLiteStoreExpireStalePeersRemovesRegistrationAndFailsMessages(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	if err := store.UpsertPeerRegistration(PeerRegistrationRecord{
		PeerID:            "stale-peer",
		TokenHash:         "token-stale",
		RuntimeInstanceID: "runtime-stale",
		SessionID:         7,
		WorktreeRootPath:  "/tmp/repo",
		RepoKey:           "repo-key",
		AdapterID:         "codex",
		AdapterFamily:     "codex",
		Label:             "Stale Peer",
		Title:             "Stale Peer",
		CreatedAt:         now.Add(-time.Hour),
		LastHeartbeatAt:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("UpsertPeerRegistration() error = %v", err)
	}

	messageID, err := store.CreatePeerMessage(PeerMessageRecord{
		FromPeerID: "sender",
		ToPeerID:   "stale-peer",
		FromLabel:  "Sender",
		FromTitle:  "Sender",
		Body:       "Are you still there?",
		Status:     PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage() error = %v", err)
	}

	if err := store.ExpireStalePeers(now.Add(-time.Minute), now, "peer session is offline"); err != nil {
		t.Fatalf("ExpireStalePeers() error = %v", err)
	}

	if _, err := store.GetPeerRegistration("stale-peer"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPeerRegistration() error = %v, want sql.ErrNoRows", err)
	}

	var status string
	var failureReason string
	if err := store.db.QueryRowContext(context.Background(), `
		SELECT status, failure_reason
		FROM peer_messages
		WHERE id = ?
	`, messageID).Scan(&status, &failureReason); err != nil {
		t.Fatalf("query peer_messages: %v", err)
	}
	if status != PeerMessageStatusFailed {
		t.Fatalf("message status = %q, want %q", status, PeerMessageStatusFailed)
	}
	if failureReason != "peer session is offline" {
		t.Fatalf("failure_reason = %q, want %q", failureReason, "peer session is offline")
	}
}

func TestSQLiteStoreDeleteAndClearPeerMessages(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	for _, record := range []PeerRegistrationRecord{
		{
			PeerID:            "peer-a",
			TokenHash:         "token-a",
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo-a",
			RepoKey:           "repo-a",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Peer A",
			Title:             "Peer A",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "peer-b",
			TokenHash:         "token-b",
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo-b",
			RepoKey:           "repo-b",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Peer B",
			Title:             "Peer B",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	firstID, err := store.CreatePeerMessage(PeerMessageRecord{
		FromPeerID: "peer-a",
		ToPeerID:   "peer-b",
		FromLabel:  "Peer A",
		FromTitle:  "Peer A",
		Body:       "First",
		Status:     PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(first) error = %v", err)
	}
	secondID, err := store.CreatePeerMessage(PeerMessageRecord{
		FromPeerID: "peer-b",
		ToPeerID:   "peer-a",
		FromLabel:  "Peer B",
		FromTitle:  "Peer B",
		Body:       "Second",
		Status:     PeerMessageStatusAcked,
		CreatedAt:  now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(second) error = %v", err)
	}

	if err := store.DeletePeerMessage(firstID); err != nil {
		t.Fatalf("DeletePeerMessage() error = %v", err)
	}

	messages, err := store.ListRecentPeerMessages(10)
	if err != nil {
		t.Fatalf("ListRecentPeerMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].ID != secondID {
		t.Fatalf("messages = %#v, want only second message", messages)
	}

	if err := store.ClearPeerMessages(); err != nil {
		t.Fatalf("ClearPeerMessages() error = %v", err)
	}

	messages, err = store.ListRecentPeerMessages(10)
	if err != nil {
		t.Fatalf("ListRecentPeerMessages() after clear error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want no messages after clear", messages)
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	return store
}
