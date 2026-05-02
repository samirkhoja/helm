import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import {
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  CopyPlus,
  FileText,
  Folder,
  LoaderCircle,
} from "lucide-react";

import type { FileDirectoryState } from "../../hooks/filesPanelTypes";

export type FilesPanelHandle = {
  focusPrimaryAction: () => void;
};

type CopyKind = "relative" | "absolute";

type TreeDirectoryProps = {
  activeFilePath: string | null;
  copiedFilePath: { path: string; kind: CopyKind } | null;
  depth: number;
  directoryStates: Record<string, FileDirectoryState>;
  entries: Array<{
    expandable: boolean;
    kind: "directory" | "file";
    name: string;
    path: string;
  }>;
  expandedPaths: Set<string>;
  onCopyFilePath: (path: string, kind: CopyKind) => void;
  onLoadDirectory: (path: string) => void;
  onOpenFile: (path: string) => void;
  onToggleDirectory: (path: string) => void;
  openingFilePath: string | null;
  worktreeRootPath: string | null;
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
  worktreeRootPath: string | null;
};

function joinAbsolutePath(rootPath: string, relativePath: string): string {
  const trimmedRoot = rootPath.replace(/\/+$/, "");
  const trimmedRelative = relativePath.replace(/^\/+/, "");
  if (!trimmedRelative) {
    return trimmedRoot;
  }
  return `${trimmedRoot}/${trimmedRelative}`;
}

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
    worktreeRootPath,
  } = props;

  return (
    <div className="files-tree">
      {entries.map((entry) => {
        const isDirectory = entry.kind === "directory";
        const isExpanded = isDirectory && expandedPaths.has(entry.path);
        const isOpening = !isDirectory && openingFilePath === entry.path;
        const copiedKind =
          !isDirectory && copiedFilePath?.path === entry.path
            ? copiedFilePath.kind
            : null;
        const childState = isDirectory
          ? directoryStates[entry.path] ?? null
          : null;
        const absolutePath = worktreeRootPath
          ? joinAbsolutePath(worktreeRootPath, entry.path)
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
                <>
                  <button
                    aria-label={`Copy relative path of ${entry.path}`}
                    className="files-tree__copy"
                    title={
                      copiedKind === "relative" ? "Copied" : "Copy relative path"
                    }
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      onCopyFilePath(entry.path, "relative");
                    }}
                  >
                    {copiedKind === "relative" ? (
                      <Check aria-hidden="true" size={13} strokeWidth={2} />
                    ) : (
                      <Copy aria-hidden="true" size={13} strokeWidth={2} />
                    )}
                  </button>
                  {absolutePath ? (
                    <button
                      aria-label={`Copy full path of ${entry.path}`}
                      className="files-tree__copy"
                      title={
                        copiedKind === "absolute" ? "Copied" : "Copy full path"
                      }
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        onCopyFilePath(entry.path, "absolute");
                      }}
                    >
                      {copiedKind === "absolute" ? (
                        <Check aria-hidden="true" size={13} strokeWidth={2} />
                      ) : (
                        <CopyPlus aria-hidden="true" size={13} strokeWidth={2} />
                      )}
                    </button>
                  ) : null}
                </>
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
                    worktreeRootPath={worktreeRootPath}
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
      worktreeRootPath,
    } = props;

    const rootRef = useRef<HTMLDivElement | null>(null);
    const expandedPaths = new Set(expandedDirectoryPaths);
    const [copiedFilePath, setCopiedFilePath] = useState<{
      path: string;
      kind: CopyKind;
    } | null>(null);
    const copyResetTimerRef = useRef<number | null>(null);

    const copyFilePath = (relativePath: string, kind: CopyKind) => {
      const value =
        kind === "absolute" && worktreeRootPath
          ? joinAbsolutePath(worktreeRootPath, relativePath)
          : relativePath;
      const writeToClipboard =
        typeof navigator !== "undefined" && navigator.clipboard?.writeText
          ? navigator.clipboard.writeText.bind(navigator.clipboard)
          : null;

      const finish = (success: boolean) => {
        if (!success) {
          return;
        }
        setCopiedFilePath({ path: relativePath, kind });
        if (copyResetTimerRef.current !== null) {
          window.clearTimeout(copyResetTimerRef.current);
        }
        copyResetTimerRef.current = window.setTimeout(() => {
          setCopiedFilePath(null);
          copyResetTimerRef.current = null;
        }, 1500);
      };

      if (writeToClipboard) {
        writeToClipboard(value).then(
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
          worktreeRootPath={worktreeRootPath}
        />
      </div>
    );
  },
);

FilesPanel.displayName = "FilesPanel";
