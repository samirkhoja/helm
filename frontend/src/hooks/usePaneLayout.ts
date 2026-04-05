import { type CSSProperties, useEffect, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";

import { saveUIState } from "../backend";
import type { UIStateDTO } from "../types";
import type { UtilityPanelTab } from "../utils/appShell";

type DragState = {
  edge: "left" | "right";
  pointerId: number;
  startX: number;
  startWidth: number;
  currentWidth: number;
};

type UsePaneLayoutOptions = {
  uiHydrated: boolean;
  collapsedRepoKeys: string[];
  onError: (error: unknown) => void;
};

const minSidebarWidth = 208;
const maxSidebarWidth = 420;
const minDiffPanelWidth = 280;
const maxDiffPanelWidth = 560;
const defaultUtilityPanelTab: UtilityPanelTab = "diff";
const defaultTerminalFontSize = 12;
const minTerminalFontSize = 11;
const maxTerminalFontSize = 24;

function clearBrowserSelection() {
  window.getSelection()?.removeAllRanges();
}

function roundPanelWidth(value: number) {
  return Math.round(value);
}

function normalizeUtilityPanelTab(
  value: UIStateDTO["utilityPanelTab"] | string | null | undefined,
): UtilityPanelTab {
  switch (value) {
    case "diff":
    case "files":
    case "peers":
      return value;
    default:
      return defaultUtilityPanelTab;
  }
}

function clampTerminalFontSize(value: number | null | undefined) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return defaultTerminalFontSize;
  }
  if (value < minTerminalFontSize) {
    return defaultTerminalFontSize;
  }
  if (value > maxTerminalFontSize) {
    return maxTerminalFontSize;
  }
  return Math.round(value);
}

export function usePaneLayout(options: UsePaneLayoutOptions) {
  const { uiHydrated, collapsedRepoKeys, onError } = options;

  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [sidebarWidth, setSidebarWidth] = useState(248);
  const [diffPanelOpen, setDiffPanelOpen] = useState(false);
  const [diffPanelWidth, setDiffPanelWidth] = useState(360);
  const [diffPanelFullscreen, setDiffPanelFullscreen] = useState(false);
  const [utilityPanelTab, setUtilityPanelTab] =
    useState<UtilityPanelTab>("diff");
  const [terminalFontSize, setTerminalFontSize] = useState(
    defaultTerminalFontSize,
  );

  const windowContentRef = useRef<HTMLDivElement>(null);
  const dragStateRef = useRef<DragState | null>(null);
  const onErrorRef = useRef(onError);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  const clampSidebarWidth = (value: number) =>
    roundPanelWidth(
      Math.min(maxSidebarWidth, Math.max(minSidebarWidth, value)),
    );

  const clampDiffPanelWidth = (value: number) =>
    roundPanelWidth(
      Math.min(maxDiffPanelWidth, Math.max(minDiffPanelWidth, value)),
    );

  const setWindowVariable = (name: string, value: string) => {
    windowContentRef.current?.style.setProperty(name, value);
  };

  const hydrateUIState = (uiState: UIStateDTO) => {
    setSidebarOpen(uiState.sidebarOpen);
    setSidebarWidth(clampSidebarWidth(uiState.sidebarWidth));
    setDiffPanelOpen(uiState.diffPanelOpen);
    setDiffPanelWidth(clampDiffPanelWidth(uiState.diffPanelWidth));
    setTerminalFontSize(clampTerminalFontSize(uiState.terminalFontSize));
    setUtilityPanelTab(normalizeUtilityPanelTab(uiState.utilityPanelTab));
  };

  const toggleSidebar = () => {
    setSidebarOpen((current) => !current);
  };

  const closeUtilityPanel = () => {
    setDiffPanelFullscreen(false);
    setDiffPanelOpen(false);
  };

  const toggleUtilityPanel = (tab: UtilityPanelTab) => {
    if (diffPanelOpen && utilityPanelTab === tab) {
      closeUtilityPanel();
      return;
    }

    setUtilityPanelTab(tab);
    if (tab !== "diff") {
      setDiffPanelFullscreen(false);
    }
    setDiffPanelOpen(true);
  };

  const toggleDiffFullscreen = () => {
    if (!diffPanelOpen || utilityPanelTab !== "diff") {
      setUtilityPanelTab("diff");
      setDiffPanelOpen(true);
      setDiffPanelFullscreen(true);
      return;
    }

    setDiffPanelFullscreen((current) => !current);
  };

  const dismissUtilityOverlay = () => {
    if (diffPanelFullscreen) {
      setDiffPanelFullscreen(false);
      return true;
    }
    if (diffPanelOpen) {
      closeUtilityPanel();
      return true;
    }
    return false;
  };

  const zoomInTerminal = () => {
    setTerminalFontSize((current) =>
      Math.min(maxTerminalFontSize, current + 1),
    );
  };

  const zoomOutTerminal = () => {
    setTerminalFontSize((current) =>
      Math.max(minTerminalFontSize, current - 1),
    );
  };

  const resetTerminalZoom = () => {
    setTerminalFontSize(defaultTerminalFontSize);
  };

  const beginSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!sidebarOpen) {
      return;
    }

    event.preventDefault();
    clearBrowserSelection();
    dragStateRef.current = {
      edge: "left",
      pointerId: event.pointerId,
      startX: event.clientX,
      startWidth: sidebarWidth,
      currentWidth: sidebarWidth,
    };
    document.body.classList.add("is-resizing-pane");
  };

  const beginDiffPanelResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!diffPanelOpen || diffPanelFullscreen) {
      return;
    }

    event.preventDefault();
    clearBrowserSelection();
    dragStateRef.current = {
      edge: "right",
      pointerId: event.pointerId,
      startX: event.clientX,
      startWidth: diffPanelWidth,
      currentWidth: diffPanelWidth,
    };
    document.body.classList.add("is-resizing-pane");
  };

  useEffect(() => {
    const onPointerMove = (event: PointerEvent) => {
      const dragState = dragStateRef.current;
      if (!dragState || dragState.pointerId !== event.pointerId) {
        return;
      }

      clearBrowserSelection();

      const nextWidth =
        dragState.edge === "left"
          ? clampSidebarWidth(
              dragState.startWidth + (event.clientX - dragState.startX),
            )
          : clampDiffPanelWidth(
              dragState.startWidth + (dragState.startX - event.clientX),
            );

      dragState.currentWidth = nextWidth;

      if (dragState.edge === "left") {
        setWindowVariable("--sidebar-track", `${nextWidth}px`);
        return;
      }

      setWindowVariable("--diff-track", `${nextWidth}px`);
    };

    const stopDragging = (event: PointerEvent) => {
      const dragState = dragStateRef.current;
      if (!dragState || dragState.pointerId !== event.pointerId) {
        return;
      }

      dragStateRef.current = null;
      clearBrowserSelection();
      document.body.classList.remove("is-resizing-pane");

      if (dragState.edge === "left") {
        setSidebarWidth(dragState.currentWidth);
        return;
      }

      setDiffPanelWidth(dragState.currentWidth);
    };

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", stopDragging);
    window.addEventListener("pointercancel", stopDragging);

    return () => {
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", stopDragging);
      window.removeEventListener("pointercancel", stopDragging);
      document.body.classList.remove("is-resizing-pane");
    };
  }, []);

  useEffect(() => {
    if (!diffPanelOpen || utilityPanelTab !== "diff") {
      setDiffPanelFullscreen(false);
    }
  }, [diffPanelOpen, utilityPanelTab]);

  useEffect(() => {
    if (!uiHydrated) {
      return;
    }

    const timer = window.setTimeout(() => {
      void saveUIState({
        sidebarOpen,
        sidebarWidth: roundPanelWidth(sidebarWidth),
        diffPanelOpen,
        diffPanelWidth: roundPanelWidth(diffPanelWidth),
        terminalFontSize: clampTerminalFontSize(terminalFontSize),
        utilityPanelTab,
        collapsedRepoKeys,
      }).catch((error) => {
        onErrorRef.current(error);
      });
    }, 180);

    return () => {
      window.clearTimeout(timer);
    };
  }, [
    collapsedRepoKeys,
    diffPanelOpen,
    diffPanelWidth,
    terminalFontSize,
    utilityPanelTab,
    sidebarOpen,
    sidebarWidth,
    uiHydrated,
  ]);

  const windowContentStyle = {
    "--sidebar-track": sidebarOpen ? `${sidebarWidth}px` : "0px",
    "--sidebar-resizer-track": sidebarOpen ? "10px" : "0px",
    "--diff-resizer-track": diffPanelOpen ? "10px" : "0px",
    "--diff-track": diffPanelOpen ? `${diffPanelWidth}px` : "0px",
    gridTemplateColumns:
      "var(--sidebar-track) var(--sidebar-resizer-track) minmax(0, 1fr) var(--diff-resizer-track) var(--diff-track)",
  } as CSSProperties;

  return {
    beginDiffPanelResize,
    beginSidebarResize,
    closeUtilityPanel,
    diffPanelFullscreen,
    diffPanelOpen,
    dismissUtilityOverlay,
    hydrateUIState,
    resetTerminalZoom,
    setDiffPanelFullscreen,
    sidebarOpen,
    terminalFontSize,
    toggleDiffFullscreen,
    toggleSidebar,
    toggleUtilityPanel,
    utilityPanelTab,
    windowContentRef,
    windowContentStyle,
    zoomInTerminal,
    zoomOutTerminal,
  };
}
