import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import {
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  FileText,
  Folder,
  LoaderCircle,
} from "lucide-react";

import type { FileDirectoryState } from "../../hooks/filesPanelTypes";

export type FilesPanelHandle = {
  focusPrimaryAction: () => void;
};

type TreeDirectoryProps = {
  activeFilePath: string | null;
  copiedFilePath: string | null;
  depth: number;
  directoryStates: Record<string, FileDirectoryState>;
  entries: Array<{
    expandable: boolean;
    kind: "directory" | "file";
    name: string;
    path: string;
  }>;
  expandedPaths: Set<string>;
  onCopyFilePath: (path: string) => void;
  onLoadDirectory: (path: string) => void;
  onOpenFile: (path: string) => void;
  onToggleDirectory: (path: string) => void;
  openingFilePath: string | null;
};

type FilesPanelProps = {
  activeFilePath: string | null;
  activeWorktreeId: number | null;
  directoryStates: Record<string, FileDirectoryState>;
  expandedDirectoryPaths: string[];
  onLoadDirectory: (path: string) => void;
  onOpenFile: (path: string) => void;
  onToggleDirectory: (path: string) => void;
  openingFilePath: string | null;
  rootDirectoryState: FileDirectoryState;
};

function TreeDirectory(props: TreeDirectoryProps) {
  const {
    activeFilePath,
    copiedFilePath,
    depth,
    directoryStates,
    entries,
    expandedPaths,
    onCopyFilePath,
    onLoadDirectory,
    onOpenFile,
    onToggleDirectory,
    openingFilePath,
  } = props;

  return (
    <div className="files-tree">
      {entries.map((entry) => {
        const isDirectory = entry.kind === "directory";
        const isExpanded = isDirectory && expandedPaths.has(entry.path);
        const isOpening = !isDirectory && openingFilePath === entry.path;
        const isCopied = !isDirectory && copiedFilePath === entry.path;
        const childState = isDirectory
          ? directoryStates[entry.path] ?? null
          : null;

        return (
          <div className="files-tree__node" key={entry.path}>
            <div
              className={`files-tree__row-wrap${activeFilePath === entry.path ? " is-active" : ""}`}
            >
              <button
                aria-busy={isOpening}
                className={`files-tree__row${activeFilePath === entry.path ? " is-active" : ""}${isOpening ? " is-pending" : ""}`}
                style={{ paddingLeft: `${depth * 14 + 10}px` }}
                type="button"
                onClick={() => {
                  if (isDirectory) {
                    onToggleDirectory(entry.path);
                    return;
                  }
                  onOpenFile(entry.path);
                }}
              >
                <span className="files-tree__icon">
                  {isDirectory ? (
                    isExpanded ? (
                      <ChevronDown aria-hidden="true" size={13} strokeWidth={1.8} />
                    ) : (
                      <ChevronRight aria-hidden="true" size={13} strokeWidth={1.8} />
                    )
                  ) : (
                    <FileText aria-hidden="true" size={13} strokeWidth={1.8} />
                  )}
                </span>
                {isDirectory ? (
                  <Folder aria-hidden="true" className="files-tree__glyph" size={14} />
                ) : null}
                <span className="files-tree__label">{entry.name}</span>
                {isOpening ? (
                  <LoaderCircle
                    aria-hidden="true"
                    className="files-tree__spinner"
                    size={12}
                    strokeWidth={1.9}
                  />
                ) : null}
              </button>
              {!isDirectory ? (
                <button
                  aria-label={`Copy path of ${entry.path}`}
                  className="files-tree__copy"
                  title={isCopied ? "Copied" : "Copy file path"}
                  type="button"
                  onClick={(event) => {
                    event.stopPropagation();
                    onCopyFilePath(entry.path);
                  }}
                >
                  {isCopied ? (
                    <Check aria-hidden="true" size={13} strokeWidth={2} />
                  ) : (
                    <Copy aria-hidden="true" size={13} strokeWidth={2} />
                  )}
                </button>
              ) : null}
            </div>

            {isDirectory && isExpanded ? (
              <div className="files-tree__children">
                {childState?.loading && !childState.loaded ? (
                  <div
                    className="files-tree__status"
                    style={{ paddingLeft: `${depth * 14 + 34}px` }}
                  >
                    Loading...
                  </div>
                ) : childState?.error ? (
                  <div
                    className="files-tree__status files-tree__status--error"
                    style={{ paddingLeft: `${depth * 14 + 34}px` }}
                  >
                    <span>{childState.error}</span>
                    <button
                      className="ghost-button files-tree__retry"
                      type="button"
                      onClick={() => onLoadDirectory(entry.path)}
                    >
                      Retry
                    </button>
                  </div>
                ) : childState && childState.items.length > 0 ? (
                  <TreeDirectory
                    activeFilePath={activeFilePath}
                    copiedFilePath={copiedFilePath}
                    depth={depth + 1}
                    directoryStates={directoryStates}
                    entries={childState.items}
                    expandedPaths={expandedPaths}
                    onCopyFilePath={onCopyFilePath}
                    onLoadDirectory={onLoadDirectory}
                    onOpenFile={onOpenFile}
                    onToggleDirectory={onToggleDirectory}
                    openingFilePath={openingFilePath}
                  />
                ) : (
                  <div
                    className="files-tree__status"
                    style={{ paddingLeft: `${depth * 14 + 34}px` }}
                  >
                    Empty
                  </div>
                )}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

export const FilesPanel = forwardRef<FilesPanelHandle, FilesPanelProps>(
  function FilesPanel(props, ref) {
    const {
      activeFilePath,
      activeWorktreeId,
      directoryStates,
      expandedDirectoryPaths,
      onLoadDirectory,
      onOpenFile,
      onToggleDirectory,
      openingFilePath,
      rootDirectoryState,
    } = props;

    const rootRef = useRef<HTMLDivElement | null>(null);
    const expandedPaths = new Set(expandedDirectoryPaths);
    const [copiedFilePath, setCopiedFilePath] = useState<string | null>(null);
    const copyResetTimerRef = useRef<number | null>(null);

    const copyFilePath = (relativePath: string) => {
      const writeToClipboard =
        typeof navigator !== "undefined" && navigator.clipboard?.writeText
          ? navigator.clipboard.writeText.bind(navigator.clipboard)
          : null;

      const finish = (success: boolean) => {
        if (!success) {
          return;
        }
        setCopiedFilePath(relativePath);
        if (copyResetTimerRef.current !== null) {
          window.clearTimeout(copyResetTimerRef.current);
        }
        copyResetTimerRef.current = window.setTimeout(() => {
          setCopiedFilePath(null);
          copyResetTimerRef.current = null;
        }, 1500);
      };

      if (writeToClipboard) {
        writeToClipboard(relativePath).then(
          () => finish(true),
          () => finish(false),
        );
      } else {
        finish(false);
      }
    };

    useEffect(() => {
      return () => {
        if (copyResetTimerRef.current !== null) {
          window.clearTimeout(copyResetTimerRef.current);
        }
      };
    }, []);

    useImperativeHandle(ref, () => ({
      focusPrimaryAction() {
        const root = rootRef.current;
        if (!root) {
          return;
        }

        const focusTarget =
          root.querySelector<HTMLButtonElement>(".files-tree__row.is-active") ??
          root.querySelector<HTMLButtonElement>(".files-tree__row") ??
          root.querySelector<HTMLButtonElement>(".files-tree__retry");

        if (focusTarget) {
          focusTarget.focus();
          return;
        }

        root.focus();
      },
    }), []);

    if (!activeWorktreeId) {
      return (
        <div
          ref={rootRef}
          className="diff-panel__empty"
          tabIndex={-1}
        >
          Open a session to browse files for that worktree.
        </div>
      );
    }

    if (rootDirectoryState.loading && !rootDirectoryState.loaded) {
      return (
        <div
          ref={rootRef}
          className="diff-panel__empty"
          tabIndex={-1}
        >
          Loading files...
        </div>
      );
    }

    if (rootDirectoryState.error) {
      return (
        <div
          ref={rootRef}
          className="files-panel__message files-panel__message--error"
          tabIndex={-1}
        >
          <span>{rootDirectoryState.error}</span>
          <button
            className="ghost-button files-tree__retry"
            type="button"
            onClick={() => {
              void onLoadDirectory(".");
            }}
          >
            Retry
          </button>
        </div>
      );
    }

    if (rootDirectoryState.items.length === 0) {
      return (
        <div
          ref={rootRef}
          className="diff-panel__empty"
          tabIndex={-1}
        >
          This worktree is empty.
        </div>
      );
    }

    return (
      <div
        ref={rootRef}
        className="files-panel files-panel--browser"
        tabIndex={-1}
      >
        <TreeDirectory
          activeFilePath={activeFilePath}
          copiedFilePath={copiedFilePath}
          depth={0}
          directoryStates={directoryStates}
          entries={rootDirectoryState.items}
          expandedPaths={expandedPaths}
          onCopyFilePath={copyFilePath}
          onLoadDirectory={onLoadDirectory}
          onOpenFile={onOpenFile}
          onToggleDirectory={onToggleDirectory}
          openingFilePath={openingFilePath}
        />
      </div>
    );
  },
);

FilesPanel.displayName = "FilesPanel";
