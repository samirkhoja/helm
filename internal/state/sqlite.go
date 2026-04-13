package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	currentSchemaVersion       = 4
	maxCompatibleSchemaVersion = 5
)

type SQLiteStore struct {
	db   *sql.DB
	path string
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	return openSQLiteStore(path, false)
}

func OpenSQLiteStoreReadOnly(path string) (*SQLiteStore, error) {
	return openSQLiteStore(path, true)
}

func openSQLiteStore(path string, readOnly bool) (*SQLiteStore, error) {
	path = filepath.Clean(path)
	if !readOnly {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create state directory: %w", err)
		}
	}

	dsn := path
	if readOnly {
		dsn = sqliteReadOnlyDSN(path)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db, path: path}
	if !readOnly {
		if err := store.migrate(context.Background()); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return store, nil
}

func sqliteReadOnlyDSN(path string) string {
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: "mode=ro",
	}).String()
}

func (s *SQLiteStore) DBPath() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Load() (PersistedState, error) {
	ctx := context.Background()
	ui, err := s.loadUIState(ctx)
	if err != nil {
		return PersistedState{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, worktree_root_path, adapter_id, label, title, cwd_path, created_at, last_active_at
		FROM sessions
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return PersistedState{}, fmt.Errorf("load persisted sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionRecord
	for rows.Next() {
		var record SessionRecord
		var createdAt int64
		var lastActiveAt int64
		if err := rows.Scan(
			&record.ID,
			&record.WorktreeRootPath,
			&record.AdapterID,
			&record.Label,
			&record.Title,
			&record.CWDPath,
			&createdAt,
			&lastActiveAt,
		); err != nil {
			return PersistedState{}, fmt.Errorf("scan persisted session: %w", err)
		}
		record.CreatedAt = time.UnixMilli(createdAt)
		record.LastActiveAt = time.UnixMilli(lastActiveAt)
		sessions = append(sessions, record)
	}
	if err := rows.Err(); err != nil {
		return PersistedState{}, fmt.Errorf("iterate persisted sessions: %w", err)
	}

	return PersistedState{
		Sessions: sessions,
		UIState:  ui,
	}, nil
}

func (s *SQLiteStore) CreateSession(record SessionRecord, lastUsedAgentID string) (int64, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin create session transaction: %w", err)
	}
	defer rollback(tx)

	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if record.LastActiveAt.IsZero() {
		record.LastActiveAt = record.CreatedAt
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			worktree_root_path,
			adapter_id,
			label,
			title,
			cwd_path,
			created_at,
			last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		record.WorktreeRootPath,
		record.AdapterID,
		record.Label,
		record.Title,
		record.CWDPath,
		record.CreatedAt.UnixMilli(),
		record.LastActiveAt.UnixMilli(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert session row: %w", err)
	}

	storageID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read session row id: %w", err)
	}

	if strings.TrimSpace(lastUsedAgentID) != "" {
		if err := updateLastUsedAgentID(ctx, tx, lastUsedAgentID); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit create session transaction: %w", err)
	}
	return storageID, nil
}

func (s *SQLiteStore) DeleteSession(storageID int64, nextActiveStorageID int64) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete session transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, storageID); err != nil {
		return fmt.Errorf("delete session row: %w", err)
	}
	if err := setActiveSessionID(ctx, tx, nextActiveStorageID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete session transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateSessionMode(storageID int64, adapterID, label string, lastActiveAt time.Time) error {
	if lastActiveAt.IsZero() {
		lastActiveAt = time.Now()
	}
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE sessions SET adapter_id = ?, label = ?, last_active_at = ? WHERE id = ?`,
		adapterID,
		label,
		lastActiveAt.UnixMilli(),
		storageID,
	)
	if err != nil {
		return fmt.Errorf("update session mode: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateSessionCWD(storageID int64, cwdPath string, lastActiveAt time.Time) error {
	if lastActiveAt.IsZero() {
		lastActiveAt = time.Now()
	}
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE sessions SET cwd_path = ?, last_active_at = ? WHERE id = ?`,
		cwdPath,
		lastActiveAt.UnixMilli(),
		storageID,
	)
	if err != nil {
		return fmt.Errorf("update session cwd: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SetLastUsedAgentID(lastUsedAgentID string) error {
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE ui_state SET last_used_agent_id = ? WHERE id = 1`,
		lastUsedAgentID,
	)
	if err != nil {
		return fmt.Errorf("set last used agent id: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveUIPreferences(ui UIState) error {
	collapsedRepoKeys := ui.CollapsedRepoKeys
	if collapsedRepoKeys == nil {
		collapsedRepoKeys = []string{}
	}
	terminalFontSize := normalizeTerminalFontSize(ui.TerminalFontSize)

	encodedKeys, err := json.Marshal(collapsedRepoKeys)
	if err != nil {
		return fmt.Errorf("encode collapsed repo keys: %w", err)
	}
	_, err = s.db.ExecContext(context.Background(), `
		UPDATE ui_state
		SET sidebar_open = ?,
		    sidebar_width = ?,
		    diff_panel_open = ?,
		    diff_panel_width = ?,
		    terminal_font_size = ?,
		    utility_panel_tab = ?,
		    collapsed_repo_keys_json = ?
		WHERE id = 1
	`,
		boolToInt(ui.SidebarOpen),
		ui.SidebarWidth,
		boolToInt(ui.DiffPanelOpen),
		ui.DiffPanelWidth,
		terminalFontSize,
		normalizeUtilityPanelTab(ui.UtilityPanelTab),
		string(encodedKeys),
	)
	if err != nil {
		return fmt.Errorf("save ui preferences: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SetActiveSessionID(storageID int64) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin set active session transaction: %w", err)
	}
	defer rollback(tx)

	if err := setActiveSessionID(context.Background(), tx, storageID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set active session transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpsertPeerRegistration(record PeerRegistrationRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if record.LastHeartbeatAt.IsZero() {
		record.LastHeartbeatAt = record.CreatedAt
	}

	_, err := s.db.ExecContext(context.Background(), `
		INSERT INTO peer_registrations (
			peer_id,
			token_hash,
			runtime_instance_id,
			session_id,
			worktree_root_path,
			repo_key,
			adapter_id,
			adapter_family,
			label,
			title,
			summary,
			created_at,
			last_heartbeat_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			token_hash = excluded.token_hash,
			runtime_instance_id = excluded.runtime_instance_id,
			session_id = excluded.session_id,
			worktree_root_path = excluded.worktree_root_path,
			repo_key = excluded.repo_key,
			adapter_id = excluded.adapter_id,
			adapter_family = excluded.adapter_family,
			label = excluded.label,
			title = excluded.title,
			summary = excluded.summary,
			last_heartbeat_at = excluded.last_heartbeat_at
	`,
		record.PeerID,
		record.TokenHash,
		record.RuntimeInstanceID,
		record.SessionID,
		record.WorktreeRootPath,
		record.RepoKey,
		record.AdapterID,
		record.AdapterFamily,
		record.Label,
		record.Title,
		record.Summary,
		record.CreatedAt.UnixMilli(),
		record.LastHeartbeatAt.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("upsert peer registration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RemovePeerRegistration(peerID string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM peer_registrations WHERE peer_id = ?`, peerID)
	if err != nil {
		return fmt.Errorf("remove peer registration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdatePeerHeartbeat(peerID string, heartbeatAt time.Time) error {
	if heartbeatAt.IsZero() {
		heartbeatAt = time.Now()
	}
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE peer_registrations SET last_heartbeat_at = ? WHERE peer_id = ?`,
		heartbeatAt.UnixMilli(),
		peerID,
	)
	if err != nil {
		return fmt.Errorf("update peer heartbeat: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdatePeerSummary(peerID string, summary string, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	_, err := s.db.ExecContext(
		context.Background(),
		`UPDATE peer_registrations SET summary = ?, last_heartbeat_at = ? WHERE peer_id = ?`,
		summary,
		updatedAt.UnixMilli(),
		peerID,
	)
	if err != nil {
		return fmt.Errorf("update peer summary: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetPeerRegistration(peerID string) (PeerRegistrationRecord, error) {
	row := s.db.QueryRowContext(context.Background(), `
		SELECT peer_id, token_hash, runtime_instance_id, session_id, worktree_root_path, repo_key,
		       adapter_id, adapter_family, label, title, summary, created_at, last_heartbeat_at
		FROM peer_registrations
		WHERE peer_id = ?
	`, peerID)
	record, err := scanPeerRegistration(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return PeerRegistrationRecord{}, err
		}
		return PeerRegistrationRecord{}, fmt.Errorf("get peer registration: %w", err)
	}
	return record, nil
}

func (s *SQLiteStore) ListPeerRegistrations(filter PeerListFilter) ([]PeerRegistrationRecord, error) {
	conditions := []string{}
	args := []any{}

	switch filter.Scope {
	case "", PeerScopeMachine:
		// no-op
	case PeerScopeRepo:
		conditions = append(conditions, "repo_key = ?")
		args = append(args, filter.RepoKey)
	case PeerScopeWorktree:
		conditions = append(conditions, "worktree_root_path = ?")
		args = append(args, filter.WorktreeRoot)
	default:
		return nil, fmt.Errorf("unsupported peer scope %q", filter.Scope)
	}
	if !filter.IncludeSelf && strings.TrimSpace(filter.SelfPeerID) != "" {
		conditions = append(conditions, "peer_id <> ?")
		args = append(args, filter.SelfPeerID)
	}

	query := `
		SELECT peer_id, token_hash, runtime_instance_id, session_id, worktree_root_path, repo_key,
		       adapter_id, adapter_family, label, title, summary, created_at, last_heartbeat_at
		FROM peer_registrations
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY lower(label) ASC, created_at ASC, peer_id ASC"

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("list peer registrations: %w", err)
	}
	defer rows.Close()

	var records []PeerRegistrationRecord
	for rows.Next() {
		record, err := scanPeerRegistration(rows)
		if err != nil {
			return nil, fmt.Errorf("scan peer registration: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer registrations: %w", err)
	}
	return records, nil
}

func (s *SQLiteStore) CreatePeerMessage(record PeerMessageRecord) (int64, error) {
	if record.Status == "" {
		record.Status = PeerMessageStatusQueued
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	var replyTo any
	if record.ReplyToID != 0 {
		replyTo = record.ReplyToID
	}

	result, err := s.db.ExecContext(context.Background(), `
		INSERT INTO peer_messages (
			from_peer_id,
			to_peer_id,
			from_label,
			from_title,
			reply_to_id,
			body,
			status,
			created_at,
			noticed_at,
			read_at,
			acked_at,
			failed_at,
			failure_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, '')
	`,
		record.FromPeerID,
		record.ToPeerID,
		record.FromLabel,
		record.FromTitle,
		replyTo,
		record.Body,
		record.Status,
		record.CreatedAt.UnixMilli(),
	)
	if err != nil {
		return 0, fmt.Errorf("create peer message: %w", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read peer message id: %w", err)
	}
	return messageID, nil
}

func (s *SQLiteStore) ListPeerInbox(peerID string, messageID int64, limit int, includeRead bool) ([]PeerMessageRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	conditions := []string{"to_peer_id = ?"}
	args := []any{peerID}

	if messageID != 0 {
		conditions = append(conditions, "id = ?")
		args = append(args, messageID)
	}
	if includeRead {
		conditions = append(conditions, "status <> ?")
		args = append(args, PeerMessageStatusFailed)
	} else {
		conditions = append(conditions, "status IN (?, ?, ?)")
		args = append(args, PeerMessageStatusQueued, PeerMessageStatusNoticed, PeerMessageStatusRead)
	}

	query := `
		SELECT id, from_peer_id, to_peer_id, from_label, from_title, reply_to_id, body,
		       status, created_at, noticed_at, read_at, acked_at, failed_at, failure_reason
		FROM peer_messages
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("list peer inbox: %w", err)
	}
	defer rows.Close()

	var messages []PeerMessageRecord
	for rows.Next() {
		record, err := scanPeerMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan peer inbox message: %w", err)
		}
		messages = append(messages, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer inbox: %w", err)
	}
	return messages, nil
}

func (s *SQLiteStore) ListRecentPeerMessages(limit int) ([]PeerMessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(context.Background(), `
		SELECT id, from_peer_id, to_peer_id, from_label, from_title, reply_to_id, body,
		       status, created_at, noticed_at, read_at, acked_at, failed_at, failure_reason
		FROM peer_messages
		WHERE status <> ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, PeerMessageStatusFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent peer messages: %w", err)
	}
	defer rows.Close()

	var messages []PeerMessageRecord
	for rows.Next() {
		record, err := scanPeerMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recent peer message: %w", err)
		}
		messages = append(messages, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent peer messages: %w", err)
	}
	return messages, nil
}

func (s *SQLiteStore) DeletePeerMessage(messageID int64) error {
	if messageID <= 0 {
		return fmt.Errorf("message id must be greater than zero")
	}
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM peer_messages WHERE id = ?`, messageID); err != nil {
		return fmt.Errorf("delete peer message: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ClearPeerMessages() error {
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM peer_messages`); err != nil {
		return fmt.Errorf("clear peer messages: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListQueuedPeerMessagesForRuntime(runtimeInstanceID string, limit int) ([]PeerMessageRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(context.Background(), `
		SELECT pm.id, pm.from_peer_id, pm.to_peer_id, pm.from_label, pm.from_title, pm.reply_to_id,
		       pm.body, pm.status, pm.created_at, pm.noticed_at, pm.read_at, pm.acked_at, pm.failed_at, pm.failure_reason
		FROM peer_messages pm
		JOIN peer_registrations pr ON pr.peer_id = pm.to_peer_id
		WHERE pr.runtime_instance_id = ?
		  AND pm.status = ?
		ORDER BY pm.id ASC
		LIMIT ?
	`, runtimeInstanceID, PeerMessageStatusQueued, limit)
	if err != nil {
		return nil, fmt.Errorf("list queued peer messages: %w", err)
	}
	defer rows.Close()

	var messages []PeerMessageRecord
	for rows.Next() {
		record, err := scanPeerMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan queued peer message: %w", err)
		}
		messages = append(messages, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued peer messages: %w", err)
	}
	return messages, nil
}

func (s *SQLiteStore) MarkPeerMessagesNoticed(peerID string, upToMessageID int64, noticedAt time.Time) error {
	if noticedAt.IsZero() {
		noticedAt = time.Now()
	}
	_, err := s.db.ExecContext(context.Background(), `
		UPDATE peer_messages
		SET status = ?,
		    noticed_at = COALESCE(noticed_at, ?)
		WHERE to_peer_id = ?
		  AND status = ?
		  AND id <= ?
	`,
		PeerMessageStatusNoticed,
		noticedAt.UnixMilli(),
		peerID,
		PeerMessageStatusQueued,
		upToMessageID,
	)
	if err != nil {
		return fmt.Errorf("mark peer messages noticed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) MarkPeerMessagesRead(peerID string, messageIDs []int64, readAt time.Time) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if readAt.IsZero() {
		readAt = time.Now()
	}

	placeholders := makePlaceholders(len(messageIDs))
	args := []any{
		PeerMessageStatusRead,
		readAt.UnixMilli(),
		readAt.UnixMilli(),
		peerID,
	}
	for _, messageID := range messageIDs {
		args = append(args, messageID)
	}
	args = append(args, PeerMessageStatusQueued, PeerMessageStatusNoticed, PeerMessageStatusRead)

	query := `
		UPDATE peer_messages
		SET status = ?,
		    noticed_at = COALESCE(noticed_at, ?),
		    read_at = COALESCE(read_at, ?)
		WHERE to_peer_id = ?
		  AND id IN (` + placeholders + `)
		  AND status IN (?, ?, ?)
	`

	if _, err := s.db.ExecContext(context.Background(), query, args...); err != nil {
		return fmt.Errorf("mark peer messages read: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AckPeerMessage(peerID string, messageID int64, ackedAt time.Time) error {
	if ackedAt.IsZero() {
		ackedAt = time.Now()
	}
	_, err := s.db.ExecContext(context.Background(), `
		UPDATE peer_messages
		SET status = ?,
		    noticed_at = COALESCE(noticed_at, ?),
		    read_at = COALESCE(read_at, ?),
		    acked_at = COALESCE(acked_at, ?)
		WHERE to_peer_id = ?
		  AND id = ?
		  AND status IN (?, ?, ?, ?)
	`,
		PeerMessageStatusAcked,
		ackedAt.UnixMilli(),
		ackedAt.UnixMilli(),
		ackedAt.UnixMilli(),
		peerID,
		messageID,
		PeerMessageStatusQueued,
		PeerMessageStatusNoticed,
		PeerMessageStatusRead,
		PeerMessageStatusAcked,
	)
	if err != nil {
		return fmt.Errorf("ack peer message: %w", err)
	}
	return nil
}

func (s *SQLiteStore) FailPeerMessages(peerID string, failedAt time.Time, reason string) error {
	if failedAt.IsZero() {
		failedAt = time.Now()
	}
	_, err := s.db.ExecContext(context.Background(), `
		UPDATE peer_messages
		SET status = ?,
		    failed_at = COALESCE(failed_at, ?),
		    failure_reason = CASE
		        WHEN failure_reason = '' THEN ?
		        ELSE failure_reason
		    END
		WHERE to_peer_id = ?
		  AND status IN (?, ?, ?)
	`,
		PeerMessageStatusFailed,
		failedAt.UnixMilli(),
		reason,
		peerID,
		PeerMessageStatusQueued,
		PeerMessageStatusNoticed,
		PeerMessageStatusRead,
	)
	if err != nil {
		return fmt.Errorf("fail peer messages: %w", err)
	}
	return nil
}

func (s *SQLiteStore) OutstandingPeerCounts(peerIDs []string) (map[string]int, error) {
	return s.peerCountsForStatuses(peerIDs, PeerMessageStatusQueued, PeerMessageStatusNoticed, PeerMessageStatusRead)
}

func (s *SQLiteStore) UnreadPeerCounts(peerIDs []string) (map[string]int, error) {
	return s.peerCountsForStatuses(peerIDs, PeerMessageStatusQueued, PeerMessageStatusNoticed)
}

func (s *SQLiteStore) ExpireStalePeers(before time.Time, failedAt time.Time, reason string) error {
	if before.IsZero() {
		return nil
	}
	if failedAt.IsZero() {
		failedAt = time.Now()
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin expire stale peers transaction: %w", err)
	}
	defer rollback(tx)

	rows, err := tx.QueryContext(context.Background(), `
		SELECT peer_id
		FROM peer_registrations
		WHERE last_heartbeat_at < ?
	`, before.UnixMilli())
	if err != nil {
		return fmt.Errorf("query stale peers: %w", err)
	}

	var peerIDs []string
	for rows.Next() {
		var peerID string
		if err := rows.Scan(&peerID); err != nil {
			rows.Close()
			return fmt.Errorf("scan stale peer id: %w", err)
		}
		peerIDs = append(peerIDs, peerID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate stale peer ids: %w", err)
	}
	rows.Close()

	if len(peerIDs) == 0 {
		return tx.Commit()
	}

	placeholders := makePlaceholders(len(peerIDs))
	args := []any{
		PeerMessageStatusFailed,
		failedAt.UnixMilli(),
		reason,
	}
	for _, peerID := range peerIDs {
		args = append(args, peerID)
	}
	args = append(args, PeerMessageStatusQueued, PeerMessageStatusNoticed, PeerMessageStatusRead)

	updateQuery := `
		UPDATE peer_messages
		SET status = ?,
		    failed_at = COALESCE(failed_at, ?),
		    failure_reason = CASE
		        WHEN failure_reason = '' THEN ?
		        ELSE failure_reason
		    END
		WHERE to_peer_id IN (` + placeholders + `)
		  AND status IN (?, ?, ?)
	`
	if _, err := tx.ExecContext(context.Background(), updateQuery, args...); err != nil {
		return fmt.Errorf("fail stale peer messages: %w", err)
	}

	deleteArgs := make([]any, 0, len(peerIDs))
	for _, peerID := range peerIDs {
		deleteArgs = append(deleteArgs, peerID)
	}
	deleteQuery := `DELETE FROM peer_registrations WHERE peer_id IN (` + placeholders + `)`
	if _, err := tx.ExecContext(context.Background(), deleteQuery, deleteArgs...); err != nil {
		return fmt.Errorf("delete stale peers: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit expire stale peers transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("configure sqlite journal mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("configure sqlite foreign keys: %w", err)
	}

	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	switch {
	case version < 0:
		return fmt.Errorf("unsupported state schema version %d", version)
	case version < 1:
		if err := s.migrateV1(ctx); err != nil {
			return err
		}
		version = 1
		fallthrough
	case version < 2:
		if err := s.migrateV2(ctx); err != nil {
			return err
		}
		version = 2
		fallthrough
	case version < 3:
		if err := s.migrateV3(ctx); err != nil {
			return err
		}
		version = 3
		fallthrough
	case version < 4:
		if err := s.migrateV4(ctx); err != nil {
			return err
		}
	case version == currentSchemaVersion:
		// Current schema.
	case version == maxCompatibleSchemaVersion:
		// Schema v5 only adds columns that this binary does not read or write directly:
		// sessions.session_role and ui_state.preferred_orchestrator_agent_id.
		// Keeping v4 as the writable schema avoids silently fabricating a partial v5 on
		// fresh installs while still allowing existing user state to load.
	default:
		return fmt.Errorf("unsupported state schema version %d", version)
	}

	if err := s.ensureUIStateRow(ctx); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) migrateV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
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
		return fmt.Errorf("create sessions table: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ui_state (
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
		return fmt.Errorf("create ui_state table: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrateV2(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin peer schema migration: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS peer_registrations (
			peer_id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL,
			runtime_instance_id TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			worktree_root_path TEXT NOT NULL,
			repo_key TEXT NOT NULL,
			adapter_id TEXT NOT NULL,
			adapter_family TEXT NOT NULL,
			label TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_heartbeat_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create peer_registrations table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_peer_registrations_runtime
		ON peer_registrations (runtime_instance_id, last_heartbeat_at)
	`); err != nil {
		return fmt.Errorf("create peer_registrations runtime index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_peer_registrations_scope
		ON peer_registrations (repo_key, worktree_root_path)
	`); err != nil {
		return fmt.Errorf("create peer_registrations scope index: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS peer_messages (
			id INTEGER PRIMARY KEY,
			from_peer_id TEXT NOT NULL,
			to_peer_id TEXT NOT NULL,
			from_label TEXT NOT NULL,
			from_title TEXT NOT NULL,
			reply_to_id INTEGER,
			body TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			noticed_at INTEGER,
			read_at INTEGER,
			acked_at INTEGER,
			failed_at INTEGER,
			failure_reason TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("create peer_messages table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_peer_messages_recipient_status
		ON peer_messages (to_peer_id, status, created_at)
	`); err != nil {
		return fmt.Errorf("create peer_messages recipient index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_peer_messages_runtime_queue
		ON peer_messages (status, to_peer_id, id)
	`); err != nil {
		return fmt.Errorf("create peer_messages queue index: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
		return fmt.Errorf("set peer schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit peer schema migration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrateV3(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ui_state utility panel migration: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		ALTER TABLE ui_state
		ADD COLUMN utility_panel_tab TEXT NOT NULL DEFAULT 'diff'
	`); err != nil {
		return fmt.Errorf("add utility_panel_tab column: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 3`); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ui_state utility panel migration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) migrateV4(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ui_state terminal font migration: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		ALTER TABLE ui_state
		ADD COLUMN terminal_font_size INTEGER NOT NULL DEFAULT %d
	`, defaultTerminalFontSize)); err != nil {
		return fmt.Errorf("add terminal_font_size column: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 4`); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ui_state terminal font migration: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureUIStateRow(ctx context.Context) error {
	defaults := DefaultUIState()
	collapsed, err := json.Marshal(defaults.CollapsedRepoKeys)
	if err != nil {
		return fmt.Errorf("encode default collapsed repo keys: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ui_state (
			id,
			last_used_agent_id,
			active_session_id,
			sidebar_open,
			sidebar_width,
			diff_panel_open,
			diff_panel_width,
			terminal_font_size,
			utility_panel_tab,
			collapsed_repo_keys_json
		)
		VALUES (1, ?, NULL, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`,
		defaults.LastUsedAgentID,
		boolToInt(defaults.SidebarOpen),
		defaults.SidebarWidth,
		boolToInt(defaults.DiffPanelOpen),
		defaults.DiffPanelWidth,
		defaults.TerminalFontSize,
		defaults.UtilityPanelTab,
		string(collapsed),
	)
	if err != nil {
		return fmt.Errorf("ensure ui_state row: %w", err)
	}
	return nil
}

func (s *SQLiteStore) loadUIState(ctx context.Context) (UIState, error) {
	var (
		lastUsedAgentID      string
		activeSessionID      sql.NullInt64
		sidebarOpen          int
		sidebarWidth         int
		diffPanelOpen        int
		diffPanelWidth       int
		terminalFontSize     int
		utilityPanelTab      string
		collapsedRepoKeyJSON string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT last_used_agent_id, active_session_id, sidebar_open, sidebar_width, diff_panel_open, diff_panel_width, terminal_font_size, utility_panel_tab, collapsed_repo_keys_json
		FROM ui_state
		WHERE id = 1
	`).Scan(
		&lastUsedAgentID,
		&activeSessionID,
		&sidebarOpen,
		&sidebarWidth,
		&diffPanelOpen,
		&diffPanelWidth,
		&terminalFontSize,
		&utilityPanelTab,
		&collapsedRepoKeyJSON,
	)
	if err != nil {
		return UIState{}, fmt.Errorf("load ui_state row: %w", err)
	}

	var collapsedRepoKeys []string
	if collapsedRepoKeyJSON != "" {
		if err := json.Unmarshal([]byte(collapsedRepoKeyJSON), &collapsedRepoKeys); err != nil {
			return UIState{}, fmt.Errorf("decode collapsed repo keys: %w", err)
		}
	}
	if collapsedRepoKeys == nil {
		collapsedRepoKeys = []string{}
	}

	ui := DefaultUIState()
	ui.LastUsedAgentID = lastUsedAgentID
	if activeSessionID.Valid {
		ui.ActiveSessionID = activeSessionID.Int64
	}
	ui.SidebarOpen = sidebarOpen != 0
	ui.SidebarWidth = sidebarWidth
	ui.DiffPanelOpen = diffPanelOpen != 0
	ui.DiffPanelWidth = diffPanelWidth
	ui.TerminalFontSize = normalizeTerminalFontSize(terminalFontSize)
	ui.UtilityPanelTab = normalizeUtilityPanelTab(utilityPanelTab)
	ui.CollapsedRepoKeys = collapsedRepoKeys
	return ui, nil
}

func updateLastUsedAgentID(ctx context.Context, tx *sql.Tx, lastUsedAgentID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE ui_state
		SET last_used_agent_id = ?
		WHERE id = 1
	`, lastUsedAgentID)
	if err != nil {
		return fmt.Errorf("update last used agent id: %w", err)
	}
	return nil
}

func setActiveSessionID(ctx context.Context, tx *sql.Tx, storageID int64) error {
	var value any
	if storageID != 0 {
		value = storageID
	}
	_, err := tx.ExecContext(ctx, `UPDATE ui_state SET active_session_id = ? WHERE id = 1`, value)
	if err != nil {
		return fmt.Errorf("update active session id: %w", err)
	}
	return nil
}

func rollback(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type peerRegistrationScanner interface {
	Scan(dest ...any) error
}

func scanPeerRegistration(scanner peerRegistrationScanner) (PeerRegistrationRecord, error) {
	var (
		record        PeerRegistrationRecord
		createdAt     int64
		lastHeartbeat int64
	)

	if err := scanner.Scan(
		&record.PeerID,
		&record.TokenHash,
		&record.RuntimeInstanceID,
		&record.SessionID,
		&record.WorktreeRootPath,
		&record.RepoKey,
		&record.AdapterID,
		&record.AdapterFamily,
		&record.Label,
		&record.Title,
		&record.Summary,
		&createdAt,
		&lastHeartbeat,
	); err != nil {
		return PeerRegistrationRecord{}, err
	}

	record.CreatedAt = time.UnixMilli(createdAt)
	record.LastHeartbeatAt = time.UnixMilli(lastHeartbeat)
	return record, nil
}

type peerMessageScanner interface {
	Scan(dest ...any) error
}

func scanPeerMessage(scanner peerMessageScanner) (PeerMessageRecord, error) {
	var (
		record    PeerMessageRecord
		replyToID sql.NullInt64
		createdAt int64
		noticedAt sql.NullInt64
		readAt    sql.NullInt64
		ackedAt   sql.NullInt64
		failedAt  sql.NullInt64
	)

	if err := scanner.Scan(
		&record.ID,
		&record.FromPeerID,
		&record.ToPeerID,
		&record.FromLabel,
		&record.FromTitle,
		&replyToID,
		&record.Body,
		&record.Status,
		&createdAt,
		&noticedAt,
		&readAt,
		&ackedAt,
		&failedAt,
		&record.FailureReason,
	); err != nil {
		return PeerMessageRecord{}, err
	}

	record.CreatedAt = time.UnixMilli(createdAt)
	if replyToID.Valid {
		record.ReplyToID = replyToID.Int64
	}
	if noticedAt.Valid {
		record.NoticedAt = time.UnixMilli(noticedAt.Int64)
	}
	if readAt.Valid {
		record.ReadAt = time.UnixMilli(readAt.Int64)
	}
	if ackedAt.Valid {
		record.AckedAt = time.UnixMilli(ackedAt.Int64)
	}
	if failedAt.Valid {
		record.FailedAt = time.UnixMilli(failedAt.Int64)
	}
	return record, nil
}

func makePlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func (s *SQLiteStore) peerCountsForStatuses(peerIDs []string, statuses ...string) (map[string]int, error) {
	counts := make(map[string]int, len(peerIDs))
	if len(peerIDs) == 0 || len(statuses) == 0 {
		return counts, nil
	}

	query := `
		SELECT to_peer_id, COUNT(*)
		FROM peer_messages
		WHERE to_peer_id IN (` + makePlaceholders(len(peerIDs)) + `)
		  AND status IN (` + makePlaceholders(len(statuses)) + `)
		GROUP BY to_peer_id
	`

	args := make([]any, 0, len(peerIDs)+len(statuses))
	for _, peerID := range peerIDs {
		args = append(args, peerID)
	}
	for _, status := range statuses {
		args = append(args, status)
	}

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("query peer counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var peerID string
		var count int
		if err := rows.Scan(&peerID, &count); err != nil {
			return nil, fmt.Errorf("scan peer count: %w", err)
		}
		counts[peerID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer counts: %w", err)
	}
	return counts, nil
}
