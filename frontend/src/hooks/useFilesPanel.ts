import { useCallback } from "react";

import { useActiveFileEditor } from "./useActiveFileEditor";
import type {
  ActiveEditorFile,
  FileDirectoryState,
  FileNavigationTarget,
  FileOpenOptions,
} from "./filesPanelTypes";
import { useFileBrowserTree } from "./useFileBrowserTree";

export type { ActiveEditorFile, FileDirectoryState };

type UseFilesPanelOptions = {
  activeWorktreeId: number | null;
  confirmDiscardPrompt: () => Promise<boolean>;
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

  const confirmDiscardChanges = useCallback(async () => {
    if (!activeFileEditor.hasDirtyChanges) {
      return true;
    }

    const shouldDiscard = await confirmDiscardPrompt();
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
    async (
      path: string,
      target?: FileNavigationTarget,
      options?: FileOpenOptions,
    ) => {
      const needsNavigationConfirm =
        activeFileEditor.activeFile?.path !== path || options?.forceReload;
      if (needsNavigationConfirm && !(await confirmDiscardChanges())) {
        return;
      }
      await activeFileEditor.openFile(path, target, options);
    },
    [
      activeFileEditor.activeFile?.path,
      activeFileEditor.openFile,
      confirmDiscardChanges,
    ],
  );

  const closeEditor = useCallback(async () => {
    if (!(await confirmDiscardChanges())) {
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
