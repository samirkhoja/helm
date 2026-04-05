package session

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
