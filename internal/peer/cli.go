package peer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	persist "helm-wails/internal/state"
)

type CLI struct {
	Stdout io.Writer
	Stderr io.Writer
	Now    func() time.Time
}

func (cli CLI) Run(args []string) error {
	if cli.Stdout == nil {
		cli.Stdout = os.Stdout
	}
	if cli.Stderr == nil {
		cli.Stderr = os.Stderr
	}
	if cli.Now == nil {
		cli.Now = time.Now
	}
	if len(args) == 0 || isTopLevelHelp(args[0]) {
		cli.writeUsage()
		return nil
	}

	if isSubcommandHelp(args[1:]) {
		return suppressHelp(cli.runWithoutStore(args[0], args[1:]))
	}

	dbPath := strings.TrimSpace(os.Getenv("HELM_PEER_DB_PATH"))
	if dbPath == "" {
		return errors.New("HELM_PEER_DB_PATH is not set")
	}

	store, err := openStoreForCommand(dbPath, args)
	if err != nil {
		return err
	}
	defer func() {
		_ = store.Close()
	}()

	return cli.runWithStore(store, args[0], args[1:])
}

func (cli CLI) runWithoutStore(command string, args []string) error {
	switch command {
	case "list":
		return cli.runList(nil, args)
	case "send":
		return cli.runSend(nil, args)
	case "inbox":
		return cli.runInbox(nil, args)
	case "ack":
		return cli.runAck(nil, args)
	case "set-summary":
		return cli.runSetSummary(nil, args)
	default:
		return fmt.Errorf("unknown peers subcommand %q", command)
	}
}

func (cli CLI) runWithStore(store *persist.SQLiteStore, command string, args []string) error {
	switch command {
	case "list":
		return cli.runList(store, args)
	case "send":
		return cli.runSend(store, args)
	case "inbox":
		return cli.runInbox(store, args)
	case "ack":
		return cli.runAck(store, args)
	case "set-summary":
		return cli.runSetSummary(store, args)
	default:
		return fmt.Errorf("unknown peers subcommand %q", command)
	}
}

func isTopLevelHelp(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func isSubcommandHelp(args []string) bool {
	for _, arg := range args {
		if isTopLevelHelp(arg) {
			return true
		}
	}
	return false
}

func suppressHelp(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func (cli CLI) writeUsage() {
	_, _ = fmt.Fprint(cli.Stdout, "Usage:\n")
	_, _ = fmt.Fprint(cli.Stdout, "  <helm-binary> peers list [--scope repo|worktree|machine] [--include-self] [--json]\n")
	_, _ = fmt.Fprint(cli.Stdout, "  <helm-binary> peers send --to <peer-id> --message <text> [--reply-to <message-id>] [--scope repo|worktree|machine] [--json]\n")
	_, _ = fmt.Fprint(cli.Stdout, "  <helm-binary> peers inbox [--message-id <id>] [--limit <n>] [--include-read] [--mark-read] [--json]\n")
	_, _ = fmt.Fprint(cli.Stdout, "  <helm-binary> peers ack --message-id <id> [--json]\n")
	_, _ = fmt.Fprint(cli.Stdout, "  <helm-binary> peers set-summary --summary <text> [--json]\n")
	_, _ = fmt.Fprint(cli.Stdout, "\nInside Helm-managed peer-enabled sessions, a `helm` wrapper is also available on PATH.\n")
}

func openStoreForCommand(dbPath string, args []string) (*persist.SQLiteStore, error) {
	if commandUsesReadOnlyStore(args) {
		return persist.OpenSQLiteStoreReadOnly(dbPath)
	}
	return persist.OpenSQLiteStore(dbPath)
}

func commandUsesReadOnlyStore(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "list":
		return true
	case "inbox":
		return !inboxMarksRead(args[1:])
	default:
		return false
	}
}

func inboxMarksRead(args []string) bool {
	markRead := false
	for _, arg := range args {
		if value, ok := boolFlagValue(arg, "mark-read"); ok {
			markRead = value
			continue
		}
		if value, ok := boolFlagValue(arg, "peek"); ok {
			markRead = !value
		}
	}
	return markRead
}

func boolFlagValue(arg string, name string) (bool, bool) {
	for _, prefix := range []string{"--", "-"} {
		flagName := prefix + name
		switch {
		case arg == flagName:
			return true, true
		case strings.HasPrefix(arg, flagName+"="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, flagName+"="))
			if value == "" {
				return true, true
			}
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return false, false
			}
			return parsed, true
		}
	}
	return false, false
}

func displayPeerName(item peerListItem) string {
	if worktree := worktreeLabel(item.WorktreeRootPath); worktree != "" {
		return worktree
	}
	if strings.TrimSpace(item.Title) != "" {
		return item.Title
	}
	return item.Label
}

func worktreeLabel(root string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == string(filepath.Separator) || root == "" {
		return ""
	}
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return ""
	}
	return name
}

type peerListItem struct {
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
	LastHeartbeatUnixMs int64  `json:"lastHeartbeatUnixMs"`
}

func (cli CLI) runList(store *persist.SQLiteStore, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	scope := fs.String("scope", persist.PeerScopeRepo, "peer discovery scope")
	includeSelf := fs.Bool("include-self", false, "include the current peer")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	current, err := currentPeerRegistration(store)
	if err != nil {
		return err
	}

	records, err := store.ListPeerRegistrations(persist.PeerListFilter{
		Scope:        *scope,
		SelfPeerID:   current.PeerID,
		RepoKey:      current.RepoKey,
		WorktreeRoot: current.WorktreeRootPath,
		IncludeSelf:  *includeSelf,
	})
	if err != nil {
		return err
	}

	peerIDs := make([]string, 0, len(records))
	for _, record := range records {
		peerIDs = append(peerIDs, record.PeerID)
	}
	outstanding, _ := store.OutstandingPeerCounts(peerIDs)
	unread, _ := store.UnreadPeerCounts(peerIDs)

	items := make([]peerListItem, 0, len(records))
	for _, record := range records {
		items = append(items, peerListItem{
			PeerID:              record.PeerID,
			SessionID:           record.SessionID,
			AdapterID:           record.AdapterID,
			AdapterFamily:       record.AdapterFamily,
			Label:               record.Label,
			Title:               record.Title,
			Summary:             record.Summary,
			RepoKey:             record.RepoKey,
			WorktreeRootPath:    record.WorktreeRootPath,
			OutstandingCount:    outstanding[record.PeerID],
			UnreadCount:         unread[record.PeerID],
			LastHeartbeatUnixMs: record.LastHeartbeatAt.UnixMilli(),
		})
	}

	if *asJSON {
		return writeJSON(cli.Stdout, items)
	}
	for _, item := range items {
		_, _ = fmt.Fprintf(cli.Stdout, "%s\t%s\t%s\n", item.PeerID, displayPeerName(item), item.Summary)
	}
	return nil
}

func (cli CLI) runSend(store *persist.SQLiteStore, args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	toPeerID := fs.String("to", "", "recipient peer id")
	message := fs.String("message", "", "message body")
	replyToID := fs.Int64("reply-to", 0, "message id being answered")
	scope := fs.String("scope", "", "peer reachability scope: repo, worktree, or machine")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*toPeerID) == "" {
		return errors.New("--to is required")
	}
	if strings.TrimSpace(*message) == "" {
		return errors.New("--message is required")
	}
	toPeer := strings.TrimSpace(*toPeerID)

	current, err := currentPeerRegistration(store)
	if err != nil {
		return err
	}
	if *replyToID != 0 {
		if _, err := replyTargetPeer(store, current, toPeer, *replyToID); err != nil {
			return err
		}
	} else {
		if _, err := visibleTargetPeer(store, current, toPeer, *scope); err != nil {
			return err
		}
	}

	record := persist.PeerMessageRecord{
		FromPeerID: current.PeerID,
		ToPeerID:   toPeer,
		FromLabel:  current.Label,
		FromTitle:  current.Title,
		ReplyToID:  *replyToID,
		Body:       strings.TrimSpace(*message),
		Status:     persist.PeerMessageStatusQueued,
		CreatedAt:  cli.Now(),
	}

	messageID, err := store.CreatePeerMessage(record)
	if err != nil {
		return err
	}
	if *replyToID != 0 {
		if err := store.AckPeerMessage(current.PeerID, *replyToID, cli.Now()); err != nil {
			return err
		}
	}

	if *asJSON {
		return writeJSON(cli.Stdout, map[string]any{
			"id":        messageID,
			"status":    persist.PeerMessageStatusQueued,
			"replyToId": *replyToID,
			"toPeerId":  toPeer,
		})
	}
	_, _ = fmt.Fprintf(cli.Stdout, "queued message %d for %s\n", messageID, toPeer)
	return nil
}

func (cli CLI) runInbox(store *persist.SQLiteStore, args []string) error {
	fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	messageID := fs.Int64("message-id", 0, "specific message id")
	limit := fs.Int("limit", 20, "maximum number of messages")
	includeRead := fs.Bool("include-read", false, "include read and acknowledged messages")
	peek := fs.Bool("peek", false, "read without changing message state (default)")
	markRead := fs.Bool("mark-read", false, "mark returned messages as read")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = peek
	_ = markRead

	current, err := currentPeerRegistration(store)
	if err != nil {
		return err
	}

	messages, err := store.ListPeerInbox(current.PeerID, *messageID, *limit, *includeRead)
	if err != nil {
		return err
	}
	if inboxMarksRead(args) {
		messageIDs := make([]int64, 0, len(messages))
		for _, message := range messages {
			messageIDs = append(messageIDs, message.ID)
		}
		if err := store.MarkPeerMessagesRead(current.PeerID, messageIDs, cli.Now()); err != nil {
			return err
		}
		for index := range messages {
			if messages[index].ReadAt.IsZero() {
				messages[index].ReadAt = cli.Now()
			}
			messages[index].Status = persist.PeerMessageStatusRead
		}
	}

	if *asJSON {
		return writeJSON(cli.Stdout, messages)
	}
	for _, message := range messages {
		_, _ = fmt.Fprintf(cli.Stdout, "[%d] %s: %s\n", message.ID, message.FromLabel, message.Body)
	}
	return nil
}

func (cli CLI) runAck(store *persist.SQLiteStore, args []string) error {
	fs := flag.NewFlagSet("ack", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	messageID := fs.Int64("message-id", 0, "message id to acknowledge")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *messageID == 0 {
		return errors.New("--message-id is required")
	}

	current, err := currentPeerRegistration(store)
	if err != nil {
		return err
	}
	if err := store.AckPeerMessage(current.PeerID, *messageID, cli.Now()); err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(cli.Stdout, map[string]any{
			"id":     *messageID,
			"status": persist.PeerMessageStatusAcked,
		})
	}
	_, _ = fmt.Fprintf(cli.Stdout, "acknowledged %d\n", *messageID)
	return nil
}

func (cli CLI) runSetSummary(store *persist.SQLiteStore, args []string) error {
	fs := flag.NewFlagSet("set-summary", flag.ContinueOnError)
	fs.SetOutput(cli.Stderr)

	summary := fs.String("summary", "", "current work summary")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	current, err := currentPeerRegistration(store)
	if err != nil {
		return err
	}
	if err := store.UpdatePeerSummary(current.PeerID, strings.TrimSpace(*summary), cli.Now()); err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(cli.Stdout, map[string]any{
			"peerId":  current.PeerID,
			"summary": strings.TrimSpace(*summary),
		})
	}
	_, _ = fmt.Fprintf(cli.Stdout, "updated summary for %s\n", current.PeerID)
	return nil
}

func currentPeerRegistration(store *persist.SQLiteStore) (persist.PeerRegistrationRecord, error) {
	peerID := strings.TrimSpace(os.Getenv("HELM_PEER_ID"))
	token := readPeerToken()
	if peerID == "" || token == "" {
		return persist.PeerRegistrationRecord{}, errors.New("peer session credentials are not set")
	}

	record, err := store.GetPeerRegistration(peerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persist.PeerRegistrationRecord{}, errors.New("peer session is no longer registered")
		}
		return persist.PeerRegistrationRecord{}, err
	}
	if record.TokenHash != HashToken(token) {
		return persist.PeerRegistrationRecord{}, errors.New("peer session credentials are invalid")
	}
	return record, nil
}

func replyTargetPeer(store *persist.SQLiteStore, current persist.PeerRegistrationRecord, targetPeerID string, replyToID int64) (persist.PeerRegistrationRecord, error) {
	messages, err := store.ListPeerInbox(current.PeerID, replyToID, 1, true)
	if err != nil {
		return persist.PeerRegistrationRecord{}, err
	}
	if len(messages) == 0 {
		return persist.PeerRegistrationRecord{}, fmt.Errorf("reply target message %d is not in your inbox", replyToID)
	}
	parent := messages[0]
	if parent.FromPeerID != targetPeerID {
		return persist.PeerRegistrationRecord{}, fmt.Errorf("reply target %q does not match original sender %q for message %d", targetPeerID, parent.FromPeerID, replyToID)
	}

	record, err := store.GetPeerRegistration(targetPeerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persist.PeerRegistrationRecord{}, fmt.Errorf("peer %q is not registered or is offline", targetPeerID)
		}
		return persist.PeerRegistrationRecord{}, err
	}
	return record, nil
}

func visibleTargetPeer(store *persist.SQLiteStore, current persist.PeerRegistrationRecord, targetPeerID string, requestedScope string) (persist.PeerRegistrationRecord, error) {
	record, err := store.GetPeerRegistration(targetPeerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persist.PeerRegistrationRecord{}, fmt.Errorf("peer %q is not registered or is offline", targetPeerID)
		}
		return persist.PeerRegistrationRecord{}, err
	}

	scope, err := normalizedScope(requestedScope)
	if err != nil {
		return persist.PeerRegistrationRecord{}, err
	}
	if scope == "" {
		scope = discoveryScopeFromEnv()
	}
	records, err := store.ListPeerRegistrations(persist.PeerListFilter{
		Scope:        scope,
		SelfPeerID:   current.PeerID,
		RepoKey:      current.RepoKey,
		WorktreeRoot: current.WorktreeRootPath,
		IncludeSelf:  true,
	})
	if err != nil {
		return persist.PeerRegistrationRecord{}, err
	}
	for _, candidate := range records {
		if candidate.PeerID == targetPeerID {
			return record, nil
		}
	}
	return persist.PeerRegistrationRecord{}, fmt.Errorf("peer %q is not reachable in %s scope", targetPeerID, scope)
}

func discoveryScopeFromEnv() string {
	scope := strings.TrimSpace(os.Getenv(DefaultScopeEnv))
	switch scope {
	case "", persist.PeerScopeRepo:
		return persist.PeerScopeRepo
	case persist.PeerScopeWorktree:
		return persist.PeerScopeWorktree
	case persist.PeerScopeMachine:
		return persist.PeerScopeMachine
	default:
		return persist.PeerScopeRepo
	}
}

func normalizedScope(scope string) (string, error) {
	switch strings.TrimSpace(scope) {
	case "":
		return "", nil
	case persist.PeerScopeRepo:
		return persist.PeerScopeRepo, nil
	case persist.PeerScopeWorktree:
		return persist.PeerScopeWorktree, nil
	case persist.PeerScopeMachine:
		return persist.PeerScopeMachine, nil
	default:
		return "", fmt.Errorf("unsupported peer scope %q", scope)
	}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// readPeerToken returns the peer token, preferring a token file (pointed to by
// HELM_PEER_TOKEN_FILE) over the HELM_PEER_TOKEN environment variable. The file
// approach avoids leaking credentials via process environment listings.
func readPeerToken() string {
	if tokenFile := strings.TrimSpace(os.Getenv("HELM_PEER_TOKEN_FILE")); tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return strings.TrimSpace(os.Getenv("HELM_PEER_TOKEN"))
}
