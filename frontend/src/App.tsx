import {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";

import "./App.css";
import {
  activateSession,
  bootstrap,
  chooseWorkspace,
  createSession,
  createWorkspaceSession,
  createWorktreeSession,
  killSession,
  resizeSession,
  sendSessionInput,
  updateSessionMode,
} from "./backend";
import { AgentPicker } from "./components/AgentPicker";
import { CommandPalette } from "./components/CommandPalette";
import { EmptySessionState } from "./components/app-shell/EmptySessionState";
import {
  FilesPanel,
  type FilesPanelHandle,
} from "./components/app-shell/FilesPanel";
import { MainHeader } from "./components/app-shell/MainHeader";
import { Sidebar } from "./components/app-shell/Sidebar";
import { UtilityPanel } from "./components/app-shell/UtilityPanel";
import {
  SessionLauncher,
  type SessionLaunchSelection,
} from "./components/SessionLauncher";
import {
  TerminalStage,
  type TerminalStageHandle,
} from "./components/TerminalStage";
import { useDiffPanel } from "./hooks/useDiffPanel";
import { useFileEditorShell } from "./hooks/useFileEditorShell";
import { useFilesPanel } from "./hooks/useFilesPanel";
import { useGlobalCommands } from "./hooks/useGlobalCommands";
import { usePaneLayout } from "./hooks/usePaneLayout";
import { usePeerPanelModel } from "./hooks/usePeerPanelModel";
import { useSessionActivity } from "./hooks/useSessionActivity";
import { useSessionPaths } from "./hooks/useSessionPaths";
import { useWailsEvent } from "./hooks/useWailsEvent";
import type {
  AppSnapshot,
  BootstrapResult,
  PeerStateDTO,
  RepoDTO,
  SessionLifecycleEvent,
  SessionOutputEvent,
  UIStateDTO,
} from "./types";
import {
  defaultAgentId,
  describeSessionLabel,
  describeWorktreeMeta,
  flattenSessions,
  nextWorktreeDefaults,
  repoVisibleSessions,
  sessionCycleTarget,
  suggestWorktreePath,
  type MenuAction,
  type SessionLauncherState,
  type WorkspacePickerState,
} from "./utils/appShell";

const splashLogoPath = "/helm-logo.png";

interface UIState {
  snapshot: AppSnapshot | null;
  commandPalette: boolean;
  workspacePicker: WorkspacePickerState | null;
  sessionLauncher: SessionLauncherState | null;
  collapsedRepoKeys: string[];
  notice: string | null;
  launching: boolean;
}

type Action =
  | { type: "setSnapshot"; snapshot: AppSnapshot }
  | { type: "setBootstrap"; result: BootstrapResult }
  | { type: "setNotice"; notice: string | null }
  | { type: "openCommandPalette" }
  | { type: "openWorkspacePicker"; picker: WorkspacePickerState }
  | { type: "openSessionLauncher"; launcher: SessionLauncherState }
  | { type: "closeModals" }
  | { type: "setLaunching"; launching: boolean }
  | { type: "toggleRepo"; repoKey: string };

const initialState: UIState = {
  snapshot: null,
  commandPalette: false,
  workspacePicker: null,
  sessionLauncher: null,
  collapsedRepoKeys: [],
  notice: null,
  launching: false,
};

const LazyFileEditor = lazy(async () => {
  const module = await import("./components/app-shell/FileEditor");
  return { default: module.FileEditor };
});

function reducer(state: UIState, action: Action): UIState {
  switch (action.type) {
    case "setSnapshot": {
      const existingKeys = new Set(
        action.snapshot.repos.map((repo) => repo.persistenceKey),
      );
      return {
        ...state,
        snapshot: action.snapshot,
        collapsedRepoKeys: state.collapsedRepoKeys.filter((key) =>
          existingKeys.has(key),
        ),
      };
    }
    case "setBootstrap": {
      const existingKeys = new Set(
        action.result.snapshot.repos.map((repo) => repo.persistenceKey),
      );
      const collapsedRepoKeys = action.result.uiState.collapsedRepoKeys ?? [];
      return {
        ...state,
        snapshot: action.result.snapshot,
        collapsedRepoKeys: collapsedRepoKeys.filter((key) =>
          existingKeys.has(key),
        ),
        notice: action.result.restoreNotice || state.notice,
      };
    }
    case "setNotice":
      return {
        ...state,
        notice: action.notice,
      };
    case "openCommandPalette":
      return {
        ...state,
        commandPalette: true,
        sessionLauncher: null,
        workspacePicker: null,
      };
    case "openWorkspacePicker":
      return {
        ...state,
        commandPalette: false,
        sessionLauncher: null,
        workspacePicker: action.picker,
      };
    case "openSessionLauncher":
      return {
        ...state,
        commandPalette: false,
        sessionLauncher: action.launcher,
        workspacePicker: null,
      };
    case "closeModals":
      return {
        ...state,
        commandPalette: false,
        sessionLauncher: null,
        workspacePicker: null,
      };
    case "setLaunching":
      return {
        ...state,
        launching: action.launching,
      };
    case "toggleRepo": {
      const isCollapsed = state.collapsedRepoKeys.includes(action.repoKey);
      return {
        ...state,
        collapsedRepoKeys: isCollapsed
          ? state.collapsedRepoKeys.filter((key) => key !== action.repoKey)
          : [...state.collapsedRepoKeys, action.repoKey],
      };
    }
    default:
      return state;
  }
}

function App() {
  const [state, dispatch] = useReducer(reducer, initialState);
  const [uiHydrated, setUIHydrated] = useState(false);
  const [peerState, setPeerState] = useState<PeerStateDTO>({
    peers: [],
    messages: [],
  });
  const terminalRef = useRef<TerminalStageHandle>(null);
  const filesPanelRef = useRef<FilesPanelHandle>(null);
  const fallbackActivationRef = useRef(0);

  const setNotice = (notice: string | null) => {
    dispatch({ type: "setNotice", notice });
  };

  const handleError = (error: unknown) => {
    setNotice(String(error));
  };

  const snapshot = state.snapshot;
  const repos = snapshot?.repos ?? [];
  const worktrees = useMemo(
    () => repos.flatMap((repo) => repo.worktrees),
    [repos],
  );
  const sessions = useMemo(() => flattenSessions(repos), [repos]);
  const repoById = useMemo(
    () => new Map(repos.map((repo) => [repo.id, repo])),
    [repos],
  );
  const worktreeById = useMemo(
    () => new Map(worktrees.map((worktree) => [worktree.id, worktree])),
    [worktrees],
  );

  const { handleSessionCwdChange, sessionPaths } = useSessionPaths(
    sessions,
    handleError,
  );
  const sessionActivity = useSessionActivity(sessions);
  const paneLayout = usePaneLayout({
    collapsedRepoKeys: state.collapsedRepoKeys,
    onError: handleError,
    uiHydrated,
  });

  const activeSession =
    sessions.find((session) => session.id === snapshot?.activeSessionId) ??
    null;
  const resolvedActiveSession =
    activeSession ?? sessions[sessions.length - 1] ?? null;
  const activeWorktree =
    (resolvedActiveSession
      ? worktreeById.get(resolvedActiveSession.worktreeId)
      : null) ??
    worktrees.find((worktree) => worktree.id === snapshot?.activeWorktreeId) ??
    null;
  const activeRepo =
    (activeWorktree ? repoById.get(activeWorktree.repoId) : null) ??
    repos.find((repo) => repo.id === snapshot?.activeRepoId) ??
    null;
  const activeSessionId = resolvedActiveSession?.id ?? 0;

  const activeSessionLabel = resolvedActiveSession
    ? describeSessionLabel(
        resolvedActiveSession,
        worktreeById.get(resolvedActiveSession.worktreeId) ?? null,
        sessionPaths[resolvedActiveSession.id] ?? null,
      )
    : null;
  const activeWorktreeMeta = describeWorktreeMeta(activeRepo, activeWorktree);
  const availableAgents = snapshot?.availableAgents ?? [];

  const sessionLabelByPeerId = useMemo(() => {
    const labels = new Map<string, string>();
    for (const session of sessions) {
      if (!session.peerId) {
        continue;
      }
      const worktree = worktreeById.get(session.worktreeId) ?? null;
      labels.set(
        session.peerId,
        describeSessionLabel(
          session,
          worktree,
          sessionPaths[session.id] ?? null,
        ).label,
      );
    }
    return labels;
  }, [sessionPaths, sessions, worktreeById]);

  const peerPanel = usePeerPanelModel({
    onError: handleError,
    peerState,
    sessionLabelByPeerId,
    setPeerState,
  });
  const diffPanel = useDiffPanel({
    activeWorktreeId: activeWorktree?.id ?? null,
    enabled:
      paneLayout.diffPanelOpen && paneLayout.utilityPanelTab === "diff",
  });
  const confirmDiscardFileChanges = useCallback(() => {
    return window.confirm("Discard unsaved file changes?");
  }, []);
  const filesPanel = useFilesPanel({
    activeWorktreeId: activeWorktree?.id ?? null,
    confirmDiscardPrompt: confirmDiscardFileChanges,
    enabled:
      paneLayout.diffPanelOpen && paneLayout.utilityPanelTab === "files",
  });

  const sidebarRepos = useMemo(() => {
    return repos.map((repo) => ({
      id: repo.id,
      isCollapsed: state.collapsedRepoKeys.includes(repo.persistenceKey),
      name: repo.name,
      persistenceKey: repo.persistenceKey,
      sessions: repoVisibleSessions(repo).map(({ session, worktree }) => {
        const sessionLabel = describeSessionLabel(
          session,
          worktree,
          sessionPaths[session.id] ?? null,
        );
        const sessionMeta = describeWorktreeMeta(repo, worktree);
        return {
          adapterId: session.adapterId,
          fullLabel: sessionLabel.fullLabel,
          id: session.id,
          isActive: session.id === activeSessionId,
          isBusy: sessionActivity.sessionActivity[session.id]?.phase === "busy",
          label: sessionLabel.label,
          metaLabel: sessionMeta.label,
          metaTitle: sessionMeta.fullLabel,
          outstandingPeerCount: session.outstandingPeerCount,
          title: session.label,
        };
      }),
    }));
  }, [
    repos,
    sessionActivity.sessionActivity,
    sessionPaths,
    activeSessionId,
    state.collapsedRepoKeys,
  ]);

  const isBootstrapping = !uiHydrated;
  const fileEditorShell = useFileEditorShell({
    activeRepoName: activeRepo?.name ?? null,
    activeSessionLabel: activeSessionLabel?.label ?? null,
    activeSessionOpen: Boolean(resolvedActiveSession),
    activeWorktreeName: activeWorktree?.name ?? null,
    activeWorktreeMetaFullLabel: activeWorktreeMeta.fullLabel,
    activeWorktreeMetaLabel: activeWorktreeMeta.label,
    filesPanel,
    paneLayout,
    terminalRef,
  });
  const activeFile = fileEditorShell.activeFile;
  const terminalAutoFocusActive =
    !activeFile &&
    !state.commandPalette &&
    !state.workspacePicker &&
    !state.sessionLauncher;

  useWailsEvent<[SessionOutputEvent]>("session:output", (payload) => {
    terminalRef.current?.writeOutput(payload.sessionId, payload.data);
    sessionActivity.handleSessionOutput(payload);
  });

  useWailsEvent<[SessionLifecycleEvent]>("session:lifecycle", (payload) => {
    sessionActivity.handleSessionLifecycle(payload);
    if (payload.error) {
      setNotice(`${payload.status}: ${payload.error}`);
    }
  });

  useWailsEvent<[AppSnapshot]>("app:snapshot", (payload) => {
    dispatch({ type: "setSnapshot", snapshot: payload });
  });

  useWailsEvent<[PeerStateDTO]>("peer:state", (payload) => {
    setPeerState(payload);
  });

  useEffect(() => {
    let mounted = true;

    bootstrap()
      .then((result) => {
        if (!mounted) {
          return;
        }
        dispatch({ type: "setBootstrap", result });
        paneLayout.hydrateUIState(result.uiState);
        setPeerState(result.peerState);
        setUIHydrated(true);
      })
      .catch((error) => {
        if (!mounted) {
          return;
        }
        setNotice(String(error));
        setUIHydrated(true);
      });

    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    if (
      !uiHydrated ||
      !snapshot ||
      activeSession ||
      !resolvedActiveSession
    ) {
      fallbackActivationRef.current = 0;
      return;
    }

    if (fallbackActivationRef.current === resolvedActiveSession.id) {
      return;
    }
    fallbackActivationRef.current = resolvedActiveSession.id;

    void activateSession(resolvedActiveSession.id)
      .then((nextSnapshot) => {
        dispatch({ type: "setSnapshot", snapshot: nextSnapshot });
      })
      .catch((error) => {
        handleError(error);
      });
  }, [activeSession, resolvedActiveSession, snapshot, uiHydrated]);

  const openWorkspaceFlow = async () => {
    try {
      const workspace = await chooseWorkspace();
      if (!workspace) {
        return;
      }
      if (availableAgents.length === 0) {
        setNotice("No installed agents found.");
        return;
      }
      dispatch({
        type: "openWorkspacePicker",
        picker: {
          defaultAgentId: defaultAgentId(snapshot),
          workspace,
        },
      });
    } catch (error) {
      handleError(error);
    }
  };

  const openSessionFlow = (repo: RepoDTO, defaultWorktreeId: number) => {
    if (availableAgents.length === 0) {
      setNotice("No installed agents found.");
      return;
    }

    const defaults = nextWorktreeDefaults(
      repo,
      activeWorktree && activeWorktree.repoId === repo.id ? activeWorktree : null,
    );

    dispatch({
      type: "openSessionLauncher",
      launcher: {
        defaultAgentId: defaultAgentId(snapshot),
        defaultBranchName: defaults.branchName,
        defaultSourceRef: defaults.sourceRef,
        defaultWorktreeId: defaultWorktreeId || repo.worktrees[0]?.id || 0,
        repo,
      },
    });
  };

  const openSessionForRepo = (repoId: number) => {
    const repo = repoById.get(repoId);
    if (!repo) {
      return;
    }

    openSessionFlow(
      repo,
      activeWorktree?.repoId === repo.id
        ? activeWorktree.id
        : (repo.worktrees[0]?.id ?? 0),
    );
  };

  const launchWorkspaceFromPicker = async (agentId: string) => {
    if (!state.workspacePicker) {
      return;
    }
    if (!fileEditorShell.confirmNavigation()) {
      return;
    }

    dispatch({ type: "setLaunching", launching: true });
    setNotice(null);

    try {
      const nextSnapshot = await createWorkspaceSession(
        state.workspacePicker.workspace.rootPath,
        agentId,
      );
      dispatch({ type: "setSnapshot", snapshot: nextSnapshot });
      dispatch({ type: "closeModals" });
    } catch (error) {
      handleError(error);
    } finally {
      dispatch({ type: "setLaunching", launching: false });
    }
  };

  const launchSessionFromDialog = async (selection: SessionLaunchSelection) => {
    if (!state.sessionLauncher) {
      return;
    }
    if (!fileEditorShell.confirmNavigation()) {
      return;
    }

    dispatch({ type: "setLaunching", launching: true });
    setNotice(null);

    try {
      const nextSnapshot =
        selection.mode === "existing"
          ? await createSession(selection.worktreeId, selection.agentId)
          : await createWorktreeSession(state.sessionLauncher.repo.id, {
              agentId: selection.agentId,
              branchName: selection.branchName,
              mode: "new-branch",
              path: suggestWorktreePath(
                state.sessionLauncher.repo.rootPath,
                selection.branchName,
              ),
              sourceRef: state.sessionLauncher.defaultSourceRef,
            });
      dispatch({ type: "setSnapshot", snapshot: nextSnapshot });
      dispatch({ type: "closeModals" });
    } catch (error) {
      handleError(error);
    } finally {
      dispatch({ type: "setLaunching", launching: false });
    }
  };

  const activateSessionFlow = async (sessionId: number) => {
    if (
      snapshot?.activeSessionId === sessionId ||
      !fileEditorShell.confirmNavigation()
    ) {
      return;
    }

    try {
      const nextSnapshot = await activateSession(sessionId);
      dispatch({ type: "setSnapshot", snapshot: nextSnapshot });
    } catch (error) {
      handleError(error);
    }
  };

  const killSessionFlow = async (sessionId: number) => {
    if (
      snapshot?.activeSessionId === sessionId &&
      !fileEditorShell.confirmNavigation()
    ) {
      return;
    }

    try {
      const nextSnapshot = await killSession(sessionId);
      dispatch({ type: "setSnapshot", snapshot: nextSnapshot });
    } catch (error) {
      handleError(error);
    }
  };

  const dismissTransientUI = async () => {
    if (state.commandPalette || state.workspacePicker || state.sessionLauncher) {
      dispatch({ type: "closeModals" });
      return true;
    }
    if (activeFile) {
      return fileEditorShell.focusTerminal();
    }
    if (paneLayout.diffPanelFullscreen || paneLayout.diffPanelOpen) {
      return fileEditorShell.dismissUtilityOverlay();
    }

    return false;
  };

  const cycleSessions = async (direction: 1 | -1) => {
    const target = sessionCycleTarget(snapshot, sessions, direction);
    if (target) {
      await activateSessionFlow(target.id);
    }
  };

  const focusFilesPanel = useCallback(() => {
    if (!activeWorktree) {
      return false;
    }

    if (
      (!paneLayout.diffPanelOpen || paneLayout.utilityPanelTab !== "files") &&
      !fileEditorShell.toggleUtilityPanel("files")
    ) {
      return false;
    }

    window.requestAnimationFrame(() => {
      filesPanelRef.current?.focusPrimaryAction();
    });
    return true;
  }, [
    activeWorktree,
    fileEditorShell,
    paneLayout.diffPanelOpen,
    paneLayout.utilityPanelTab,
  ]);

  const ensureDiffPanelVisible = useCallback(() => {
    if (
      paneLayout.diffPanelOpen &&
      paneLayout.utilityPanelTab === "diff"
    ) {
      return true;
    }

    return fileEditorShell.toggleUtilityPanel("diff");
  }, [
    fileEditorShell,
    paneLayout.diffPanelOpen,
    paneLayout.utilityPanelTab,
  ]);

  const performAppAction = async (action: MenuAction) => {
    switch (action) {
      case "new-workspace":
        await openWorkspaceFlow();
        return;
      case "new-session":
        if (activeRepo) {
          openSessionFlow(
            activeRepo,
            activeWorktree?.repoId === activeRepo.id
              ? activeWorktree.id
              : (activeRepo.worktrees[0]?.id ?? 0),
          );
          return;
        }
        await openWorkspaceFlow();
        return;
      case "close-session":
        if (activeSessionId) {
          await killSessionFlow(activeSessionId);
        }
        return;
      case "save-file-editor":
        fileEditorShell.saveFileEditor();
        return;
      case "toggle-sidebar":
        paneLayout.toggleSidebar();
        return;
      case "toggle-diff":
        fileEditorShell.toggleUtilityPanel("diff");
        return;
      case "toggle-files":
        fileEditorShell.toggleUtilityPanel("files");
        return;
      case "toggle-peers":
        fileEditorShell.toggleUtilityPanel("peers");
        return;
      case "toggle-diff-fullscreen":
        fileEditorShell.toggleDiffFullscreen();
        return;
      case "focus-terminal":
        fileEditorShell.focusTerminal();
        return;
      case "focus-files-panel":
        focusFilesPanel();
        return;
      case "zoom-out-terminal":
        paneLayout.zoomOutTerminal();
        return;
      case "reset-terminal-zoom":
        paneLayout.resetTerminalZoom();
        return;
      case "zoom-in-terminal":
        paneLayout.zoomInTerminal();
        return;
      case "refresh-diff":
        if (ensureDiffPanelVisible()) {
          void diffPanel.refreshDiffPanel();
        }
        return;
      case "zoom-out-diff":
        if (ensureDiffPanelVisible()) {
          diffPanel.zoomOut();
        }
        return;
      case "reset-diff-zoom":
        if (ensureDiffPanelVisible()) {
          diffPanel.resetDiffTextZoom();
        }
        return;
      case "zoom-in-diff":
        if (ensureDiffPanelVisible()) {
          diffPanel.zoomIn();
        }
        return;
      case "previous-session":
      case "previous-session-alt":
        await cycleSessions(-1);
        return;
      case "next-session":
      case "next-session-alt":
        await cycleSessions(1);
        return;
      case "command-palette":
        dispatch({ type: "openCommandPalette" });
        return;
      case "dismiss-overlay":
        await dismissTransientUI();
        return;
    }
  };

  useGlobalCommands({ performAppAction });

  useEffect(() => {
    if (!uiHydrated) {
      return;
    }
    const splash = document.getElementById("startup-splash");
    if (splash) {
      splash.remove();
    }
  }, [uiHydrated]);

  if (isBootstrapping) {
    return (
      <div className="app-shell">
        <div className="drag-strip" />
        <div className="launch-splash">
          <div className="launch-splash__content">
            <img
              alt="Helm"
              className="launch-splash__logo"
              src={splashLogoPath}
            />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <div className="drag-strip" />

      <div
        ref={paneLayout.windowContentRef}
        className={`window-content${paneLayout.diffPanelFullscreen ? " is-diff-fullscreen" : ""}`}
        style={paneLayout.windowContentStyle}
      >
        <Sidebar
          open={paneLayout.sidebarOpen}
          repos={sidebarRepos}
          onActivateSession={(sessionId) => {
            void activateSessionFlow(sessionId);
          }}
          onCloseSession={(sessionId) => {
            void killSessionFlow(sessionId);
          }}
          onOpenSession={openSessionForRepo}
          onOpenWorkspace={() => {
            void openWorkspaceFlow();
          }}
          onToggleRepo={(repoKey) => {
            dispatch({ type: "toggleRepo", repoKey });
          }}
          onToggleSidebar={paneLayout.toggleSidebar}
        />

        <div
          aria-hidden={!paneLayout.sidebarOpen}
          className={`sidebar-resizer${paneLayout.sidebarOpen ? "" : " is-hidden"}`}
          onPointerDown={paneLayout.beginSidebarResize}
        />

        <main className="main-panel">
          <MainHeader
            activeAgentId={resolvedActiveSession?.adapterId ?? null}
            activeSessionLabel={fileEditorShell.activeSessionLabel}
            diffPanelActive={
              paneLayout.diffPanelOpen && paneLayout.utilityPanelTab === "diff"
            }
            fileEditorDirty={filesPanel.hasDirtyChanges}
            fileEditorOpen={Boolean(activeFile)}
            fileEditorSaving={activeFile?.saving ?? false}
            filesPanelActive={fileEditorShell.filesPanelActive}
            localOutstandingPeerCount={peerPanel.localOutstandingPeerCount}
            onCloseFileEditor={fileEditorShell.closeFileEditor}
            onSaveFileEditor={fileEditorShell.saveFileEditor}
            peersPanelActive={
              paneLayout.diffPanelOpen && paneLayout.utilityPanelTab === "peers"
            }
            sidebarOpen={paneLayout.sidebarOpen}
            subtitle={fileEditorShell.headerSubtitle}
            title={fileEditorShell.headerTitle}
            onToggleDiff={() => {
              fileEditorShell.toggleUtilityPanel("diff");
            }}
            onToggleFiles={() => {
              fileEditorShell.toggleUtilityPanel("files");
            }}
            onTogglePeers={() => {
              fileEditorShell.toggleUtilityPanel("peers");
            }}
            onToggleSidebar={paneLayout.toggleSidebar}
          />

          <section className="terminal-panel">
            {resolvedActiveSession ? (
              <TerminalStage
                ref={terminalRef}
                activeSessionId={activeSessionId}
                autoFocusActive={terminalAutoFocusActive}
                fontSize={paneLayout.terminalFontSize}
                sessions={sessions}
                onInput={(sessionId, data) => {
                  if (data.includes("\r") || data.includes("\n")) {
                    sessionActivity.armSessionActivity(sessionId);
                  }
                  void sendSessionInput(sessionId, data);
                }}
                onResize={(sessionId, cols, rows) => {
                  void resizeSession(sessionId, cols, rows);
                }}
                onSessionCwdChange={handleSessionCwdChange}
                onSessionModeChange={(sessionId, adapterId) => {
                  sessionActivity.handleSessionModeChange(
                    sessionId,
                    adapterId === "shell" ? "shell" : "agent",
                  );
                  void updateSessionMode(sessionId, adapterId).catch((error) => {
                    handleError(error);
                  });
                }}
              />
            ) : (
              <EmptySessionState
                onOpenWorkspace={() => {
                  void openWorkspaceFlow();
                }}
              />
            )}

            {activeFile ? (
              <div className="terminal-panel__overlay">
                <Suspense
                  fallback={
                    <div className="files-panel files-panel--editor">
                      <div className="files-panel__editor-shell files-panel__editor-shell--loading" />
                    </div>
                  }
                >
                  <LazyFileEditor
                    ref={fileEditorShell.fileEditorRef}
                    activeFile={activeFile}
                    onDirtyChange={filesPanel.setActiveFileDirty}
                    onSave={filesPanel.saveActiveFile}
                  />
                </Suspense>
              </div>
            ) : null}
          </section>

          {state.notice ? <div className="notice-banner">{state.notice}</div> : null}
        </main>

        <div
          aria-hidden={!paneLayout.diffPanelOpen || paneLayout.diffPanelFullscreen}
          className={`diff-resizer${paneLayout.diffPanelOpen && !paneLayout.diffPanelFullscreen ? "" : " is-hidden"}`}
          onPointerDown={paneLayout.beginDiffPanelResize}
        />

        <UtilityPanel
          clearingPeerMessages={peerPanel.clearingPeerMessages}
          deletingPeerMessageId={peerPanel.deletingPeerMessageId}
          diffBodyState={diffPanel.bodyState}
          diffPanelStyle={diffPanel.diffPanelStyle}
          diffTextZoom={diffPanel.diffTextZoom}
          filesBody={
            <FilesPanel
              ref={filesPanelRef}
              activeFilePath={filesPanel.activeFile?.path ?? null}
              activeWorktreeId={activeWorktree?.id ?? null}
              directoryStates={filesPanel.directoryStates}
              expandedDirectoryPaths={filesPanel.expandedDirectoryPaths}
              openingFilePath={filesPanel.openingFilePath}
              rootDirectoryState={filesPanel.rootDirectoryState}
              onLoadDirectory={filesPanel.loadDirectory}
              onOpenFile={filesPanel.openFile}
              onToggleDirectory={filesPanel.toggleDirectory}
            />
          }
          filesMeta={fileEditorShell.filesPanelMeta}
          filesTitle={fileEditorShell.filesPanelTitle}
          fullscreen={paneLayout.diffPanelFullscreen}
          isOpen={paneLayout.diffPanelOpen}
          livePeerCount={peerPanel.livePeerCount}
          livePeers={peerPanel.livePeers}
          recentMessageCount={peerPanel.recentMessageCount}
          recentMessages={peerPanel.recentMessages}
          repoName={activeRepo?.name ?? null}
          tab={paneLayout.utilityPanelTab}
          utilityLabel={activeWorktree ? activeWorktreeMeta.label : null}
          zoomInEnabled={diffPanel.canZoomIn}
          zoomOutEnabled={diffPanel.canZoomOut}
          onClearPeerMessages={() => {
            void peerPanel.handleClearPeerMessages();
          }}
          onClose={fileEditorShell.closeUtilityPanel}
          onDeletePeerMessage={(messageId) => {
            void peerPanel.handleDeletePeerMessage(messageId);
          }}
          onRefreshDiff={() => {
            void diffPanel.refreshDiffPanel();
          }}
          onResetDiffTextZoom={diffPanel.resetDiffTextZoom}
          onToggleDiffTarget={diffPanel.toggleDiffTarget}
          onToggleFullscreen={() =>
            paneLayout.setDiffPanelFullscreen((current) => !current)
          }
          onZoomInDiff={diffPanel.zoomIn}
          onZoomOutDiff={diffPanel.zoomOut}
        />
      </div>

      <AgentPicker
        agents={availableAgents}
        defaultAgentId={
          state.workspacePicker?.defaultAgentId ?? defaultAgentId(snapshot)
        }
        submitting={state.launching}
        target={state.workspacePicker?.workspace ?? null}
        onClose={() => dispatch({ type: "closeModals" })}
        onSelect={(agentId) => {
          void launchWorkspaceFromPicker(agentId);
        }}
      />

      <SessionLauncher
        agents={availableAgents}
        defaultAgentId={
          state.sessionLauncher?.defaultAgentId ?? defaultAgentId(snapshot)
        }
        defaultBranchName={
          state.sessionLauncher?.defaultBranchName ?? "feature/worktree"
        }
        defaultWorktreeId={state.sessionLauncher?.defaultWorktreeId ?? 0}
        repo={state.sessionLauncher?.repo ?? null}
        submitting={state.launching}
        onClose={() => dispatch({ type: "closeModals" })}
        onSelect={(selection) => {
          void launchSessionFromDialog(selection);
        }}
      />

      <CommandPalette
        open={state.commandPalette}
        repos={repos}
        sessionPaths={sessionPaths}
        activeSessionId={activeSessionId}
        activeWorktreeId={activeWorktree?.id ?? null}
        onActivateSession={(sessionId) => {
          dispatch({ type: "closeModals" });
          fileEditorShell.closeFileEditor();
          void activateSessionFlow(sessionId);
        }}
        onOpenFile={(path) => {
          dispatch({ type: "closeModals" });
          void filesPanel.openFile(path);
        }}
        onAction={(action) => {
          dispatch({ type: "closeModals" });
          void performAppAction(action);
        }}
        onClose={() => dispatch({ type: "closeModals" })}
      />
    </div>
  );
}

export default App;
