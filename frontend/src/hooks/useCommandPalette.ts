import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { listWorktreeFiles, searchWorktreeContents } from "../backend";
import type { RepoDTO, WorktreeContentMatch } from "../types";
import {
  describeSessionLabel,
  describeWorktreeMeta,
  repoVisibleSessions,
  type MenuAction,
} from "../utils/appShell";

/* ── Item types ──────────────────────────────────────────────── */

export interface PaletteSessionItem {
  kind: "session";
  id: string;
  sessionId: number;
  label: string;
  detail: string;
  isActive: boolean;
  adapterId: string;
}

export interface PaletteActionItem {
  kind: "action";
  id: string;
  action: MenuAction;
  label: string;
  shortcut: string;
}

export interface PaletteFileItem {
  kind: "file";
  id: string;
  path: string;
  filename: string;
}

export interface PaletteContentItem {
  kind: "content";
  id: string;
  path: string;
  filename: string;
  line: number;
  column: number;
  preview: string;
}

export type PaletteItem =
  | PaletteSessionItem
  | PaletteActionItem
  | PaletteFileItem
  | PaletteContentItem;

/* ── Action registry ─────────────────────────────────────────── */

const ACTION_REGISTRY: PaletteActionItem[] = [
  { kind: "action", id: "a:new-workspace", action: "new-workspace", label: "New Workspace", shortcut: "\u2318O" },
  { kind: "action", id: "a:new-session", action: "new-session", label: "New Session", shortcut: "\u2318T" },
  { kind: "action", id: "a:close-session", action: "close-session", label: "Close Session", shortcut: "\u2318W" },
  { kind: "action", id: "a:save-file-editor", action: "save-file-editor", label: "Save File", shortcut: "\u2318S" },
  { kind: "action", id: "a:toggle-sidebar", action: "toggle-sidebar", label: "Toggle Sidebar", shortcut: "\u2318B" },
  { kind: "action", id: "a:toggle-diff", action: "toggle-diff", label: "Toggle Diff Panel", shortcut: "\u21E7\u2318D" },
  { kind: "action", id: "a:toggle-files", action: "toggle-files", label: "Toggle Files Panel", shortcut: "\u21E7\u2318E" },
  { kind: "action", id: "a:toggle-shell", action: "toggle-shell", label: "Toggle Shell Panel", shortcut: "\u21E7\u2318S" },
  { kind: "action", id: "a:toggle-peers", action: "toggle-peers", label: "Toggle Peers Panel", shortcut: "\u21E7\u2318P" },
  { kind: "action", id: "a:toggle-diff-fullscreen", action: "toggle-diff-fullscreen", label: "Toggle Diff Fullscreen", shortcut: "\u21E7\u2318F" },
  { kind: "action", id: "a:focus-terminal", action: "focus-terminal", label: "Focus Terminal", shortcut: "\u23181" },
  { kind: "action", id: "a:focus-files-panel", action: "focus-files-panel", label: "Focus Files Panel", shortcut: "\u23182" },
  { kind: "action", id: "a:zoom-out-terminal", action: "zoom-out-terminal", label: "Zoom Out Terminal Text", shortcut: "\u2318-" },
  { kind: "action", id: "a:reset-terminal-zoom", action: "reset-terminal-zoom", label: "Reset Terminal Text Zoom", shortcut: "\u23180" },
  { kind: "action", id: "a:zoom-in-terminal", action: "zoom-in-terminal", label: "Zoom In Terminal Text", shortcut: "\u2318+" },
  { kind: "action", id: "a:refresh-diff", action: "refresh-diff", label: "Refresh Diff", shortcut: "\u2325\u2318R" },
];

/* ── Helpers ──────────────────────────────────────────────────── */

function substringMatch(query: string, text: string): boolean {
  if (!query) return true;
  return text.toLowerCase().includes(query.toLowerCase());
}

function fileNameFromPath(path: string): string {
  const lastSlash = path.lastIndexOf("/");
  return lastSlash >= 0 ? path.slice(lastSlash + 1) : path;
}

const fileListRefreshIntervalMs = 2000;

/* ── Hook ─────────────────────────────────────────────────────── */

export type PaletteMode = "sessions" | "actions" | "files";

interface UseCommandPaletteOptions {
  open: boolean;
  repos: RepoDTO[];
  sessionPaths: Record<number, string>;
  activeSessionId: number;
  activeWorktreeId: number | null;
}

export function useCommandPalette(options: UseCommandPaletteOptions) {
  const { open, repos, sessionPaths, activeSessionId, activeWorktreeId } = options;

  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // File list state (loaded async when entering file mode)
  const [fileList, setFileList] = useState<string[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const filesCacheRef = useRef<{ worktreeId: number; files: string[] } | null>(null);
  const fileListRequestIdRef = useRef(0);
  const [contentMatches, setContentMatches] = useState<WorktreeContentMatch[]>([]);
  const [contentLoading, setContentLoading] = useState(false);
  const contentRequestIdRef = useRef(0);

  // Reset state when palette opens
  useEffect(() => {
    if (open) {
      setQuery("");
      setSelectedIndex(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Derive mode from query prefix
  const mode: PaletteMode = query.startsWith(">")
    ? "actions"
    : query.startsWith("/")
      ? "files"
      : "sessions";

  const searchQuery =
    mode === "sessions" ? query.trim() : query.slice(1).trim();

  // Load file list when entering file mode
  useEffect(() => {
    if (!open || mode !== "files" || !activeWorktreeId) {
      if (!activeWorktreeId) {
        setFileList([]);
      }
      setFilesLoading(false);
      return;
    }

    let cancelled = false;
    let requestInFlight = false;
    const requestId = fileListRequestIdRef.current + 1;
    fileListRequestIdRef.current = requestId;

    const cachedFiles =
      filesCacheRef.current?.worktreeId === activeWorktreeId
        ? filesCacheRef.current.files
        : null;

    if (cachedFiles) {
      setFileList(cachedFiles);
      setFilesLoading(false);
    } else {
      setFileList([]);
      setFilesLoading(true);
    }

    const refreshFileList = (showLoading: boolean) => {
      if (cancelled || requestInFlight) {
        return;
      }

      requestInFlight = true;
      if (showLoading) {
        setFilesLoading(true);
      }

      listWorktreeFiles(activeWorktreeId).then(
        (files) => {
          if (cancelled || fileListRequestIdRef.current !== requestId) {
            return;
          }
          filesCacheRef.current = { worktreeId: activeWorktreeId, files };
          setFileList(files);
          setFilesLoading(false);
        },
        () => {
          if (cancelled || fileListRequestIdRef.current !== requestId) {
            return;
          }
          if (filesCacheRef.current?.worktreeId !== activeWorktreeId) {
            setFileList([]);
          }
          setFilesLoading(false);
        },
      ).finally(() => {
        requestInFlight = false;
      });
    };

    refreshFileList(!cachedFiles);
    const intervalId = window.setInterval(() => {
      refreshFileList(false);
    }, fileListRefreshIntervalMs);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, [open, mode, activeWorktreeId]);

  useEffect(() => {
    if (!open || mode !== "files" || !activeWorktreeId || searchQuery.length < 2) {
      contentRequestIdRef.current += 1;
      setContentMatches([]);
      setContentLoading(false);
      return;
    }

    const requestId = contentRequestIdRef.current + 1;
    contentRequestIdRef.current = requestId;
    setContentMatches([]);
    setContentLoading(true);
    const timeoutId = window.setTimeout(() => {
      searchWorktreeContents(activeWorktreeId, searchQuery, 100).then(
        (matches) => {
          if (contentRequestIdRef.current !== requestId) {
            return;
          }
          setContentMatches(matches);
          setContentLoading(false);
        },
        () => {
          if (contentRequestIdRef.current !== requestId) {
            return;
          }
          setContentMatches([]);
          setContentLoading(false);
        },
      );
    }, 150);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [activeWorktreeId, mode, open, searchQuery]);

  // Build session items
  const sessionItems = useMemo((): PaletteSessionItem[] => {
    if (!open) return [];
    const items: PaletteSessionItem[] = [];
    for (const repo of repos) {
      for (const { worktree, session } of repoVisibleSessions(repo)) {
        const sessionLabel = describeSessionLabel(
          session,
          worktree,
          sessionPaths[session.id] ?? null,
        );
        const meta = describeWorktreeMeta(repo, worktree);
        items.push({
          kind: "session",
          id: `s:${session.id}`,
          sessionId: session.id,
          label: sessionLabel.label,
          detail: `${repo.name} \u2022 ${meta.label}`,
          isActive: session.id === activeSessionId,
          adapterId: session.adapterId,
        });
      }
    }
    return items;
  }, [open, repos, sessionPaths, activeSessionId]);

  // Build file items
  const fileItems = useMemo((): PaletteFileItem[] => {
    return fileList.map((path) => ({
      kind: "file" as const,
      id: `f:${path}`,
      path,
      filename: fileNameFromPath(path),
    }));
  }, [fileList]);

  const contentItems = useMemo((): PaletteContentItem[] => {
    return contentMatches.map((match) => ({
      kind: "content" as const,
      id: `c:${match.path}:${match.line}:${match.column}`,
      path: match.path,
      filename: fileNameFromPath(match.path),
      line: match.line,
      column: match.column,
      preview: match.preview,
    }));
  }, [contentMatches]);

  // Filter items based on mode and query
  const filteredItems = useMemo((): PaletteItem[] => {
    if (mode === "actions") {
      if (!searchQuery) return ACTION_REGISTRY;
      return ACTION_REGISTRY.filter((item) => substringMatch(searchQuery, item.label));
    }
    if (mode === "files") {
      const matchedFiles = !searchQuery
        ? fileItems
        : fileItems.filter((item) => substringMatch(searchQuery, item.path));
      const pathMatches = matchedFiles.slice(0, 200);
      if (searchQuery.length < 2) {
        return pathMatches;
      }
      return [...pathMatches, ...contentItems];
    }
    if (!searchQuery) return sessionItems;
    return sessionItems.filter(
      (item) => substringMatch(searchQuery, item.label) || substringMatch(searchQuery, item.detail),
    );
  }, [mode, searchQuery, sessionItems, fileItems, contentItems]);

  // Clamp selectedIndex when items change
  useEffect(() => {
    setSelectedIndex((prev) => Math.min(prev, Math.max(0, filteredItems.length - 1)));
  }, [filteredItems.length]);

  // Keyboard handler
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, filteredItems.length - 1));
          break;
        case "ArrowUp":
          event.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
      }
    },
    [filteredItems.length],
  );

  return {
    query,
    setQuery,
    selectedIndex,
    setSelectedIndex,
    filteredItems,
    contentLoading,
    filesLoading,
    handleKeyDown,
    inputRef,
    mode,
  };
}
