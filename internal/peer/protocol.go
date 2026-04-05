package peer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const InterruptVersion = "1"

type InterruptEnvelope struct {
	MessageID   int64
	FromPeer    string
	FromLabel   string
	Kind        string
	AckRequired bool
	UnreadCount int
	NextCommand string
	Preview     string
}

func NewCredentials(prefix string) (string, string, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	peerBytes := make([]byte, 6)
	if _, err := rand.Read(peerBytes); err != nil {
		return "", "", err
	}

	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		prefix = "peer"
	}
	peerID := fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(peerBytes))
	token := hex.EncodeToString(tokenBytes)
	return peerID, token, nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func FormatInterrupt(envelope InterruptEnvelope) string {
	kind := strings.TrimSpace(envelope.Kind)
	if kind == "" {
		kind = "message"
	}
	nextCommand := strings.TrimSpace(envelope.NextCommand)
	if nextCommand == "" {
		nextCommand = fmt.Sprintf("helm peers inbox --message-id %d", envelope.MessageID)
	}

	preview := strings.Join(strings.Fields(envelope.Preview), " ")
	if len(preview) > 400 {
		preview = preview[:397] + "..."
	}

	return fmt.Sprintf(
		"\n[HELM_PEER_EVENT v%s]\nmessage_id=%d\nfrom_peer=%s\nfrom_label=%s\nkind=%s\nack_required=%t\nunread_count=%d\nnext_command=%s\npreview=%s\n[/HELM_PEER_EVENT]\n",
		InterruptVersion,
		envelope.MessageID,
		envelope.FromPeer,
		envelope.FromLabel,
		kind,
		envelope.AckRequired,
		envelope.UnreadCount,
		nextCommand,
		preview,
	)
}
