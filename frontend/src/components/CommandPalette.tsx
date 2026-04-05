import { File, Search } from "lucide-react";
import { useCallback, useEffect, useRef } from "react";


import type { RepoDTO } from "../types";
import type { MenuAction } from "../utils/appShell";
import {
  useCommandPalette,
  type PaletteItem,
} from "../hooks/useCommandPalette";
import { AgentIcon } from "./icons";

interface CommandPaletteProps {
  open: boolean;
  repos: RepoDTO[];
  sessionPaths: Record<number, string>;
  activeSessionId: number;
  activeWorktreeId: number | null;
  onActivateSession: (sessionId: number) => void;
  onOpenFile: (path: string) => void;
  onAction: (action: MenuAction) => void;
  onClose: () => void;
}

export function CommandPalette(props: CommandPaletteProps) {
  const {
    open,
    repos,
    sessionPaths,
    activeSessionId,
    activeWorktreeId,
    onActivateSession,
    onOpenFile,
    onAction,
    onClose,
  } = props;

  const palette = useCommandPalette({
    open,
    repos,
    sessionPaths,
    activeSessionId,
    activeWorktreeId,
  });

  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const container = listRef.current;
    if (!container) return;
    const selectedEl = container.children[palette.selectedIndex] as
      | HTMLElement
      | undefined;
    selectedEl?.scrollIntoView({ block: "nearest" });
  }, [open, palette.selectedIndex]);

  const handleSelect = useCallback(
    (item: PaletteItem) => {
      if (item.kind === "session") {
        onActivateSession(item.sessionId);
      } else if (item.kind === "file") {
        onOpenFile(item.path);
      } else {
        onAction(item.action);
      }
    },
    [onActivateSession, onOpenFile, onAction],
  );

  const handleKeyDown = (event: React.KeyboardEvent) => {
    palette.handleKeyDown(event);
    if (event.key === "Enter") {
      const selectedItem = palette.filteredItems[palette.selectedIndex];
      if (selectedItem) {
        event.preventDefault();
        handleSelect(selectedItem);
      }
    }
  };

  if (!open) return null;

  const placeholder =
    palette.mode === "actions"
      ? "Search actions\u2026"
      : palette.mode === "files"
        ? "Search files\u2026"
        : "Search sessions\u2026";

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="command-palette"
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="command-palette__search">
          <Search
            aria-hidden="true"
            className="command-palette__search-icon"
            size={16}
            strokeWidth={1.8}
          />
          <input
            ref={palette.inputRef}
            className="command-palette__input"
            placeholder={placeholder}
            type="text"
            value={palette.query}
            onChange={(e) => palette.setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </div>

        {palette.mode === "sessions" && !palette.query ? (
          <div className="command-palette__hint">
            Type <kbd>&gt;</kbd> for actions or <kbd>/</kbd> for files
          </div>
        ) : null}

        <div className="command-palette__list" ref={listRef}>
          {palette.mode === "files" && palette.filesLoading ? (
            <div className="command-palette__empty">Loading files\u2026</div>
          ) : palette.filteredItems.length === 0 ? (
            <div className="command-palette__empty">No results</div>
          ) : (
            palette.filteredItems.map((item, index) => (
              <button
                key={item.id}
                className={`command-palette__item${index === palette.selectedIndex ? " is-selected" : ""}`}
                type="button"
                onPointerEnter={() => palette.setSelectedIndex(index)}
                onPointerDown={(event) => {
                  event.preventDefault();
                  handleSelect(item);
                }}
              >
                {item.kind === "session" ? (
                  <>
                    <AgentIcon
                      agentId={item.adapterId}
                      className="command-palette__item-icon"
                      size={14}
                    />
                    <span className="command-palette__item-label">
                      {item.label}
                    </span>
                    <span className="command-palette__item-detail">
                      {item.detail}
                    </span>
                    {item.isActive ? (
                      <span className="command-palette__active-badge">
                        active
                      </span>
                    ) : null}
                  </>
                ) : item.kind === "file" ? (
                  <>
                    <File
                      aria-hidden="true"
                      className="command-palette__item-icon"
                      size={14}
                      strokeWidth={1.6}
                    />
                    <span className="command-palette__item-label">
                      {item.filename}
                    </span>
                    <span className="command-palette__item-detail">
                      {item.path}
                    </span>
                  </>
                ) : (
                  <>
                    <span className="command-palette__item-label">
                      {item.label}
                    </span>
                    <kbd className="command-palette__shortcut">
                      {item.shortcut}
                    </kbd>
                  </>
                )}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
