import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";

import {
  commitWorktree,
  createWorktreeBranch,
  getCommitRangeDiff,
  getFileDiff,
  getWorktreeCommitHistory,
  getWorktreeDiff,
  pushWorktree,
  stageWorktreeAll,
  stageWorktreePath,
  unstageWorktreePath,
} from "../backend";
import type {
  AppSnapshot,
  CommitDiff,
  FileDiff,
  GitCommitSummary,
  WorktreeDiff,
} from "../types";

export type DiffMode = "changes" | "history";

export type DiffTarget = {
  path: string;
  staged: boolean;
  kind: "staged" | "unstaged" | "untracked";
};

export type FileDiffState = {
  loading: boolean;
  error: string | null;
  data: FileDiff | null;
};

export type CommitDiffState = {
  loading: boolean;
  error: string | null;
  data: CommitDiff | null;
};

export type DiffFilePathBusyKind = "stagePath" | "unstagePath";

export type DiffSectionItemViewModel = {
  addedLabel: string | null;
  fileDiffState: FileDiffState;
  isOpen: boolean;
  key: string;
  path: string;
  pathBusy: DiffFilePathBusyKind | null;
  removedLabel: string | null;
  target: DiffTarget;
};

export type DiffSectionViewModel = {
  emptyText: string;
  items: DiffSectionItemViewModel[];
  key: string;
  title: string;
};

export type DiffActionStatus = {
  kind: "error" | "success";
  message: string;
};

export type DiffHistoryViewModel = {
  commits: GitCommitSummary[];
  loading: boolean;
  error: string | null;
  baseRef: string | null;
  headRef: string | null;
  compareState: CommitDiffState;
};

export type DiffBodyState =
  | { kind: "no-worktree" }
  | { kind: "error"; message: string }
  | { kind: "loading" }
  | { kind: "not-git" }
  | { kind: "empty" }
  | {
      kind: "ready";
      worktreeId: number;
      mode: DiffMode;
      gitBranch: string;
      actionStatus: DiffActionStatus | null;
      busyAction: "branch" | "commit" | "push" | "stage" | null;
      canCommit: boolean;
      canCreateBranch: boolean;
      canPush: boolean;
      canStageAll: boolean;
      history: DiffHistoryViewModel;
      sections: DiffSectionViewModel[];
      stagedCount: number;
      unstagedCount: number;
      untrackedCount: number;
    };

type DiffState = {
  data: WorktreeDiff | null;
  error: string | null;
  loading: boolean;
};

type HistoryState = DiffHistoryViewModel;

type UseDiffPanelOptions = {
  activeWorktreeId: number | null;
  enabled: boolean;
  onSnapshot: (snapshot: AppSnapshot) => void;
};

const minDiffTextZoom = 80;
const maxDiffTextZoom = 170;
const diffTextZoomStep = 10;
const commitHistoryLimit = 10;
const detachedHeadLabel = "Detached HEAD";
const noGitBranchLabel = "No git branch";

function formatDiffDelta(value: number, prefix: "+" | "-") {
  return `${prefix}${value}`;
}

function buildDiffTargets(diff: WorktreeDiff): DiffTarget[] {
  return [
    ...diff.unstaged.map((item) => ({
      kind: "unstaged" as const,
      path: item.path,
      staged: false,
    })),
    ...diff.staged.map((item) => ({
      kind: "staged" as const,
      path: item.path,
      staged: true,
    })),
    ...diff.untracked.map((path) => ({
      kind: "untracked" as const,
      path,
      staged: false,
    })),
  ];
}

function buildDiffSummarySignature(diff: WorktreeDiff | null) {
  if (!diff) {
    return "";
  }

  return JSON.stringify({
    staged: diff.staged.map((item) => ({
      added: item.added,
      path: item.path,
      removed: item.removed,
      status: item.status,
    })),
    unstaged: diff.unstaged.map((item) => ({
      added: item.added,
      path: item.path,
      removed: item.removed,
      status: item.status,
    })),
    untracked: diff.untracked,
    worktreeId: diff.worktreeId,
  });
}

function isSameDiffTarget(left: DiffTarget, right: DiffTarget) {
  return (
    left.kind === right.kind &&
    left.path === right.path &&
    left.staged === right.staged
  );
}

function diffTargetKey(worktreeId: number, target: DiffTarget) {
  return `${worktreeId}:${target.kind}:${target.staged ? "staged" : "unstaged"}:${target.path}`;
}

function resolveHistorySelection(
  commits: GitCommitSummary[],
  currentBaseRef: string | null,
  currentHeadRef: string | null,
) {
  const available = new Set(commits.map((item) => item.hash));

  const headRef =
    currentHeadRef && available.has(currentHeadRef)
      ? currentHeadRef
      : (commits[0]?.hash ?? null);

  let baseRef =
    currentBaseRef && available.has(currentBaseRef) ? currentBaseRef : null;

  if (!baseRef && commits.length > 1) {
    const fallback = commits.find((item) => item.hash !== headRef);
    baseRef = fallback?.hash ?? null;
  }

  return {
    baseRef,
    headRef,
  };
}

const emptyFileDiffState: FileDiffState = {
  data: null,
  error: null,
  loading: false,
};

const emptyCommitDiffState: CommitDiffState = {
  data: null,
  error: null,
  loading: false,
};

const emptyHistoryState: HistoryState = {
  baseRef: null,
  commits: [],
  compareState: emptyCommitDiffState,
  error: null,
  headRef: null,
  loading: false,
};

export function useDiffPanel(options: UseDiffPanelOptions) {
  const { activeWorktreeId, enabled, onSnapshot } = options;

  const [diffState, setDiffState] = useState<DiffState>({
    data: null,
    error: null,
    loading: false,
  });
  const [historyState, setHistoryState] = useState<HistoryState>(emptyHistoryState);
  const [mode, setMode] = useState<DiffMode>("changes");
  const [openDiffTargets, setOpenDiffTargets] = useState<DiffTarget[]>([]);
  const [fileDiffStates, setFileDiffStates] = useState<
    Record<string, FileDiffState>
  >({});
  const [diffTextZoom, setDiffTextZoom] = useState(100);
  const [actionStatus, setActionStatus] = useState<DiffActionStatus | null>(null);
  const [busyAction, setBusyAction] = useState<
    "branch" | "commit" | "push" | "stage" | null
  >(null);
  const [busyPathAction, setBusyPathAction] = useState<{
    path: string;
    kind: DiffFilePathBusyKind;
  } | null>(null);

  const fileDiffCacheRef = useRef(new Map<string, FileDiff>());
  const diffSummarySignatureRef = useRef("");
  const currentWorktreeIdRef = useRef<number | null>(null);
  const diffRequestIdRef = useRef(0);
  const historyRequestIdRef = useRef(0);
  const compareRequestIdRef = useRef(0);

  const loadWorktreeDiff = async (worktreeId: number, showLoading = true) => {
    const requestId = ++diffRequestIdRef.current;
    if (showLoading) {
      setDiffState((current) => ({
        ...current,
        error: null,
        loading: true,
      }));
    }

    try {
      const data = await getWorktreeDiff(worktreeId);
      if (
        requestId !== diffRequestIdRef.current ||
        currentWorktreeIdRef.current !== worktreeId
      ) {
        return false;
      }

      const nextSignature = buildDiffSummarySignature(data);
      const didChange = diffSummarySignatureRef.current !== nextSignature;
      diffSummarySignatureRef.current = nextSignature;
      if (didChange) {
        fileDiffCacheRef.current.clear();
      }

      setDiffState({
        data,
        error: null,
        loading: false,
      });
      return true;
    } catch (error) {
      if (
        requestId !== diffRequestIdRef.current ||
        currentWorktreeIdRef.current !== worktreeId
      ) {
        return false;
      }

      setDiffState((current) => ({
        data: current.data,
        error: String(error),
        loading: false,
      }));
      return false;
    }
  };

  const loadCommitHistory = async (
    worktreeId: number,
    preserveSelection = true,
  ) => {
    const requestId = ++historyRequestIdRef.current;
    setHistoryState((current) => ({
      ...current,
      error: null,
      loading: true,
    }));

    try {
      const commits = await getWorktreeCommitHistory(worktreeId, commitHistoryLimit);
      if (
        requestId !== historyRequestIdRef.current ||
        currentWorktreeIdRef.current !== worktreeId
      ) {
        return false;
      }

      setHistoryState((current) => {
        const selection = resolveHistorySelection(
          commits,
          preserveSelection ? current.baseRef : null,
          preserveSelection ? current.headRef : null,
        );
        const sameSelection =
          selection.baseRef === current.baseRef &&
          selection.headRef === current.headRef;
        return {
          baseRef: selection.baseRef,
          commits,
          compareState:
            selection.baseRef && selection.headRef && sameSelection
              ? current.compareState
              : emptyCommitDiffState,
          error: null,
          headRef: selection.headRef,
          loading: false,
        };
      });
      return true;
    } catch (error) {
      if (
        requestId !== historyRequestIdRef.current ||
        currentWorktreeIdRef.current !== worktreeId
      ) {
        return false;
      }

      setHistoryState((current) => ({
        ...current,
        error: String(error),
        loading: false,
      }));
      return false;
    }
  };

  useEffect(() => {
    if (!enabled) {
      return;
    }

    if (!activeWorktreeId) {
      currentWorktreeIdRef.current = null;
      diffSummarySignatureRef.current = "";
      fileDiffCacheRef.current.clear();
      setDiffState({
        data: null,
        error: null,
        loading: false,
      });
      setHistoryState(emptyHistoryState);
      setOpenDiffTargets([]);
      setFileDiffStates({});
      setActionStatus(null);
      setBusyAction(null);
      return;
    }

    const worktreeChanged = currentWorktreeIdRef.current !== activeWorktreeId;
    currentWorktreeIdRef.current = activeWorktreeId;

    if (worktreeChanged) {
      diffSummarySignatureRef.current = "";
      fileDiffCacheRef.current.clear();
      setOpenDiffTargets([]);
      setFileDiffStates({});
      setActionStatus(null);
      setBusyAction(null);
      setHistoryState(emptyHistoryState);
    }

    void loadWorktreeDiff(activeWorktreeId, true);

    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") {
        return;
      }
      void loadWorktreeDiff(activeWorktreeId, false);
      if (mode === "history") {
        void loadCommitHistory(activeWorktreeId, true);
      }
    }, 8000);

    return () => {
      window.clearInterval(timer);
    };
  }, [activeWorktreeId, enabled, mode]);

  useEffect(() => {
    if (!enabled || !activeWorktreeId || !diffState.data?.isGitRepo || mode !== "history") {
      return;
    }
    if (historyState.loading || historyState.commits.length > 0) {
      return;
    }
    void loadCommitHistory(activeWorktreeId, true);
  }, [
    activeWorktreeId,
    diffState.data?.isGitRepo,
    enabled,
    historyState.commits.length,
    historyState.loading,
    mode,
  ]);

  useEffect(() => {
    if (!enabled || !diffState.data || diffState.data.worktreeId !== activeWorktreeId) {
      return;
    }

    const targets = buildDiffTargets(diffState.data);
    if (targets.length === 0) {
      setOpenDiffTargets([]);
      setFileDiffStates({});
      return;
    }

    setOpenDiffTargets((current) =>
      current.filter((openTarget) =>
        targets.some((target) => isSameDiffTarget(target, openTarget)),
      ),
    );
  }, [activeWorktreeId, diffState.data, enabled]);

  useEffect(() => {
    if (
      !enabled ||
      !activeWorktreeId ||
      !diffState.data ||
      diffState.data.worktreeId !== activeWorktreeId ||
      openDiffTargets.length === 0
    ) {
      return;
    }

    let cancelled = false;

    for (const target of openDiffTargets) {
      const cacheKey = diffTargetKey(activeWorktreeId, target);
      const cached = fileDiffCacheRef.current.get(cacheKey);

      if (cached) {
        setFileDiffStates((current) => {
          const existing = current[cacheKey];
          if (
            existing &&
            !existing.loading &&
            !existing.error &&
            existing.data === cached
          ) {
            return current;
          }

          return {
            ...current,
            [cacheKey]: {
              data: cached,
              error: null,
              loading: false,
            },
          };
        });
        continue;
      }

      setFileDiffStates((current) => {
        const existing = current[cacheKey];
        if (existing?.loading) {
          return current;
        }

        return {
          ...current,
          [cacheKey]: {
            data: existing?.data ?? null,
            error: null,
            loading: true,
          },
        };
      });

      void (async () => {
        try {
          const data = await getFileDiff(
            activeWorktreeId,
            target.path,
            target.staged,
          );
          if (cancelled) {
            return;
          }

          fileDiffCacheRef.current.set(cacheKey, data);
          setFileDiffStates((current) => ({
            ...current,
            [cacheKey]: {
              data,
              error: null,
              loading: false,
            },
          }));
        } catch (error) {
          if (cancelled) {
            return;
          }

          setFileDiffStates((current) => ({
            ...current,
            [cacheKey]: {
              data: current[cacheKey]?.data ?? null,
              error: String(error),
              loading: false,
            },
          }));
        }
      })();
    }

    return () => {
      cancelled = true;
    };
  }, [activeWorktreeId, diffState.data, enabled, openDiffTargets]);

  useEffect(() => {
    if (
      !enabled ||
      mode !== "history" ||
      !activeWorktreeId ||
      !diffState.data?.isGitRepo
    ) {
      return;
    }

    const { baseRef, headRef } = historyState;
    if (!baseRef || !headRef) {
      setHistoryState((current) => ({
        ...current,
        compareState: emptyCommitDiffState,
      }));
      return;
    }

    let cancelled = false;
    const requestId = ++compareRequestIdRef.current;

    setHistoryState((current) => ({
      ...current,
      compareState: {
        data: null,
        error: null,
        loading: true,
      },
    }));

    void (async () => {
      try {
        const data = await getCommitRangeDiff(activeWorktreeId, baseRef, headRef);
        if (
          cancelled ||
          requestId !== compareRequestIdRef.current ||
          currentWorktreeIdRef.current !== activeWorktreeId
        ) {
          return;
        }

        setHistoryState((current) => {
          if (current.baseRef !== baseRef || current.headRef !== headRef) {
            return current;
          }
          return {
            ...current,
            compareState: {
              data,
              error: null,
              loading: false,
            },
          };
        });
      } catch (error) {
        if (
          cancelled ||
          requestId !== compareRequestIdRef.current ||
          currentWorktreeIdRef.current !== activeWorktreeId
        ) {
          return;
        }

        setHistoryState((current) => {
          if (current.baseRef !== baseRef || current.headRef !== headRef) {
            return current;
          }
          return {
            ...current,
            compareState: {
              data: current.compareState.data,
              error: String(error),
              loading: false,
            },
          };
        });
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [
    activeWorktreeId,
    diffState.data?.isGitRepo,
    enabled,
    historyState.baseRef,
    historyState.headRef,
    mode,
  ]);

  const refreshDiffPanel = async () => {
    if (!activeWorktreeId) {
      return;
    }

    fileDiffCacheRef.current.clear();
    await loadWorktreeDiff(activeWorktreeId, true);
    if (mode === "history") {
      await loadCommitHistory(activeWorktreeId, true);
    }
  };

  const withAction = async <T,>(
    action: "branch" | "commit" | "push" | "stage",
    run: () => Promise<T>,
  ) => {
    setBusyAction(action);
    setActionStatus(null);
    try {
      const result = await run();
      return result;
    } catch (error) {
      setActionStatus({
        kind: "error",
        message: String(error),
      });
      return null;
    } finally {
      setBusyAction(null);
    }
  };

  const createBranch = async (branchName: string) => {
    if (!activeWorktreeId) {
      return false;
    }

    const snapshot = await withAction("branch", async () => {
      return await createWorktreeBranch(activeWorktreeId, branchName);
    });
    if (!snapshot) {
      return false;
    }

    onSnapshot(snapshot);
    setActionStatus({
      kind: "success",
      message: `Switched to ${branchName.trim()}.`,
    });
    await loadWorktreeDiff(activeWorktreeId, true);
    if (mode === "history") {
      await loadCommitHistory(activeWorktreeId, false);
    }
    return true;
  };

  const stagePath = async (path: string) => {
    if (!activeWorktreeId || busyPathAction) {
      return;
    }

    setBusyPathAction({ kind: "stagePath", path });
    try {
      await stageWorktreePath(activeWorktreeId, path);
      await loadWorktreeDiff(activeWorktreeId, false);
    } catch (error) {
      setActionStatus({ kind: "error", message: String(error) });
    } finally {
      setBusyPathAction(null);
    }
  };

  const unstagePath = async (path: string) => {
    if (!activeWorktreeId || busyPathAction) {
      return;
    }

    setBusyPathAction({ kind: "unstagePath", path });
    try {
      await unstageWorktreePath(activeWorktreeId, path);
      await loadWorktreeDiff(activeWorktreeId, false);
    } catch (error) {
      setActionStatus({ kind: "error", message: String(error) });
    } finally {
      setBusyPathAction(null);
    }
  };

  const stageAll = async () => {
    if (!activeWorktreeId) {
      return;
    }

    const result = await withAction("stage", async () => {
      return await stageWorktreeAll(activeWorktreeId);
    });
    if (!result) {
      return;
    }

    setActionStatus({
      kind: "success",
      message: result.message,
    });
    await loadWorktreeDiff(activeWorktreeId, true);
  };

  const commitChanges = async (message: string) => {
    if (!activeWorktreeId) {
      return false;
    }

    const result = await withAction("commit", async () => {
      return await commitWorktree(activeWorktreeId, message);
    });
    if (!result) {
      return false;
    }

    setActionStatus({
      kind: "success",
      message: result.message,
    });
    await loadWorktreeDiff(activeWorktreeId, true);
    await loadCommitHistory(activeWorktreeId, false);
    return true;
  };

  const pushChanges = async () => {
    if (!activeWorktreeId) {
      return;
    }

    const result = await withAction("push", async () => {
      return await pushWorktree(activeWorktreeId);
    });
    if (!result) {
      return;
    }

    setActionStatus({
      kind: "success",
      message: result.message,
    });
  };

  const changeMode = (nextMode: DiffMode) => {
    setMode(nextMode);
    if (
      nextMode === "history" &&
      activeWorktreeId &&
      diffState.data?.isGitRepo
    ) {
      void loadCommitHistory(activeWorktreeId, true);
    }
  };

  const toggleDiffTarget = (target: DiffTarget) => {
    setOpenDiffTargets((current) => {
      if (current.some((item) => isSameDiffTarget(item, target))) {
        return current.filter((item) => !isSameDiffTarget(item, target));
      }
      return [...current, target];
    });
  };

  const sections = useMemo<DiffSectionViewModel[]>(() => {
    if (!diffState.data) {
      return [];
    }

    const diffStateForTarget = (target: DiffTarget): FileDiffState => {
      if (!activeWorktreeId) {
        return emptyFileDiffState;
      }
      return (
        fileDiffStates[diffTargetKey(activeWorktreeId, target)] ??
        emptyFileDiffState
      );
    };

    const pathBusyKindFor = (
      path: string,
      expected: DiffFilePathBusyKind,
    ): DiffFilePathBusyKind | null =>
      busyPathAction && busyPathAction.path === path && busyPathAction.kind === expected
        ? expected
        : null;

    return [
      {
        emptyText: "No staged changes.",
        items: diffState.data.staged.map((item) => {
          const target: DiffTarget = {
            kind: "staged",
            path: item.path,
            staged: true,
          };
          return {
            addedLabel: formatDiffDelta(item.added, "+"),
            fileDiffState: diffStateForTarget(target),
            isOpen: openDiffTargets.some((openTarget) =>
              isSameDiffTarget(openTarget, target),
            ),
            key: `staged-${item.path}`,
            path: item.path,
            pathBusy: pathBusyKindFor(item.path, "unstagePath"),
            removedLabel: formatDiffDelta(item.removed, "-"),
            target,
          };
        }),
        key: "staged",
        title: "Staged",
      },
      {
        emptyText: "No unstaged changes.",
        items: diffState.data.unstaged.map((item) => {
          const target: DiffTarget = {
            kind: "unstaged",
            path: item.path,
            staged: false,
          };
          return {
            addedLabel: formatDiffDelta(item.added, "+"),
            fileDiffState: diffStateForTarget(target),
            isOpen: openDiffTargets.some((openTarget) =>
              isSameDiffTarget(openTarget, target),
            ),
            key: `unstaged-${item.path}`,
            path: item.path,
            pathBusy: pathBusyKindFor(item.path, "stagePath"),
            removedLabel: formatDiffDelta(item.removed, "-"),
            target,
          };
        }),
        key: "unstaged",
        title: "Unstaged",
      },
      {
        emptyText: "No untracked files.",
        items: diffState.data.untracked.map((path) => {
          const target: DiffTarget = {
            kind: "untracked",
            path,
            staged: false,
          };
          return {
            addedLabel: null,
            fileDiffState: diffStateForTarget(target),
            isOpen: openDiffTargets.some((openTarget) =>
              isSameDiffTarget(openTarget, target),
            ),
            key: `untracked-${path}`,
            path,
            pathBusy: pathBusyKindFor(path, "stagePath"),
            removedLabel: null,
            target,
          };
        }),
        key: "untracked",
        title: "Untracked",
      },
    ];
  }, [activeWorktreeId, busyPathAction, diffState.data, fileDiffStates, openDiffTargets]);

  const bodyState = useMemo<DiffBodyState>(() => {
    if (!activeWorktreeId) {
      return { kind: "no-worktree" };
    }
    if (diffState.data && diffState.data.worktreeId !== activeWorktreeId) {
      return { kind: "loading" };
    }
    if (diffState.error) {
      return {
        kind: "error",
        message: diffState.error,
      };
    }
    if (diffState.loading && !diffState.data) {
      return { kind: "loading" };
    }
    if (diffState.data && !diffState.data.isGitRepo) {
      return { kind: "not-git" };
    }
    if (diffState.data) {
      const gitBranch = diffState.data.gitBranch || noGitBranchLabel;
      const canPush =
        gitBranch !== "" &&
        gitBranch !== noGitBranchLabel &&
        gitBranch !== detachedHeadLabel;

      return {
        kind: "ready",
        worktreeId: diffState.data.worktreeId,
        mode,
        gitBranch,
        actionStatus,
        busyAction,
        canCommit: diffState.data.staged.length > 0,
        canCreateBranch: true,
        canPush,
        canStageAll:
          diffState.data.unstaged.length > 0 || diffState.data.untracked.length > 0,
        history: historyState,
        sections,
        stagedCount: diffState.data.staged.length,
        unstagedCount: diffState.data.unstaged.length,
        untrackedCount: diffState.data.untracked.length,
      };
    }
    return { kind: "empty" };
  }, [
    activeWorktreeId,
    actionStatus,
    busyAction,
    diffState.data,
    diffState.error,
    diffState.loading,
    historyState,
    mode,
    sections,
  ]);

  const adjustDiffTextZoom = (delta: number) => {
    setDiffTextZoom((current) =>
      Math.min(maxDiffTextZoom, Math.max(minDiffTextZoom, current + delta)),
    );
  };

  const diffPanelStyle = {
    "--diff-text-zoom": String(diffTextZoom / 100),
  } as CSSProperties;

  return {
    bodyState,
    canZoomIn: diffTextZoom < maxDiffTextZoom,
    canZoomOut: diffTextZoom > minDiffTextZoom,
    commitChanges,
    createBranch,
    diffPanelStyle,
    diffTextZoom,
    pushChanges,
    refreshDiffPanel,
    resetDiffTextZoom: () => setDiffTextZoom(100),
    selectHistoryBase: (hash: string) =>
      setHistoryState((current) => ({
        ...current,
        baseRef: current.baseRef === hash ? null : hash,
        compareState: emptyCommitDiffState,
      })),
    selectHistoryHead: (hash: string) =>
      setHistoryState((current) => ({
        ...current,
        headRef: current.headRef === hash ? null : hash,
        compareState: emptyCommitDiffState,
      })),
    setMode: changeMode,
    stageAll,
    stagePath,
    toggleDiffTarget,
    unstagePath,
    zoomIn: () => adjustDiffTextZoom(diffTextZoomStep),
    zoomOut: () => adjustDiffTextZoom(-diffTextZoomStep),
  };
}
