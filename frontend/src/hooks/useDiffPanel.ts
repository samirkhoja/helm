import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";

import { getFileDiff, getWorktreeDiff } from "../backend";
import type { FileDiff, WorktreeDiff } from "../types";

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

export type DiffSectionItemViewModel = {
  addedLabel: string | null;
  fileDiffState: FileDiffState;
  isOpen: boolean;
  key: string;
  path: string;
  removedLabel: string | null;
  target: DiffTarget;
};

export type DiffSectionViewModel = {
  emptyText: string;
  items: DiffSectionItemViewModel[];
  key: string;
  title: string;
};

export type DiffBodyState =
  | { kind: "no-worktree" }
  | { kind: "error"; message: string }
  | { kind: "loading" }
  | { kind: "not-git" }
  | { kind: "empty" }
  | { kind: "ready"; sections: DiffSectionViewModel[] };

type DiffState = {
  data: WorktreeDiff | null;
  error: string | null;
  loading: boolean;
};

type UseDiffPanelOptions = {
  activeWorktreeId: number | null;
  enabled: boolean;
};

const minDiffTextZoom = 80;
const maxDiffTextZoom = 170;
const diffTextZoomStep = 10;

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

const emptyFileDiffState: FileDiffState = {
  data: null,
  error: null,
  loading: false,
};

export function useDiffPanel(options: UseDiffPanelOptions) {
  const { activeWorktreeId, enabled } = options;

  const [diffState, setDiffState] = useState<DiffState>({
    data: null,
    error: null,
    loading: false,
  });
  const [openDiffTargets, setOpenDiffTargets] = useState<DiffTarget[]>([]);
  const [fileDiffStates, setFileDiffStates] = useState<
    Record<string, FileDiffState>
  >({});
  const [diffTextZoom, setDiffTextZoom] = useState(100);

  const fileDiffCacheRef = useRef(new Map<string, FileDiff>());
  const diffSummarySignatureRef = useRef("");
  const currentWorktreeIdRef = useRef<number | null>(null);

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
      setOpenDiffTargets([]);
      setFileDiffStates({});
      return;
    }

    let cancelled = false;
    const worktreeChanged = currentWorktreeIdRef.current !== activeWorktreeId;
    currentWorktreeIdRef.current = activeWorktreeId;

    if (worktreeChanged) {
      diffSummarySignatureRef.current = "";
      fileDiffCacheRef.current.clear();
      setDiffState({
        data: null,
        error: null,
        loading: true,
      });
      setOpenDiffTargets([]);
      setFileDiffStates({});
    } else {
      setDiffState((current) => ({
        ...current,
        error: null,
        loading: current.data === null,
      }));
    }

    const refreshDiff = async () => {
      try {
        const data = await getWorktreeDiff(activeWorktreeId);
        if (cancelled) {
          return;
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
      } catch (error) {
        if (cancelled) {
          return;
        }

        setDiffState((current) => ({
          data: current.data,
          error: String(error),
          loading: false,
        }));
      }
    };

    void refreshDiff();
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") {
        return;
      }
      void refreshDiff();
    }, 8000);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [activeWorktreeId, enabled]);

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
  }, [diffState.data, enabled]);

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

  const refreshDiffPanel = async () => {
    if (!activeWorktreeId) {
      return;
    }

    fileDiffCacheRef.current.clear();
    setDiffState((current) => ({
      ...current,
      error: null,
      loading: true,
    }));

    try {
      const data = await getWorktreeDiff(activeWorktreeId);
      diffSummarySignatureRef.current = buildDiffSummarySignature(data);
      setDiffState({
        data,
        error: null,
        loading: false,
      });
    } catch (error) {
      setDiffState((current) => ({
        data: current.data,
        error: String(error),
        loading: false,
      }));
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

    return [
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
            removedLabel: formatDiffDelta(item.removed, "-"),
            target,
          };
        }),
        key: "unstaged",
        title: "Unstaged",
      },
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
            removedLabel: formatDiffDelta(item.removed, "-"),
            target,
          };
        }),
        key: "staged",
        title: "Staged",
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
            removedLabel: null,
            target,
          };
        }),
        key: "untracked",
        title: "Untracked",
      },
    ];
  }, [activeWorktreeId, diffState.data, fileDiffStates, openDiffTargets]);

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
      return {
        kind: "ready",
        sections,
      };
    }
    return { kind: "empty" };
  }, [activeWorktreeId, diffState.data, diffState.error, diffState.loading, sections]);

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
    diffPanelStyle,
    diffTextZoom,
    refreshDiffPanel,
    resetDiffTextZoom: () => setDiffTextZoom(100),
    toggleDiffTarget,
    zoomIn: () => adjustDiffTextZoom(diffTextZoomStep),
    zoomOut: () => adjustDiffTextZoom(-diffTextZoomStep),
  };
}
