package peer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	persist "helm-wails/internal/state"
)

func TestCLIRoundTripListSendInboxReplyAckAndSummary(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"
	receiverToken := "receiver-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken(receiverToken),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Receiver",
			Title:             "Receiver",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)

	asPeer := func(peerID string, token string) CLI {
		t.Setenv("HELM_PEER_ID", peerID)
		t.Setenv("HELM_PEER_TOKEN", token)
		return CLI{
			Now: func() time.Time {
				return now.Add(2 * time.Minute)
			},
		}
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("sender", senderToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"list", "--json"}); err != nil {
			t.Fatalf("CLI.Run(list) error = %v", err)
		}

		var peers []struct {
			PeerID string `json:"peerId"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &peers); err != nil {
			t.Fatalf("json.Unmarshal(list) error = %v", err)
		}
		if len(peers) != 1 || peers[0].PeerID != "receiver" {
			t.Fatalf("list peers = %#v, want receiver only", peers)
		}
	}

	var originalMessageID int64
	{
		var stdout bytes.Buffer
		cli := asPeer("sender", senderToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"send", "--to", "receiver", "--message", "Client API changed", "--json"}); err != nil {
			t.Fatalf("CLI.Run(send) error = %v", err)
		}

		var result struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("json.Unmarshal(send) error = %v", err)
		}
		if result.ID == 0 {
			t.Fatalf("send result = %#v, want non-zero id", result)
		}
		originalMessageID = result.ID
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("sender", senderToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"set-summary", "--summary", "Reviewing API changes", "--json"}); err != nil {
			t.Fatalf("CLI.Run(set-summary) error = %v", err)
		}

		record, err := store.GetPeerRegistration("sender")
		if err != nil {
			t.Fatalf("GetPeerRegistration(sender) error = %v", err)
		}
		if record.Summary != "Reviewing API changes" {
			t.Fatalf("sender summary = %q, want updated summary", record.Summary)
		}
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("receiver", receiverToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"inbox", "--message-id", fmt.Sprint(originalMessageID), "--json"}); err != nil {
			t.Fatalf("CLI.Run(inbox) error = %v", err)
		}

		var inbox []struct {
			ID     int64  `json:"ID"`
			Body   string `json:"Body"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &inbox); err != nil {
			t.Fatalf("json.Unmarshal(inbox) error = %v", err)
		}
		if len(inbox) != 1 || inbox[0].ID != originalMessageID {
			t.Fatalf("inbox = %#v, want original message", inbox)
		}
		if inbox[0].Status != persist.PeerMessageStatusQueued {
			t.Fatalf("inbox status = %q, want %q", inbox[0].Status, persist.PeerMessageStatusQueued)
		}
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("receiver", receiverToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"inbox", "--message-id", fmt.Sprint(originalMessageID), "--mark-read", "--json"}); err != nil {
			t.Fatalf("CLI.Run(inbox --mark-read) error = %v", err)
		}

		var inbox []struct {
			ID     int64  `json:"ID"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &inbox); err != nil {
			t.Fatalf("json.Unmarshal(inbox --mark-read) error = %v", err)
		}
		if len(inbox) != 1 || inbox[0].ID != originalMessageID {
			t.Fatalf("mark-read inbox = %#v, want original message", inbox)
		}
		if inbox[0].Status != persist.PeerMessageStatusRead {
			t.Fatalf("mark-read inbox status = %q, want %q", inbox[0].Status, persist.PeerMessageStatusRead)
		}
	}

	var replyMessageID int64
	{
		var stdout bytes.Buffer
		cli := asPeer("receiver", receiverToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"send", "--to", "sender", "--reply-to", fmt.Sprint(originalMessageID), "--message", "Acknowledged", "--json"}); err != nil {
			t.Fatalf("CLI.Run(reply send) error = %v", err)
		}

		var result struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("json.Unmarshal(reply send) error = %v", err)
		}
		replyMessageID = result.ID
	}

	originalMessages, err := store.ListPeerInbox("receiver", originalMessageID, 1, true)
	if err != nil {
		t.Fatalf("ListPeerInbox(receiver) error = %v", err)
	}
	if len(originalMessages) != 1 || originalMessages[0].Status != persist.PeerMessageStatusAcked {
		t.Fatalf("original messages = %#v, want acked original", originalMessages)
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("sender", senderToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"inbox", "--message-id", fmt.Sprint(replyMessageID), "--json"}); err != nil {
			t.Fatalf("CLI.Run(sender inbox) error = %v", err)
		}

		var inbox []struct {
			ID        int64  `json:"ID"`
			ReplyToID int64  `json:"ReplyToID"`
			Status    string `json:"Status"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &inbox); err != nil {
			t.Fatalf("json.Unmarshal(sender inbox) error = %v", err)
		}
		if len(inbox) != 1 || inbox[0].ID != replyMessageID || inbox[0].ReplyToID != originalMessageID {
			t.Fatalf("sender inbox = %#v, want reply message", inbox)
		}
	}

	{
		var stdout bytes.Buffer
		cli := asPeer("sender", senderToken)
		cli.Stdout = &stdout
		cli.Stderr = &bytes.Buffer{}

		if err := cli.Run([]string{"ack", "--message-id", fmt.Sprint(replyMessageID), "--json"}); err != nil {
			t.Fatalf("CLI.Run(ack) error = %v", err)
		}

		var result struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("json.Unmarshal(ack) error = %v", err)
		}
		if result.Status != persist.PeerMessageStatusAcked {
			t.Fatalf("ack result = %#v, want acked status", result)
		}
	}

	replyMessages, err := store.ListPeerInbox("sender", replyMessageID, 1, true)
	if err != nil {
		t.Fatalf("ListPeerInbox(sender) error = %v", err)
	}
	if len(replyMessages) != 1 || replyMessages[0].Status != persist.PeerMessageStatusAcked {
		t.Fatalf("reply messages = %#v, want acked reply", replyMessages)
	}
}

func TestCLISendRejectsUnknownAndOutOfScopePeers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "other-repo-peer",
			TokenHash:         HashToken("other-token"),
			RuntimeInstanceID: "runtime-b",
			SessionID:         9,
			WorktreeRootPath:  "/tmp/other",
			RepoKey:           "other-repo",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Other",
			Title:             "Other",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "sender")
	t.Setenv("HELM_PEER_TOKEN", senderToken)
	t.Setenv(DefaultScopeEnv, persist.PeerScopeRepo)

	newCLI := func() CLI {
		return CLI{
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
			Now: func() time.Time {
				return now
			},
		}
	}

	if err := newCLI().Run([]string{"send", "--to", "missing-peer", "--message", "hello"}); err == nil {
		t.Fatalf("Run(send missing peer) error = nil, want failure")
	}

	if err := newCLI().Run([]string{"send", "--to", "other-repo-peer", "--message", "hello"}); err == nil {
		t.Fatalf("Run(send out-of-scope peer) error = nil, want failure")
	}

	if err := newCLI().Run([]string{"send", "--scope", persist.PeerScopeMachine, "--to", "other-repo-peer", "--message", "hello"}); err != nil {
		t.Fatalf("Run(send machine scope) error = %v, want success", err)
	}

	messages, err := store.ListRecentPeerMessages(10)
	if err != nil {
		t.Fatalf("ListRecentPeerMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].ToPeerID != "other-repo-peer" {
		t.Fatalf("messages = %#v, want one queued machine-scope message", messages)
	}
}

func TestCLIHelpDoesNotRequirePeerEnvironment(t *testing.T) {
	newCLI := func(stdout *bytes.Buffer, stderr *bytes.Buffer) CLI {
		return CLI{
			Stdout: stdout,
			Stderr: stderr,
		}
	}

	{
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if err := newCLI(&stdout, &stderr).Run([]string{"--help"}); err != nil {
			t.Fatalf("Run(--help) error = %v", err)
		}
		if !strings.Contains(stdout.String(), "<helm-binary> peers send --to <peer-id>") {
			t.Fatalf("stdout = %q, want top-level usage", stdout.String())
		}
	}

	{
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if err := newCLI(&stdout, &stderr).Run([]string{"send", "--help"}); err != nil {
			t.Fatalf("Run(send --help) error = %v", err)
		}
		if !strings.Contains(stderr.String(), "-scope") {
			t.Fatalf("stderr = %q, want send help with scope flag", stderr.String())
		}
	}
}

func TestCLIReplyToInboundMessageIgnoresDefaultScope(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"
	receiverToken := "receiver-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo-a",
			RepoKey:           "repo-a",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken(receiverToken),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo-b",
			RepoKey:           "repo-b",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Receiver",
			Title:             "Receiver",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	parentID, err := store.CreatePeerMessage(persist.PeerMessageRecord{
		FromPeerID: "sender",
		ToPeerID:   "receiver",
		FromLabel:  "Sender",
		FromTitle:  "Sender",
		Body:       "Need status",
		Status:     persist.PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage() error = %v", err)
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "receiver")
	t.Setenv("HELM_PEER_TOKEN", receiverToken)
	t.Setenv(DefaultScopeEnv, persist.PeerScopeRepo)

	var stdout bytes.Buffer
	cli := CLI{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return now.Add(time.Minute)
		},
	}

	if err := cli.Run([]string{"send", "--to", "sender", "--reply-to", fmt.Sprint(parentID), "--message", "I am idle", "--json"}); err != nil {
		t.Fatalf("CLI.Run(reply cross-repo) error = %v", err)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(reply) error = %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("reply result = %#v, want non-zero id", result)
	}

	parentMessages, err := store.ListPeerInbox("receiver", parentID, 1, true)
	if err != nil {
		t.Fatalf("ListPeerInbox(receiver) error = %v", err)
	}
	if len(parentMessages) != 1 || parentMessages[0].Status != persist.PeerMessageStatusAcked {
		t.Fatalf("parent messages = %#v, want acked parent", parentMessages)
	}
}

func TestCLIListReadsFromReadOnlyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"
	receiverToken := "receiver-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken(receiverToken),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Receiver",
			Title:             "Receiver",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	if err := os.Chmod(dbPath, 0o444); err != nil {
		t.Fatalf("Chmod(%s) error = %v", dbPath, err)
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "sender")
	t.Setenv("HELM_PEER_TOKEN", senderToken)

	var stdout bytes.Buffer
	cli := CLI{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return now
		},
	}

	if err := cli.Run([]string{"list", "--json"}); err != nil {
		t.Fatalf("CLI.Run(list --json) error = %v", err)
	}

	var peers []struct {
		PeerID string `json:"peerId"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &peers); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(peers) != 1 || peers[0].PeerID != "receiver" {
		t.Fatalf("list peers = %#v, want receiver only", peers)
	}
}

func TestCLIListTextShowsSessionTitle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Codex",
			Title:             "Codex 1",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken("receiver-token"),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Codex",
			Title:             "Codex 2",
			Summary:           "Reviewing peer runtime",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "sender")
	t.Setenv("HELM_PEER_TOKEN", senderToken)

	var stdout bytes.Buffer
	cli := CLI{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return now
		},
	}

	if err := cli.Run([]string{"list", "--scope", "repo"}); err != nil {
		t.Fatalf("CLI.Run(list text) error = %v", err)
	}

	got := stdout.String()
	want := "receiver\trepo\tReviewing peer runtime\n"
	if got != want {
		t.Fatalf("list output = %q, want %q", got, want)
	}
}

func TestReadPeerTokenPrefersTokenFileOverEnvironment(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "peer.token")
	if err := os.WriteFile(tokenPath, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}

	t.Setenv("HELM_PEER_TOKEN_FILE", tokenPath)
	t.Setenv("HELM_PEER_TOKEN", "env-token")

	got := readPeerToken()
	if got != "file-token" {
		t.Fatalf("readPeerToken() = %q, want file-token (token file must win over env)", got)
	}
}

func TestReadPeerTokenFallsBackToEnvironmentWhenFileMissing(t *testing.T) {
	t.Setenv("HELM_PEER_TOKEN_FILE", filepath.Join(t.TempDir(), "missing.token"))
	t.Setenv("HELM_PEER_TOKEN", "env-token")

	if got := readPeerToken(); got != "env-token" {
		t.Fatalf("readPeerToken() = %q, want env-token fallback", got)
	}
}

func TestCLIAuthenticatesUsingTokenFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken("receiver-token"),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Receiver",
			Title:             "Receiver",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	tokenFile := filepath.Join(t.TempDir(), "sender.token")
	if err := os.WriteFile(tokenFile, []byte(senderToken), 0o600); err != nil {
		t.Fatalf("WriteFile(token file) error = %v", err)
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "sender")
	t.Setenv("HELM_PEER_TOKEN_FILE", tokenFile)
	// HELM_PEER_TOKEN intentionally unset: authentication must succeed
	// purely through the token file.
	os.Unsetenv("HELM_PEER_TOKEN")

	var stdout bytes.Buffer
	cli := CLI{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return now
		},
	}

	if err := cli.Run([]string{"list", "--json"}); err != nil {
		t.Fatalf("CLI.Run(list) error = %v", err)
	}

	var peers []struct {
		PeerID string `json:"peerId"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &peers); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(peers) != 1 || peers[0].PeerID != "receiver" {
		t.Fatalf("list peers = %#v, want receiver only", peers)
	}
}

func TestCLIInboxDefaultsToReadOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := persist.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}

	now := time.Unix(1710000000, 0)
	senderToken := "sender-token"
	receiverToken := "receiver-token"

	for _, record := range []persist.PeerRegistrationRecord{
		{
			PeerID:            "sender",
			TokenHash:         HashToken(senderToken),
			RuntimeInstanceID: "runtime-a",
			SessionID:         1,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "codex",
			AdapterFamily:     "codex",
			Label:             "Sender",
			Title:             "Sender",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
		{
			PeerID:            "receiver",
			TokenHash:         HashToken(receiverToken),
			RuntimeInstanceID: "runtime-b",
			SessionID:         2,
			WorktreeRootPath:  "/tmp/repo",
			RepoKey:           "repo-key",
			AdapterID:         "claude-code",
			AdapterFamily:     "claude",
			Label:             "Receiver",
			Title:             "Receiver",
			CreatedAt:         now,
			LastHeartbeatAt:   now,
		},
	} {
		if err := store.UpsertPeerRegistration(record); err != nil {
			t.Fatalf("UpsertPeerRegistration(%s) error = %v", record.PeerID, err)
		}
	}

	messageID, err := store.CreatePeerMessage(persist.PeerMessageRecord{
		FromPeerID: "sender",
		ToPeerID:   "receiver",
		FromLabel:  "Sender",
		FromTitle:  "Sender",
		Body:       "Status update?",
		Status:     persist.PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage() error = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}
	if err := os.Chmod(dbPath, 0o444); err != nil {
		t.Fatalf("Chmod(%s) error = %v", dbPath, err)
	}

	t.Setenv("HELM_PEER_DB_PATH", dbPath)
	t.Setenv("HELM_PEER_ID", "receiver")
	t.Setenv("HELM_PEER_TOKEN", receiverToken)

	var stdout bytes.Buffer
	cli := CLI{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		Now: func() time.Time {
			return now
		},
	}

	if err := cli.Run([]string{"inbox", "--message-id", fmt.Sprint(messageID), "--json"}); err != nil {
		t.Fatalf("CLI.Run(inbox default) error = %v", err)
	}

	var inbox []struct {
		ID     int64  `json:"ID"`
		Status string `json:"Status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &inbox); err != nil {
		t.Fatalf("json.Unmarshal(inbox default) error = %v", err)
	}
	if len(inbox) != 1 || inbox[0].ID != messageID {
		t.Fatalf("inbox = %#v, want queued message", inbox)
	}
	if inbox[0].Status != persist.PeerMessageStatusQueued {
		t.Fatalf("inbox status = %q, want %q", inbox[0].Status, persist.PeerMessageStatusQueued)
	}
}
