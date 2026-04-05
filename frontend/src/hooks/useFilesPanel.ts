import { useCallback } from "react";

import { useActiveFileEditor } from "./useActiveFileEditor";
import type {
  ActiveEditorFile,
  FileDirectoryState,
} from "./filesPanelTypes";
import { useFileBrowserTree } from "./useFileBrowserTree";

export type { ActiveEditorFile, FileDirectoryState };

type UseFilesPanelOptions = {
  activeWorktreeId: number | null;
  confirmDiscardPrompt: () => boolean;
  enabled: boolean;
};

export function useFilesPanel(options: UseFilesPanelOptions) {
  const { activeWorktreeId, confirmDiscardPrompt, enabled } = options;

  const fileBrowserTree = useFileBrowserTree({
    activeWorktreeId,
    enabled,
  });
  const activeFileEditor = useActiveFileEditor({
    activeWorktreeId,
  });

  const confirmDiscardChanges = useCallback(() => {
    if (!activeFileEditor.hasDirtyChanges) {
      return true;
    }

    const shouldDiscard = confirmDiscardPrompt();
    if (shouldDiscard) {
      activeFileEditor.discardUnsavedChanges();
    }
    return shouldDiscard;
  }, [
    activeFileEditor.discardUnsavedChanges,
    activeFileEditor.hasDirtyChanges,
    confirmDiscardPrompt,
  ]);

  const openFile = useCallback(
    async (path: string) => {
      if (!confirmDiscardChanges()) {
        return;
      }
      await activeFileEditor.openFile(path);
    },
    [activeFileEditor.openFile, confirmDiscardChanges],
  );

  const closeEditor = useCallback(() => {
    if (!confirmDiscardChanges()) {
      return false;
    }
    return activeFileEditor.closeEditor();
  }, [activeFileEditor.closeEditor, confirmDiscardChanges]);

  return {
    activeFile: activeFileEditor.activeFile,
    closeEditor,
    confirmDiscardChanges,
    directoryStates: fileBrowserTree.directoryStates,
    discardUnsavedChanges: activeFileEditor.discardUnsavedChanges,
    expandedDirectoryPaths: fileBrowserTree.expandedDirectoryPaths,
    hasDirtyChanges: activeFileEditor.hasDirtyChanges,
    loadDirectory: fileBrowserTree.loadDirectory,
    openingFilePath: activeFileEditor.openingFilePath,
    openFile,
    rootDirectoryState: fileBrowserTree.rootDirectoryState,
    saveActiveFile: activeFileEditor.saveActiveFile,
    setActiveFileDirty: activeFileEditor.setActiveFileDirty,
    toggleDirectory: fileBrowserTree.toggleDirectory,
  };
}
