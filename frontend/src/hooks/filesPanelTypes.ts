import type { WorktreeEntry } from "../types";

export type FileDirectoryState = {
  error: string | null;
  items: WorktreeEntry[];
  loaded: boolean;
  loading: boolean;
};

export type FileNavigationTarget = {
  column: number;
  line: number;
};

export type FileOpenOptions = {
  forceReload: boolean;
};

export type ActiveEditorFile = {
  dirty: boolean;
  editorSync: {
    reason: "discard" | "navigate" | "open" | "save";
    strategy: "rebase-document" | "replace-document" | "reveal-location";
    target: FileNavigationTarget | null;
    token: number;
  };
  error: string | null;
  loading: boolean;
  path: string;
  savedContent: string;
  saving: boolean;
  versionToken: string;
};
