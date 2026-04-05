import { useCallback, useEffect, useMemo, useRef } from "react";
import type { RefObject } from "react";

import type { FileEditorHandle } from "../components/app-shell/file-editor/types";
import type { TerminalStageHandle } from "../components/TerminalStage";
import type { UtilityPanelTab } from "../utils/appShell";
import type { ActiveEditorFile } from "./filesPanelTypes";

type FilesPanelController = {
  activeFile: ActiveEditorFile | null;
  confirmDiscardChanges: () => boolean;
  closeEditor: () => boolean;
  saveActiveFile: (contentOverride?: string) => Promise<void>;
};

type PaneLayoutController = {
  closeUtilityPanel: () => void;
  diffPanelFullscreen: boolean;
  diffPanelOpen: boolean;
  setDiffPanelFullscreen: (next: boolean | ((current: boolean) => boolean)) => void;
  toggleDiffFullscreen: () => void;
  toggleUtilityPanel: (tab: UtilityPanelTab) => void;
  utilityPanelTab: UtilityPanelTab;
};

type UseFileEditorShellOptions = {
  activeRepoName: string | null;
  activeSessionLabel: string | null;
  activeSessionOpen: boolean;
  activeWorktreeName: string | null;
  activeWorktreeMetaFullLabel: string;
  activeWorktreeMetaLabel: string;
  filesPanel: FilesPanelController;
  paneLayout: PaneLayoutController;
  terminalRef: RefObject<TerminalStageHandle>;
};

function isLeavingFilesContext(
  diffPanelOpen: boolean,
  utilityPanelTab: UtilityPanelTab,
  nextTab: UtilityPanelTab | null,
) {
  if (!diffPanelOpen || utilityPanelTab !== "files") {
    return false;
  }
  if (nextTab === null) {
    return true;
  }
  return nextTab !== "files";
}

export function useFileEditorShell(options: UseFileEditorShellOptions) {
  const {
    activeRepoName,
    activeSessionLabel,
    activeSessionOpen,
    activeWorktreeName,
    activeWorktreeMetaFullLabel,
    activeWorktreeMetaLabel,
    filesPanel,
    paneLayout,
    terminalRef,
  } = options;

  const fileEditorRef = useRef<FileEditorHandle | null>(null);
  const previousActiveFilePathRef = useRef<string | null>(null);

  const activeFile = filesPanel.activeFile;
  const filesPanelActive =
    paneLayout.diffPanelOpen && paneLayout.utilityPanelTab === "files";
  const filesPanelTitle = "Files";
  const filesPanelMeta = activeWorktreeName ? activeWorktreeMetaLabel : null;

  const headerTitle = useMemo(() => {
    if (!activeFile?.path) {
      return activeSessionLabel ?? "Helm";
    }
    return activeFile.path.split("/").pop() ?? activeFile.path;
  }, [activeFile?.path, activeSessionLabel]);

  const headerSubtitle = useMemo(() => {
    if (activeFile) {
      return (
        activeWorktreeName ??
        activeSessionLabel ??
        activeWorktreeMetaFullLabel
      );
    }
    if (activeWorktreeName) {
      return `${activeRepoName ?? ""} • ${activeWorktreeMetaLabel}`;
    }
    return "A terminal built for agents";
  }, [
    activeFile,
    activeRepoName,
    activeSessionLabel,
    activeWorktreeMetaFullLabel,
    activeWorktreeMetaLabel,
    activeWorktreeName,
  ]);

  const confirmNavigation = useCallback(() => {
    if (!activeFile) {
      return true;
    }
    return filesPanel.confirmDiscardChanges();
  }, [activeFile, filesPanel]);

  const closeFileEditor = useCallback(() => {
    return filesPanel.closeEditor();
  }, [filesPanel]);

  const closeUtilityPanel = useCallback(() => {
    if (
      isLeavingFilesContext(
        paneLayout.diffPanelOpen,
        paneLayout.utilityPanelTab,
        null,
      ) &&
      !confirmNavigation()
    ) {
      return false;
    }
    paneLayout.closeUtilityPanel();
    return true;
  }, [
    confirmNavigation,
    paneLayout.diffPanelOpen,
    paneLayout.utilityPanelTab,
    paneLayout.closeUtilityPanel,
  ]);

  const toggleUtilityPanel = useCallback(
    (tab: UtilityPanelTab) => {
      if (
        isLeavingFilesContext(
          paneLayout.diffPanelOpen,
          paneLayout.utilityPanelTab,
          tab,
        ) &&
        !confirmNavigation()
      ) {
        return false;
      }
      paneLayout.toggleUtilityPanel(tab);
      return true;
    },
    [
      confirmNavigation,
      paneLayout.diffPanelOpen,
      paneLayout.utilityPanelTab,
      paneLayout.toggleUtilityPanel,
    ],
  );

  const dismissUtilityOverlay = useCallback(() => {
    if (paneLayout.diffPanelFullscreen) {
      paneLayout.setDiffPanelFullscreen(false);
      return true;
    }
    if (paneLayout.diffPanelOpen) {
      return closeUtilityPanel();
    }
    return false;
  }, [
    closeUtilityPanel,
    paneLayout.diffPanelFullscreen,
    paneLayout.diffPanelOpen,
    paneLayout.setDiffPanelFullscreen,
  ]);

  const toggleDiffFullscreen = useCallback(() => {
    if (
      isLeavingFilesContext(
        paneLayout.diffPanelOpen,
        paneLayout.utilityPanelTab,
        "diff",
      ) &&
      !confirmNavigation()
    ) {
      return false;
    }

    paneLayout.toggleDiffFullscreen();
    return true;
  }, [
    confirmNavigation,
    paneLayout.diffPanelOpen,
    paneLayout.toggleDiffFullscreen,
    paneLayout.utilityPanelTab,
  ]);

  const saveFileEditor = useCallback(() => {
    void filesPanel.saveActiveFile(fileEditorRef.current?.getCurrentContent());
  }, [filesPanel]);

  const focusTerminal = useCallback(() => {
    if (activeFile && !closeFileEditor()) {
      return false;
    }
    terminalRef.current?.focusActive();
    return true;
  }, [activeFile, closeFileEditor, terminalRef]);

  useEffect(() => {
    const previousPath = previousActiveFilePathRef.current;
    const currentPath = activeFile?.path ?? null;
    previousActiveFilePathRef.current = currentPath;

    if (!previousPath || currentPath || !activeSessionOpen) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      terminalRef.current?.focusActive();
    });

    return () => {
      window.cancelAnimationFrame(frame);
    };
  }, [activeFile?.path, activeSessionOpen, terminalRef]);

  return {
    activeFile,
    activeSessionLabel,
    closeFileEditor,
    closeUtilityPanel,
    confirmNavigation,
    dismissUtilityOverlay,
    fileEditorRef,
    filesPanelActive,
    filesPanelMeta,
    filesPanelTitle,
    focusTerminal,
    headerSubtitle,
    headerTitle,
    saveFileEditor,
    toggleDiffFullscreen,
    toggleUtilityPanel,
  };
}
