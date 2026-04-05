import type { WorktreeEntry } from "../types";

export type FileDirectoryState = {
  error: string | null;
  items: WorktreeEntry[];
  loaded: boolean;
  loading: boolean;
};

export type ActiveEditorFile = {
  dirty: boolean;
  editorSync: {
    reason: "discard" | "open" | "save";
    strategy: "rebase-document" | "replace-document";
    token: number;
  };
  error: string | null;
  loading: boolean;
  path: string;
  savedContent: string;
  saving: boolean;
  versionToken: string;
};
