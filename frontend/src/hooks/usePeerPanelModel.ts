import { useMemo, useState } from "react";

import {
  clearPeerMessages,
  confirmClearPeerMessages,
  deletePeerMessage,
} from "../backend";
import type { PeerDTO, PeerMessageDTO, PeerStateDTO } from "../types";
import { trimPathLabel } from "../utils/appShell";

export type LivePeerViewModel = {
  adapterLabel: string;
  displayName: string;
  heartbeatLabel: string;
  isSelf: boolean;
  locationLabel: string;
  outstandingCount: number;
  peerId: string;
  summary: string;
  unreadCount: number;
};

export type PeerMessageViewModel = {
  body: string;
  createdAtLabel: string;
  failureReason: string;
  fromLabel: string;
  id: number;
  isDeleting: boolean;
  replyToId: number;
  status: string;
  statusClassName: string;
  toLabel: string;
};

type UsePeerPanelModelOptions = {
  onError: (error: unknown) => void;
  peerState: PeerStateDTO;
  sessionLabelByPeerId: Map<string, string>;
  setPeerState: (value: PeerStateDTO | ((current: PeerStateDTO) => PeerStateDTO)) => void;
};

function formatPanelTimestamp(unixMs: number) {
  if (!unixMs) {
    return "";
  }

  return new Date(unixMs).toLocaleString([], {
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    month: "short",
  });
}

function peerDisplayName(
  peer: PeerDTO | null | undefined,
  sessionLabelByPeerId: Map<string, string>,
) {
  if (!peer) {
    return "Unknown peer";
  }

  const sessionLabel = sessionLabelByPeerId.get(peer.peerId);
  if (sessionLabel) {
    return sessionLabel;
  }

  return peer.title || peer.label || peer.peerId;
}

function peerMessageStatusClass(status: string) {
  switch (status) {
    case "queued":
      return "peer-status peer-status--queued";
    case "noticed":
      return "peer-status peer-status--noticed";
    case "read":
      return "peer-status peer-status--read";
    case "acked":
      return "peer-status peer-status--acked";
    case "failed":
      return "peer-status peer-status--failed";
    default:
      return "peer-status";
  }
}

export function usePeerPanelModel(options: UsePeerPanelModelOptions) {
  const { onError, peerState, sessionLabelByPeerId, setPeerState } = options;

  const [deletingPeerMessageId, setDeletingPeerMessageId] = useState<
    number | null
  >(null);
  const [clearingPeerMessages, setClearingPeerMessages] = useState(false);

  const peerById = useMemo(
    () => new Map(peerState.peers.map((peer) => [peer.peerId, peer])),
    [peerState.peers],
  );

  const sortedPeers = useMemo(() => {
    return [...peerState.peers].sort((left, right) => {
      if (left.isSelf !== right.isSelf) {
        return left.isSelf ? -1 : 1;
      }
      if (left.outstandingCount !== right.outstandingCount) {
        return right.outstandingCount - left.outstandingCount;
      }
      if (left.unreadCount !== right.unreadCount) {
        return right.unreadCount - left.unreadCount;
      }
      if (left.lastHeartbeatUnixMs !== right.lastHeartbeatUnixMs) {
        return right.lastHeartbeatUnixMs - left.lastHeartbeatUnixMs;
      }
      return peerDisplayName(left, sessionLabelByPeerId).localeCompare(
        peerDisplayName(right, sessionLabelByPeerId),
      );
    });
  }, [peerState.peers, sessionLabelByPeerId]);

  const recentPeerMessages = useMemo(() => {
    return [...peerState.messages].sort(
      (left, right) => right.createdAtUnixMs - left.createdAtUnixMs,
    );
  }, [peerState.messages]);

  const localOutstandingPeerCount = useMemo(() => {
    return peerState.peers.reduce((sum, peer) => {
      if (!peer.isSelf) {
        return sum;
      }
      return sum + peer.outstandingCount;
    }, 0);
  }, [peerState.peers]);

  const livePeers = useMemo<LivePeerViewModel[]>(() => {
    return sortedPeers.map((peer) => ({
      adapterLabel: peer.adapterFamily || peer.adapterId,
      displayName: peerDisplayName(peer, sessionLabelByPeerId),
      heartbeatLabel: formatPanelTimestamp(peer.lastHeartbeatUnixMs),
      isSelf: peer.isSelf,
      locationLabel: trimPathLabel(peer.worktreeRootPath || peer.repoKey, 2),
      outstandingCount: peer.outstandingCount,
      peerId: peer.peerId,
      summary: peer.summary || "No summary published yet.",
      unreadCount: peer.unreadCount,
    }));
  }, [sessionLabelByPeerId, sortedPeers]);

  const recentMessages = useMemo<PeerMessageViewModel[]>(() => {
    return recentPeerMessages.map((message: PeerMessageDTO) => {
      const fromPeer = peerById.get(message.fromPeerId);
      const toPeer = peerById.get(message.toPeerId);

      return {
        body: message.body,
        createdAtLabel: formatPanelTimestamp(message.createdAtUnixMs),
        failureReason: message.failureReason,
        fromLabel: peerDisplayName(fromPeer, sessionLabelByPeerId),
        id: message.id,
        isDeleting: deletingPeerMessageId === message.id,
        replyToId: message.replyToId,
        status: message.status,
        statusClassName: peerMessageStatusClass(message.status),
        toLabel: peerDisplayName(toPeer, sessionLabelByPeerId),
      };
    });
  }, [
    deletingPeerMessageId,
    peerById,
    recentPeerMessages,
    sessionLabelByPeerId,
  ]);

  const handleDeletePeerMessage = async (messageId: number) => {
    setDeletingPeerMessageId(messageId);
    try {
      const nextState = await deletePeerMessage(messageId);
      setPeerState(nextState);
    } catch (error) {
      onError(error);
    } finally {
      setDeletingPeerMessageId((current) =>
        current === messageId ? null : current,
      );
    }
  };

  const handleClearPeerMessages = async () => {
    if (recentPeerMessages.length === 0) {
      return;
    }

    const shouldClear = await confirmClearPeerMessages();
    if (!shouldClear) {
      return;
    }

    setClearingPeerMessages(true);
    try {
      const nextState = await clearPeerMessages();
      setPeerState(nextState);
    } catch (error) {
      onError(error);
    } finally {
      setClearingPeerMessages(false);
    }
  };

  return {
    clearingPeerMessages,
    deletingPeerMessageId,
    handleClearPeerMessages,
    handleDeletePeerMessage,
    livePeerCount: livePeers.length,
    livePeers,
    localOutstandingPeerCount,
    recentMessageCount: recentMessages.length,
    recentMessages,
  };
}
