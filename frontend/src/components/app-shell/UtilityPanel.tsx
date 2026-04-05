import { type CSSProperties, type ReactNode } from "react";
import { Maximize2, Minimize2, Minus, Plus, RotateCw, X } from "lucide-react";

import type {
  DiffBodyState,
  DiffTarget,
} from "../../hooks/useDiffPanel";
import type {
  LivePeerViewModel,
  PeerMessageViewModel,
} from "../../hooks/usePeerPanelModel";
import type { UtilityPanelTab } from "../../utils/appShell";
import { DiffPanel } from "./DiffPanel";
import { PeerPanel } from "./PeerPanel";

type UtilityPanelProps = {
  clearingPeerMessages: boolean;
  deletingPeerMessageId: number | null;
  diffBodyState: DiffBodyState;
  diffPanelStyle: CSSProperties;
  diffTextZoom: number;
  filesBody: ReactNode;
  filesMeta: string | null;
  filesTitle: string;
  fullscreen: boolean;
  isOpen: boolean;
  livePeerCount: number;
  livePeers: LivePeerViewModel[];
  onClearPeerMessages: () => void;
  onClose: () => void;
  onDeletePeerMessage: (messageId: number) => void;
  onRefreshDiff: () => void;
  onResetDiffTextZoom: () => void;
  onToggleDiffTarget: (target: DiffTarget) => void;
  onToggleFullscreen: () => void;
  onZoomInDiff: () => void;
  onZoomOutDiff: () => void;
  recentMessageCount: number;
  recentMessages: PeerMessageViewModel[];
  repoName: string | null;
  tab: UtilityPanelTab;
  utilityLabel: string | null;
  zoomInEnabled: boolean;
  zoomOutEnabled: boolean;
};

export function UtilityPanel(props: UtilityPanelProps) {
  const {
    clearingPeerMessages,
    deletingPeerMessageId,
    diffBodyState,
    diffPanelStyle,
    diffTextZoom,
    filesBody,
    filesMeta,
    filesTitle,
    fullscreen,
    isOpen,
    livePeerCount,
    livePeers,
    onClearPeerMessages,
    onClose,
    onDeletePeerMessage,
    onRefreshDiff,
    onResetDiffTextZoom,
    onToggleDiffTarget,
    onToggleFullscreen,
    onZoomInDiff,
    onZoomOutDiff,
    recentMessageCount,
    recentMessages,
    repoName,
    tab,
    utilityLabel,
    zoomInEnabled,
    zoomOutEnabled,
  } = props;

  return (
    <aside
      className={`diff-panel${isOpen ? "" : " is-collapsed"}${fullscreen ? " is-fullscreen" : ""}`}
      style={tab === "diff" ? diffPanelStyle : undefined}
    >
      {isOpen ? (
        <div className="diff-panel__content">
          <div className="diff-panel__header">
            <div>
              <div className="eyebrow">
                {tab === "diff"
                  ? "Git diff"
                  : tab === "files"
                    ? "Files"
                    : "Peer network"}
              </div>
              <h2>
                {tab === "diff"
                  ? repoName ?? "No repo"
                  : tab === "files"
                    ? filesTitle
                    : "Helm peers"}
              </h2>
              {tab === "diff" ? (
                utilityLabel ? (
                  <p className="diff-panel__meta">{utilityLabel}</p>
                ) : null
              ) : tab === "files" ? (
                filesMeta ? <p className="diff-panel__meta">{filesMeta}</p> : null
              ) : (
                <p className="diff-panel__meta">
                  {livePeerCount} live peer
                  {livePeerCount === 1 ? "" : "s"} • {recentMessageCount} recent
                  {" "}message
                  {recentMessageCount === 1 ? "" : "s"}
                </p>
              )}
            </div>

            <div className="diff-panel__actions">
              {tab === "diff" ? (
                <>
                  <button
                    aria-label="Refresh diff"
                    className="ghost-button diff-panel__control"
                    title="Refresh diff"
                    type="button"
                    onClick={onRefreshDiff}
                  >
                    <RotateCw
                      absoluteStrokeWidth
                      aria-hidden="true"
                      className="diff-panel__control-icon"
                      size={15}
                      strokeWidth={1.65}
                    />
                  </button>
                  <button
                    aria-label="Zoom out diff text"
                    className="ghost-button diff-panel__control"
                    disabled={!zoomOutEnabled}
                    title="Zoom out diff text"
                    type="button"
                    onClick={onZoomOutDiff}
                  >
                    <Minus
                      absoluteStrokeWidth
                      aria-hidden="true"
                      className="diff-panel__control-icon"
                      size={16}
                      strokeWidth={1.85}
                    />
                  </button>
                  <button
                    aria-label="Reset diff text zoom"
                    className="ghost-button diff-panel__zoom-readout"
                    title="Reset diff text zoom"
                    type="button"
                    onClick={onResetDiffTextZoom}
                  >
                    {diffTextZoom}%
                  </button>
                  <button
                    aria-label="Zoom in diff text"
                    className="ghost-button diff-panel__control"
                    disabled={!zoomInEnabled}
                    title="Zoom in diff text"
                    type="button"
                    onClick={onZoomInDiff}
                  >
                    <Plus
                      absoluteStrokeWidth
                      aria-hidden="true"
                      className="diff-panel__control-icon"
                      size={16}
                      strokeWidth={1.85}
                    />
                  </button>
                  <button
                    aria-label={
                      fullscreen
                        ? "Return diff panel to split view"
                        : "Expand diff panel to fullscreen"
                    }
                    className="ghost-button diff-panel__control"
                    title={
                      fullscreen
                        ? "Return diff panel to split view"
                        : "Expand diff panel to fullscreen"
                    }
                    type="button"
                    onClick={onToggleFullscreen}
                  >
                    {fullscreen ? (
                      <Minimize2
                        absoluteStrokeWidth
                        aria-hidden="true"
                        className="diff-panel__control-icon diff-panel__control-icon--fullscreen"
                        size={16}
                        strokeWidth={1.8}
                      />
                    ) : (
                      <Maximize2
                        absoluteStrokeWidth
                        aria-hidden="true"
                        className="diff-panel__control-icon diff-panel__control-icon--fullscreen"
                        size={16}
                        strokeWidth={1.8}
                      />
                    )}
                  </button>
                </>
              ) : null}

              <button
                aria-label="Close panel"
                className="ghost-button diff-panel__control"
                title="Close panel"
                type="button"
                onClick={onClose}
              >
                <X
                  absoluteStrokeWidth
                  aria-hidden="true"
                  className="diff-panel__control-icon"
                  size={16}
                  strokeWidth={1.85}
                />
              </button>
            </div>
          </div>

          {tab === "diff" ? (
            <DiffPanel
              bodyState={diffBodyState}
              onToggleDiffTarget={onToggleDiffTarget}
            />
          ) : tab === "files" ? (
            filesBody
          ) : (
            <PeerPanel
              clearingPeerMessages={clearingPeerMessages}
              deletingPeerMessageId={deletingPeerMessageId}
              livePeers={livePeers}
              onClearPeerMessages={onClearPeerMessages}
              onDeletePeerMessage={onDeletePeerMessage}
              recentMessages={recentMessages}
            />
          )}
        </div>
      ) : null}
    </aside>
  );
}
