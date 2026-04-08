import { useEffect, useRef, useState } from "react";

import { readWorktreeFile, saveWorktreeFile } from "../backend";
import type {
  ActiveEditorFile,
  FileNavigationTarget,
  FileOpenOptions,
} from "./filesPanelTypes";

function nextEditorSync(
  current: ActiveEditorFile | null,
  request: Omit<ActiveEditorFile["editorSync"], "token">,
): ActiveEditorFile["editorSync"] {
  return {
    ...request,
    token: (current?.editorSync.token ?? 0) + 1,
  };
}

type UseActiveFileEditorOptions = {
  activeWorktreeId: number | null;
};

export function useActiveFileEditor(options: UseActiveFileEditorOptions) {
  const { activeWorktreeId } = options;

  const [activeFile, setActiveFile] = useState<ActiveEditorFile | null>(null);
  const [openingFilePath, setOpeningFilePath] = useState<string | null>(null);

  const currentWorktreeIdRef = useRef<number | null>(activeWorktreeId);
  const fileRequestIdRef = useRef(0);

  useEffect(() => {
    if (currentWorktreeIdRef.current === activeWorktreeId) {
      return;
    }

    currentWorktreeIdRef.current = activeWorktreeId;
    fileRequestIdRef.current += 1;
    setActiveFile(null);
    setOpeningFilePath(null);
  }, [activeWorktreeId]);

  const hasDirtyChanges = Boolean(activeFile?.dirty);

  const discardUnsavedChanges = () => {
    setActiveFile((current) => {
      if (!current) {
        return current;
      }
      return {
        ...current,
        dirty: false,
        editorSync: nextEditorSync(current, {
          reason: "discard",
          strategy: "replace-document",
          target: null,
        }),
        error: null,
      };
    });
  };

  const openFile = async (
    path: string,
    target?: FileNavigationTarget,
    options?: FileOpenOptions,
  ) => {
    if (!activeWorktreeId) {
      return;
    }

    const forceReload = options?.forceReload ?? false;
    if (activeFile?.path === path && !activeFile.error && !forceReload) {
      if (!target) {
        return;
      }

      setActiveFile((current) => {
        if (!current || current.path !== path) {
          return current;
        }
        return {
          ...current,
          editorSync: nextEditorSync(current, {
            reason: "navigate",
            strategy: "reveal-location",
            target,
          }),
          error: null,
        };
      });
      return;
    }

    if (openingFilePath === path) {
      return;
    }

    const worktreeId = activeWorktreeId;
    const requestId = fileRequestIdRef.current + 1;
    fileRequestIdRef.current = requestId;
    if (activeFile) {
      setActiveFile((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          error: null,
          loading: true,
        };
      });
    }
    setOpeningFilePath(path);

    try {
      const file = await readWorktreeFile(worktreeId, path);
      if (
        currentWorktreeIdRef.current !== worktreeId ||
        fileRequestIdRef.current !== requestId
      ) {
        return;
      }

      setOpeningFilePath(null);
      setActiveFile({
        dirty: false,
        editorSync: nextEditorSync(null, {
          reason: "open",
          strategy: "replace-document",
          target: target ?? null,
        }),
        error: null,
        loading: false,
        path: file.path,
        savedContent: file.content,
        saving: false,
        versionToken: file.versionToken,
      });
    } catch (error) {
      if (
        currentWorktreeIdRef.current !== worktreeId ||
        fileRequestIdRef.current !== requestId
      ) {
        return;
      }

      setOpeningFilePath(null);
      const errorMessage = `Unable to open ${path}: ${String(error)}`;
      setActiveFile((current) => {
        if (!current) {
          return {
            dirty: false,
            editorSync: nextEditorSync(null, {
              reason: "open",
              strategy: "replace-document",
              target: null,
            }),
            error: errorMessage,
            loading: false,
            path,
            savedContent: "",
            saving: false,
            versionToken: "",
          };
        }
        return {
          ...current,
          error: errorMessage,
          loading: false,
        };
      });
    }
  };

  const closeEditor = () => {
    fileRequestIdRef.current += 1;
    setOpeningFilePath(null);
    setActiveFile(null);
    return true;
  };

  const setActiveFileDirty = (dirty: boolean) => {
    setActiveFile((current) => {
      if (!current || current.dirty === dirty) {
        return current;
      }
      return {
        ...current,
        dirty,
        error: null,
      };
    });
  };

  const saveActiveFile = async (contentOverride?: string) => {
    if (
      !activeFile ||
      !activeWorktreeId ||
      activeFile.loading ||
      activeFile.saving
    ) {
      return;
    }

    const worktreeId = activeWorktreeId;
    const path = activeFile.path;
    const content =
      contentOverride ??
      (activeFile.dirty ? null : activeFile.savedContent);
    if (content === null) {
      return;
    }
    const versionToken = activeFile.versionToken;

    setActiveFile((current) => {
      if (!current || current.path !== path) {
        return current;
      }
      return {
        ...current,
        error: null,
        saving: true,
      };
    });

    try {
      const savedFile = await saveWorktreeFile(
        worktreeId,
        path,
        content,
        versionToken,
      );
      if (currentWorktreeIdRef.current !== worktreeId) {
        return;
      }

      setActiveFile((current) => {
        if (!current || current.path !== path) {
          return current;
        }
        return {
          ...current,
          dirty: false,
          editorSync: nextEditorSync(current, {
            reason: "save",
            strategy: "rebase-document",
            target: null,
          }),
          error: null,
          loading: false,
          path: savedFile.path,
          savedContent: savedFile.content,
          saving: false,
          versionToken: savedFile.versionToken,
        };
      });
    } catch (error) {
      if (currentWorktreeIdRef.current !== worktreeId) {
        return;
      }

      setActiveFile((current) => {
        if (!current || current.path !== path) {
          return current;
        }
        return {
          ...current,
          error: String(error),
          saving: false,
        };
      });
    }
  };

  return {
    activeFile,
    closeEditor,
    discardUnsavedChanges,
    hasDirtyChanges,
    openingFilePath,
    openFile,
    saveActiveFile,
    setActiveFileDirty,
  };
}
