import { ArrowRight, X } from "lucide-react";

import type {
  LivePeerViewModel,
  PeerMessageViewModel,
} from "../../hooks/usePeerPanelModel";
import { SpinnerIcon } from "../icons";

type PeerPanelProps = {
  clearingPeerMessages: boolean;
  deletingPeerMessageId: number | null;
  livePeers: LivePeerViewModel[];
  onClearPeerMessages: () => void;
  onDeletePeerMessage: (messageId: number) => void;
  recentMessages: PeerMessageViewModel[];
};

export function PeerPanel(props: PeerPanelProps) {
  const {
    clearingPeerMessages,
    deletingPeerMessageId,
    livePeers,
    onClearPeerMessages,
    onDeletePeerMessage,
    recentMessages,
  } = props;

  if (livePeers.length === 0 && recentMessages.length === 0) {
    return (
      <div className="diff-panel__empty">
        No Helm-managed peers are connected yet.
      </div>
    );
  }

  return (
    <div className="diff-panel__body">
      <section className="diff-section">
        <div className="diff-section__header">
          <strong>Live peers</strong>
          <span>{livePeers.length}</span>
        </div>
        {livePeers.length ? (
          <div className="peer-list">
            {livePeers.map((peer) => (
              <article className="peer-card" key={peer.peerId}>
                <div className="peer-card__header">
                  <div className="peer-card__titles">
                    <strong>{peer.displayName}</strong>
                    <small>{peer.peerId}</small>
                  </div>
                  <div className="peer-card__badges">
                    {peer.isSelf ? (
                      <span className="peer-pill peer-pill--self">Local</span>
                    ) : null}
                    {peer.unreadCount > 0 ? (
                      <span className="peer-pill">{peer.unreadCount} unread</span>
                    ) : null}
                    {peer.outstandingCount > 0 ? (
                      <span className="peer-pill peer-pill--active">
                        {peer.outstandingCount} open
                      </span>
                    ) : null}
                  </div>
                </div>
                <div className="peer-card__meta">
                  <span>{peer.adapterLabel}</span>
                  <span>{peer.locationLabel}</span>
                  <span>{peer.heartbeatLabel}</span>
                </div>
                <p className="peer-card__summary">{peer.summary}</p>
              </article>
            ))}
          </div>
        ) : (
          <div className="diff-section__empty">No live peers found.</div>
        )}
      </section>

      <section className="diff-section">
        <div className="diff-section__header">
          <strong>Recent messages</strong>
          <div className="diff-section__header-actions">
            {recentMessages.length ? (
              <button
                className="ghost-button diff-section__action"
                disabled={
                  clearingPeerMessages || deletingPeerMessageId !== null
                }
                type="button"
                onClick={onClearPeerMessages}
              >
                {clearingPeerMessages ? "Clearing..." : "Clear all"}
              </button>
            ) : null}
            <span>{recentMessages.length}</span>
          </div>
        </div>
        {recentMessages.length ? (
          <div className="peer-message-list">
            {recentMessages.map((message) => (
              <article className="peer-message-card" key={message.id}>
                <div className="peer-message-card__top">
                  <strong>#{message.id}</strong>
                  <div className="peer-message-card__top-actions">
                    <span className={message.statusClassName}>
                      {message.status}
                    </span>
                    <button
                      aria-label={`Delete peer message ${message.id}`}
                      className="icon-button peer-message-card__delete"
                      disabled={
                        clearingPeerMessages ||
                        deletingPeerMessageId === message.id
                      }
                      title="Delete message"
                      type="button"
                      onClick={() => onDeletePeerMessage(message.id)}
                    >
                      {message.isDeleting ? (
                        <SpinnerIcon aria-hidden="true" size={12} />
                      ) : (
                        <X aria-hidden="true" size={13} strokeWidth={1.9} />
                      )}
                    </button>
                  </div>
                </div>
                <div className="peer-message-card__route">
                  <span>{message.fromLabel}</span>
                  <ArrowRight
                    aria-hidden="true"
                    className="peer-message-card__route-arrow"
                    size={12}
                    strokeWidth={1.8}
                  />
                  <span>{message.toLabel}</span>
                </div>
                <p className="peer-message-card__body">{message.body}</p>
                <div className="peer-message-card__meta">
                  <span>{message.createdAtLabel}</span>
                  {message.replyToId ? (
                    <span>Reply to #{message.replyToId}</span>
                  ) : null}
                  {message.failureReason ? (
                    <span>{message.failureReason}</span>
                  ) : null}
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="diff-section__empty">
            No peer messages have been exchanged yet.
          </div>
        )}
      </section>
    </div>
  );
}
