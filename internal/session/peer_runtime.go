package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"helm-wails/internal/agent"
	"helm-wails/internal/peer"
	persist "helm-wails/internal/state"
)

const (
	peerHeartbeatInterval  = 5 * time.Second
	peerPollInterval       = 1 * time.Second
	peerExpiryWindow       = 30 * time.Second
	peerRecentMessageLimit = 50
)

type peerLaunchState struct {
	PeerID string
	Token  string
}

type localPeerState struct {
	SessionID                int
	PeerID                   string
	TokenHash                string
	WorktreeRootPath         string
	RepoKey                  string
	AdapterID                string
	AdapterFamily            string
	Label                    string
	Title                    string
	LastHeartbeatAt          time.Time
	LastInterruptedMessageID int64
	LastInterruptedUnread    int
}

type sessionPeerDecoration struct {
	PeerID               string
	PeerCapable          bool
	PeerSummary          string
	OutstandingPeerCount int
}

type peerRuntime struct {
	store     persist.Store
	support   *peer.SupportManager
	writeFunc func(sessionID int, data string) error
	onChange  func()

	runtimeID string

	mu             sync.RWMutex
	localBySession map[int]*localPeerState
	localByPeerID  map[string]*localPeerState
	cachedSnapshot PeerStateDTO
	snapshotValid  bool
	lastDigest     string
	stopCh         chan struct{}
	doneCh         chan struct{}
}

func newPeerRuntime(store persist.Store, support *peer.SupportManager, writeFunc func(int, string) error, onChange func()) *peerRuntime {
	runtimeID, _, err := peer.NewCredentials("runtime")
	if err != nil {
		runtimeID = fmt.Sprintf("runtime-%d", time.Now().UnixNano())
	}

	return &peerRuntime{
		store:          store,
		support:        support,
		writeFunc:      writeFunc,
		onChange:       onChange,
		runtimeID:      runtimeID,
		localBySession: map[int]*localPeerState{},
		localByPeerID:  map[string]*localPeerState{},
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
}

func (r *peerRuntime) start() {
	if r == nil {
		return
	}
	go r.watchLoop()
}

func (r *peerRuntime) stop() {
	if r == nil {
		return
	}
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
	<-r.doneCh
}

func (r *peerRuntime) prepareLaunch(spec agent.LaunchSpec) (agent.LaunchSpec, *peerLaunchState, error) {
	if r == nil || !spec.PeerEnabled {
		return spec, nil, nil
	}

	env, launch, err := r.prepareSessionShellEnv(spec.Env)
	if err != nil {
		return spec, nil, err
	}
	spec.Env = env

	spec, err = r.prepareShellAdapterSpec(spec)
	if err != nil {
		return spec, nil, err
	}
	return spec, launch, nil
}

func (r *peerRuntime) prepareSessionShellEnv(env []string) ([]string, *peerLaunchState, error) {
	if r == nil {
		return env, nil, nil
	}

	launchSupport, err := r.support.PrepareLaunch(peer.FamilyGeneric, env)
	if err != nil {
		return env, nil, err
	}

	prefix := "shell"
	peerID, token, err := peer.NewCredentials(prefix)
	if err != nil {
		return env, nil, err
	}

	tokenFile, err := r.writePeerTokenFile(peerID, token)
	if err != nil {
		return env, nil, err
	}

	env = mergeLaunchEnv(env, launchSupport.Env)
	env = mergeLaunchEnv(env, map[string]string{
		"HELM_PEER_ID":                peerID,
		"HELM_PEER_TOKEN_FILE":        tokenFile,
		"HELM_PEER_DB_PATH":           r.store.DBPath(),
		"HELM_PEER_SUPPORT_ROOT":      r.support.Root(),
		peer.DefaultScopeEnv:          persist.PeerScopeRepo,
		"HELM_PEER_INTERRUPT_VERSION": peer.InterruptVersion,
		"HELM_PEER_RUNTIME_INSTANCE":  r.runtimeID,
	})

	return env, &peerLaunchState{
		PeerID: peerID,
		Token:  token,
	}, nil
}

func (r *peerRuntime) peerTokensDir() string {
	if r == nil || r.support == nil {
		return ""
	}
	return filepath.Join(r.support.Root(), "tokens", r.runtimeID)
}

func (r *peerRuntime) writePeerTokenFile(peerID, token string) (string, error) {
	dir := r.peerTokensDir()
	if dir == "" {
		return "", fmt.Errorf("peer support root is not configured")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create peer token directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("tighten peer token directory permissions: %w", err)
	}
	tokenPath := filepath.Join(dir, peerID+".token")
	if err := os.WriteFile(tokenPath, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("write peer token file: %w", err)
	}
	return tokenPath, nil
}

func (r *peerRuntime) removePeerTokenFile(peerID string) {
	dir := r.peerTokensDir()
	if dir == "" || strings.TrimSpace(peerID) == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, peerID+".token"))
}

func (r *peerRuntime) prepareShellAdapterSpec(spec agent.LaunchSpec) (agent.LaunchSpec, error) {
	if r == nil || !spec.PeerEnabled {
		return spec, nil
	}

	launchSupport, err := r.support.PrepareLaunch(spec.Family, spec.Env)
	if err != nil {
		return spec, err
	}

	spec.Args = append(spec.Args, launchSupport.ExtraArgs...)
	spec.Env = mergeLaunchEnv(spec.Env, launchSupport.Env)
	return spec, nil
}

func (r *peerRuntime) registerSession(sessionID int, launch *peerLaunchState, worktreeRootPath, repoKey, adapterID, adapterFamily, label, title string) error {
	if r == nil || launch == nil {
		return nil
	}

	now := time.Now()
	record := persist.PeerRegistrationRecord{
		PeerID:            launch.PeerID,
		TokenHash:         peer.HashToken(launch.Token),
		RuntimeInstanceID: r.runtimeID,
		SessionID:         sessionID,
		WorktreeRootPath:  worktreeRootPath,
		RepoKey:           repoKey,
		AdapterID:         adapterID,
		AdapterFamily:     adapterFamily,
		Label:             label,
		Title:             title,
		Summary:           "",
		CreatedAt:         now,
		LastHeartbeatAt:   now,
	}
	if err := r.store.UpsertPeerRegistration(record); err != nil {
		return err
	}

	state := &localPeerState{
		SessionID:        sessionID,
		PeerID:           launch.PeerID,
		TokenHash:        record.TokenHash,
		WorktreeRootPath: worktreeRootPath,
		RepoKey:          repoKey,
		AdapterID:        adapterID,
		AdapterFamily:    adapterFamily,
		Label:            label,
		Title:            title,
		LastHeartbeatAt:  now,
	}

	r.mu.Lock()
	r.localBySession[sessionID] = state
	r.localByPeerID[state.PeerID] = state
	r.snapshotValid = false
	r.mu.Unlock()
	return nil
}

func (r *peerRuntime) unregisterSession(sessionID int, reason string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	state := r.localBySession[sessionID]
	if state != nil {
		delete(r.localBySession, sessionID)
		delete(r.localByPeerID, state.PeerID)
	}
	r.snapshotValid = false
	r.mu.Unlock()

	if state == nil {
		return
	}

	_ = r.store.FailPeerMessages(state.PeerID, time.Now(), reason)
	_ = r.store.RemovePeerRegistration(state.PeerID)
	r.removePeerTokenFile(state.PeerID)
}

func (r *peerRuntime) invalidateSnapshot() {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.snapshotValid = false
	r.mu.Unlock()
}

func (r *peerRuntime) sessionDecorations() map[int]sessionPeerDecoration {
	out := map[int]sessionPeerDecoration{}
	if r == nil {
		return out
	}

	_, peerByID := r.localPeers()
	snapshot := r.snapshot()
	peerBySnapshotID := make(map[string]PeerDTO, len(snapshot.Peers))
	for _, peerState := range snapshot.Peers {
		peerBySnapshotID[peerState.PeerID] = peerState
	}

	for peerID, local := range peerByID {
		peerState := peerBySnapshotID[peerID]
		out[local.SessionID] = sessionPeerDecoration{
			PeerID:               peerID,
			PeerCapable:          true,
			PeerSummary:          peerState.Summary,
			OutstandingPeerCount: peerState.OutstandingCount,
		}
	}
	return out
}

func (r *peerRuntime) snapshot() PeerStateDTO {
	if r == nil {
		return PeerStateDTO{}
	}

	r.mu.RLock()
	if r.snapshotValid {
		snapshot := r.cachedSnapshot
		r.mu.RUnlock()
		return snapshot
	}
	r.mu.RUnlock()

	snapshot, _ := r.refreshSnapshot(false)
	return snapshot
}

func (r *peerRuntime) buildSnapshot() PeerStateDTO {
	if r == nil {
		return PeerStateDTO{}
	}

	registrations, err := r.store.ListPeerRegistrations(persist.PeerListFilter{
		Scope:       persist.PeerScopeMachine,
		IncludeSelf: true,
	})
	if err != nil {
		return PeerStateDTO{}
	}

	peerIDs := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		peerIDs = append(peerIDs, registration.PeerID)
	}

	outstanding, _ := r.store.OutstandingPeerCounts(peerIDs)
	unread, _ := r.store.UnreadPeerCounts(peerIDs)

	r.mu.RLock()
	localPeerIDs := make(map[string]struct{}, len(r.localByPeerID))
	for peerID := range r.localByPeerID {
		localPeerIDs[peerID] = struct{}{}
	}
	r.mu.RUnlock()

	peers := make([]PeerDTO, 0, len(registrations))
	for _, registration := range registrations {
		_, isSelf := localPeerIDs[registration.PeerID]
		peers = append(peers, PeerDTO{
			PeerID:              registration.PeerID,
			SessionID:           registration.SessionID,
			AdapterID:           registration.AdapterID,
			AdapterFamily:       registration.AdapterFamily,
			Label:               registration.Label,
			Title:               registration.Title,
			Summary:             registration.Summary,
			RepoKey:             registration.RepoKey,
			WorktreeRootPath:    registration.WorktreeRootPath,
			OutstandingCount:    outstanding[registration.PeerID],
			UnreadCount:         unread[registration.PeerID],
			IsSelf:              isSelf,
			LastHeartbeatUnixMs: registration.LastHeartbeatAt.UnixMilli(),
		})
	}

	recentMessages, err := r.store.ListRecentPeerMessages(peerRecentMessageLimit)
	if err != nil {
		return PeerStateDTO{Peers: peers}
	}

	messages := make([]PeerMessageDTO, 0, len(recentMessages))
	for _, message := range recentMessages {
		messages = append(messages, PeerMessageDTO{
			ID:              message.ID,
			FromPeerID:      message.FromPeerID,
			ToPeerID:        message.ToPeerID,
			FromLabel:       message.FromLabel,
			FromTitle:       message.FromTitle,
			ReplyToID:       message.ReplyToID,
			Body:            message.Body,
			Status:          message.Status,
			CreatedAtUnixMs: message.CreatedAt.UnixMilli(),
			NoticedAtUnixMs: nullableUnixMilli(message.NoticedAt),
			ReadAtUnixMs:    nullableUnixMilli(message.ReadAt),
			AckedAtUnixMs:   nullableUnixMilli(message.AckedAt),
			FailedAtUnixMs:  nullableUnixMilli(message.FailedAt),
			FailureReason:   message.FailureReason,
		})
	}

	return PeerStateDTO{
		Peers:    peers,
		Messages: messages,
	}
}

func (r *peerRuntime) watchLoop() {
	defer close(r.doneCh)

	ticker := time.NewTicker(peerPollInterval)
	defer ticker.Stop()

	lastHeartbeatAt := time.Now().Add(-peerHeartbeatInterval)
	r.refreshSnapshot(false)

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			lastHeartbeatAt = r.tick(lastHeartbeatAt)
		}
	}
}

func (r *peerRuntime) tick(lastHeartbeatAt time.Time) time.Time {
	if r == nil {
		return lastHeartbeatAt
	}

	now := time.Now()
	if now.Sub(lastHeartbeatAt) >= peerHeartbeatInterval {
		_ = r.heartbeat(now)
		_ = r.store.ExpireStalePeers(now.Add(-peerExpiryWindow), now, "peer session is offline")
		lastHeartbeatAt = now
	}
	_ = r.deliverQueued(now)
	_, _ = r.refreshSnapshot(true)
	return lastHeartbeatAt
}

func (r *peerRuntime) heartbeat(now time.Time) error {
	peerIDs, _ := r.localPeers()
	for _, peerID := range peerIDs {
		if err := r.store.UpdatePeerHeartbeat(peerID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *peerRuntime) deliverQueued(now time.Time) error {
	queued, err := r.store.ListQueuedPeerMessagesForRuntime(r.runtimeID, 100)
	if err != nil {
		return err
	}
	if len(queued) == 0 {
		return nil
	}

	type deliveryState struct {
		latest     persist.PeerMessageRecord
		messageIDs []int64
	}
	grouped := map[string]deliveryState{}
	for _, message := range queued {
		state := grouped[message.ToPeerID]
		state.messageIDs = append(state.messageIDs, message.ID)
		if message.ID > state.latest.ID {
			state.latest = message
		}
		grouped[message.ToPeerID] = state
	}

	unreadCounts, _ := r.store.UnreadPeerCounts(mapKeys(grouped))

	for peerID, delivery := range grouped {
		local := r.localPeer(peerID)
		if local == nil {
			continue
		}
		unreadCount := unreadCounts[peerID]
		if local.LastInterruptedMessageID == delivery.latest.ID && local.LastInterruptedUnread == unreadCount {
			continue
		}

		if err := r.store.MarkPeerMessagesNoticed(peerID, delivery.latest.ID, now); err != nil {
			return err
		}

		envelope := peer.FormatInterrupt(peer.InterruptEnvelope{
			MessageID:   delivery.latest.ID,
			FromPeer:    delivery.latest.FromPeerID,
			FromLabel:   delivery.latest.FromLabel,
			Kind:        "message",
			AckRequired: true,
			UnreadCount: maxInt(unreadCount, len(delivery.messageIDs)),
			NextCommand: fmt.Sprintf("helm peers inbox --message-id %d", delivery.latest.ID),
			Preview:     delivery.latest.Body,
		})
		if err := r.writeFunc(local.SessionID, envelope); err != nil {
			return err
		}

		r.mu.Lock()
		if current := r.localByPeerID[peerID]; current != nil {
			current.LastInterruptedMessageID = delivery.latest.ID
			current.LastInterruptedUnread = unreadCount
		}
		r.mu.Unlock()
	}
	return nil
}

func (r *peerRuntime) refreshSnapshot(notify bool) (PeerStateDTO, bool) {
	if r == nil {
		return PeerStateDTO{}, false
	}
	snapshot := r.buildSnapshot()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return snapshot, false
	}
	digest := string(payload)

	var onChange func()
	changed := false
	r.mu.Lock()
	changed = !r.snapshotValid || digest != r.lastDigest
	r.cachedSnapshot = snapshot
	r.snapshotValid = true
	if !changed {
		r.mu.Unlock()
		return snapshot, false
	}
	r.lastDigest = digest
	if notify {
		onChange = r.onChange
	}
	r.mu.Unlock()

	if onChange != nil {
		onChange()
	}
	return snapshot, true
}

func (r *peerRuntime) localPeers() ([]string, map[string]*localPeerState) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peerIDs := make([]string, 0, len(r.localByPeerID))
	peerByID := make(map[string]*localPeerState, len(r.localByPeerID))
	for peerID, local := range r.localByPeerID {
		peerIDs = append(peerIDs, peerID)
		peerByID[peerID] = &localPeerState{
			SessionID:                local.SessionID,
			PeerID:                   local.PeerID,
			TokenHash:                local.TokenHash,
			WorktreeRootPath:         local.WorktreeRootPath,
			RepoKey:                  local.RepoKey,
			AdapterID:                local.AdapterID,
			AdapterFamily:            local.AdapterFamily,
			Label:                    local.Label,
			Title:                    local.Title,
			LastHeartbeatAt:          local.LastHeartbeatAt,
			LastInterruptedMessageID: local.LastInterruptedMessageID,
			LastInterruptedUnread:    local.LastInterruptedUnread,
		}
	}
	return peerIDs, peerByID
}

func (r *peerRuntime) localPeer(peerID string) *localPeerState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	local := r.localByPeerID[peerID]
	if local == nil {
		return nil
	}
	copy := *local
	return &copy
}

func mergeLaunchEnv(existing []string, additions map[string]string) []string {
	if len(additions) == 0 {
		return existing
	}

	envMap := make(map[string]string, len(existing)+len(additions))
	for _, item := range existing {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			envMap[key] = value
		}
	}
	for key, value := range additions {
		envMap[key] = value
	}

	out := make([]string, 0, len(envMap))
	for key, value := range envMap {
		out = append(out, key+"="+value)
	}
	return out
}

func nullableUnixMilli(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func mapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
