import { ArrowLeft, ChevronRight, Save } from "lucide-react";

import { AgentIcon, DiffPanelIcon, FilesPanelIcon, PeersIcon } from "../icons";

type MainHeaderProps = {
  activeAgentId: string | null;
  activeSessionLabel: string | null;
  diffPanelActive: boolean;
  filesPanelActive: boolean;
  fileEditorDirty: boolean;
  fileEditorOpen: boolean;
  fileEditorSaving: boolean;
  localOutstandingPeerCount: number;
  onCloseFileEditor: () => void;
  onSaveFileEditor: () => void;
  onToggleDiff: () => void;
  onToggleFiles: () => void;
  onTogglePeers: () => void;
  onToggleSidebar: () => void;
  peersPanelActive: boolean;
  sidebarOpen: boolean;
  subtitle: string;
  title: string;
};

export function MainHeader(props: MainHeaderProps) {
  const {
    activeAgentId,
    activeSessionLabel,
    diffPanelActive,
    fileEditorDirty,
    fileEditorOpen,
    fileEditorSaving,
    filesPanelActive,
    localOutstandingPeerCount,
    onCloseFileEditor,
    onSaveFileEditor,
    onToggleDiff,
    onToggleFiles,
    onTogglePeers,
    onToggleSidebar,
    peersPanelActive,
    sidebarOpen,
    subtitle,
    title,
  } = props;
  const returnToSessionLabel = activeSessionLabel ?? "Back";
  const returnToTerminalLabel = activeSessionLabel
    ? `Return to ${activeSessionLabel}`
    : "Return";

  return (
    <header className="main-header">
      <div className="main-header__title">
        {!sidebarOpen ? (
          <button
            aria-label="Expand sidebar"
            className="icon-button main-header__sidebar-toggle"
            type="button"
            onClick={onToggleSidebar}
          >
            <ChevronRight aria-hidden="true" size={15} strokeWidth={1.8} />
          </button>
        ) : null}
        <div className="main-header__meta">
          <h1>
            {fileEditorOpen ? (
              <FilesPanelIcon
                className="main-header__session-icon"
                size={14}
              />
            ) : activeAgentId ? (
              <AgentIcon
                agentId={activeAgentId}
                className="main-header__session-icon"
                size={14}
              />
            ) : null}
            <span>{title}</span>
          </h1>
          <p>{subtitle}</p>
        </div>
      </div>

      <div className="main-header__actions">
        {fileEditorOpen ? (
          <>
            <button
              aria-label={returnToTerminalLabel}
              className="ghost-button main-header__editor-action main-header__editor-action--session"
              title={returnToTerminalLabel}
              type="button"
              onClick={onCloseFileEditor}
            >
              <ArrowLeft aria-hidden="true" size={14} strokeWidth={1.85} />
              {activeAgentId ? (
                <AgentIcon
                  agentId={activeAgentId}
                  className="main-header__session-icon"
                  size={14}
                />
              ) : null}
              <span className="main-header__editor-action-label">
                {returnToSessionLabel}
              </span>
            </button>

            <button
              aria-label="Save file"
              aria-busy={fileEditorSaving}
              className={`ghost-button main-header__editor-action main-header__editor-action--save${fileEditorDirty && !fileEditorSaving ? " is-active" : ""}`}
              disabled={!fileEditorDirty || fileEditorSaving}
              title="Save file"
              type="button"
              onClick={onSaveFileEditor}
            >
              <Save aria-hidden="true" size={14} strokeWidth={1.85} />
              <span>Save</span>
            </button>
          </>
        ) : null}

        <button
          aria-label={peersPanelActive ? "Hide peers panel" : "Show peers panel"}
          className={`ghost-button main-header__utility-toggle main-header__utility-toggle--icon main-header__utility-toggle--badgeable${peersPanelActive ? " is-active" : ""}`}
          title={peersPanelActive ? "Hide peers panel" : "Show peers panel"}
          type="button"
          onClick={onTogglePeers}
        >
          <PeersIcon className="utility-toggle__icon" size={14} />
          {localOutstandingPeerCount > 0 ? (
            <span className="utility-toggle__badge utility-toggle__badge--corner">
              {localOutstandingPeerCount}
            </span>
          ) : null}
        </button>

        <button
          aria-label={filesPanelActive ? "Hide files panel" : "Show files panel"}
          className={`ghost-button main-header__utility-toggle main-header__utility-toggle--icon${filesPanelActive ? " is-active" : ""}`}
          title={filesPanelActive ? "Hide files panel" : "Show files panel"}
          type="button"
          onClick={onToggleFiles}
        >
          <FilesPanelIcon className="utility-toggle__icon" size={15} />
        </button>

        <button
          aria-label={diffPanelActive ? "Hide git diff" : "Show git diff"}
          className={`ghost-button main-header__utility-toggle main-header__utility-toggle--icon${diffPanelActive ? " is-active" : ""}`}
          title={diffPanelActive ? "Hide git diff" : "Show git diff"}
          type="button"
          onClick={onToggleDiff}
        >
          <DiffPanelIcon className="utility-toggle__icon" size={15} />
        </button>
      </div>
    </header>
  );
}
