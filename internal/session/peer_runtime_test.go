package session

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"helm-wails/internal/peer"
	persist "helm-wails/internal/state"
)

func TestPeerRuntimeDeliverQueuedWritesInterruptAndDedupes(t *testing.T) {
	t.Parallel()

	store, err := persist.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	var writes []string
	runtime := newPeerRuntime(store, nil, func(sessionID int, data string) error {
		if sessionID != 42 {
			t.Fatalf("write sessionID = %d, want 42", sessionID)
		}
		writes = append(writes, data)
		return nil
	}, nil)

	launch := &peerLaunchState{
		PeerID: "receiver",
		Token:  "receiver-token",
	}
	if err := runtime.registerSession(42, launch, "/tmp/repo", "repo-key", "codex", "codex", "Receiver", "Receiver"); err != nil {
		t.Fatalf("registerSession() error = %v", err)
	}

	now := time.Unix(1710000000, 0)
	firstID, err := store.CreatePeerMessage(persist.PeerMessageRecord{
		FromPeerID: "sender",
		ToPeerID:   "receiver",
		FromLabel:  "Sender",
		FromTitle:  "Sender",
		Body:       "First update",
		Status:     persist.PeerMessageStatusQueued,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(first) error = %v", err)
	}
	secondID, err := store.CreatePeerMessage(persist.PeerMessageRecord{
		FromPeerID: "sender",
		ToPeerID:   "receiver",
		FromLabel:  "Sender",
		FromTitle:  "Sender",
		Body:       "Second update",
		Status:     persist.PeerMessageStatusQueued,
		CreatedAt:  now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("CreatePeerMessage(second) error = %v", err)
	}

	if err := runtime.deliverQueued(now.Add(2 * time.Second)); err != nil {
		t.Fatalf("deliverQueued() error = %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("writes len = %d, want 1", len(writes))
	}
	if !strings.Contains(writes[0], "[HELM_PEER_EVENT v1]") {
		t.Fatalf("interrupt = %q, want HELM_PEER_EVENT envelope", writes[0])
	}
	if !strings.Contains(writes[0], "message_id="+strconv.FormatInt(secondID, 10)) {
		t.Fatalf("interrupt = %q, want latest message id", writes[0])
	}
	if !strings.Contains(writes[0], "unread_count=2") {
		t.Fatalf("interrupt = %q, want coalesced unread_count=2", writes[0])
	}

	inbox, err := store.ListPeerInbox("receiver", 0, 10, false)
	if err != nil {
		t.Fatalf("ListPeerInbox() error = %v", err)
	}
	if len(inbox) != 2 {
		t.Fatalf("ListPeerInbox() len = %d, want 2", len(inbox))
	}
	for _, message := range inbox {
		if message.Status != persist.PeerMessageStatusNoticed {
			t.Fatalf("message status = %q, want noticed", message.Status)
		}
	}

	if err := runtime.deliverQueued(now.Add(3 * time.Second)); err != nil {
		t.Fatalf("second deliverQueued() error = %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("writes len after duplicate delivery = %d, want 1", len(writes))
	}

	if err := store.MarkPeerMessagesRead("receiver", []int64{firstID, secondID}, now.Add(4*time.Second)); err != nil {
		t.Fatalf("MarkPeerMessagesRead() error = %v", err)
	}
}

func TestPeerRuntimeTokenFileHasRestrictivePermissionsAndIsRemovedOnUnregister(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions are not enforced on this platform")
	}

	supportRoot := t.TempDir()
	t.Setenv("HELM_PEER_SUPPORT_ROOT", supportRoot)

	support, err := peer.NewSupportManager("/tmp/helm-binary")
	if err != nil {
		t.Fatalf("NewSupportManager() error = %v", err)
	}

	store, err := persist.OpenSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	r := newPeerRuntime(store, support, func(int, string) error { return nil }, nil)

	tokenPath, err := r.writePeerTokenFile("shell-abcdef", "secret-token-value")
	if err != nil {
		t.Fatalf("writePeerTokenFile() error = %v", err)
	}

	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat(token) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token file perms = %o, want 0600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(tokenPath))
	if err != nil {
		t.Fatalf("Stat(token dir) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("token dir perms = %o, want 0700", got)
	}

	contents, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("ReadFile(token) error = %v", err)
	}
	if string(contents) != "secret-token-value" {
		t.Fatalf("token file contents = %q, want secret-token-value", contents)
	}

	// Register so unregisterSession has state to work with, then unregister
	// and verify the token file is removed from disk.
	if err := r.registerSession(7, &peerLaunchState{PeerID: "shell-abcdef", Token: "secret-token-value"}, "/tmp/repo", "repo-key", "shell", "shell", "shell", "shell"); err != nil {
		t.Fatalf("registerSession() error = %v", err)
	}
	r.unregisterSession(7, "test-cleanup")

	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("token file still present after unregister: err = %v", err)
	}
}
