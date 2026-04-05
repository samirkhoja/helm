import { useEffect, useRef, useState } from "react";

import { listWorktreeEntries } from "../backend";
import type { FileDirectoryState } from "./filesPanelTypes";

const rootDirectoryKey = ".";
const emptyDirectoryState: FileDirectoryState = {
  error: null,
  items: [],
  loaded: false,
  loading: false,
};

function normalizeDirectoryKey(path: string) {
  const trimmed = path.trim();
  return trimmed === "" || trimmed === "." ? rootDirectoryKey : trimmed;
}

function hasPathValue(path: string, list: string[]) {
  return list.includes(path);
}

type UseFileBrowserTreeOptions = {
  activeWorktreeId: number | null;
  enabled: boolean;
};

export function useFileBrowserTree(options: UseFileBrowserTreeOptions) {
  const { activeWorktreeId, enabled } = options;

  const [directoryStates, setDirectoryStates] = useState<
    Record<string, FileDirectoryState>
  >({});
  const [expandedDirectoryPaths, setExpandedDirectoryPaths] = useState<string[]>(
    [],
  );

  const currentWorktreeIdRef = useRef<number | null>(activeWorktreeId);
  const directoryStatesRef = useRef<Record<string, FileDirectoryState>>({});
  const pendingDirectoryLoadsRef = useRef(new Set<string>());

  useEffect(() => {
    directoryStatesRef.current = directoryStates;
  }, [directoryStates]);

  useEffect(() => {
    if (currentWorktreeIdRef.current === activeWorktreeId) {
      return;
    }

    currentWorktreeIdRef.current = activeWorktreeId;
    directoryStatesRef.current = {};
    pendingDirectoryLoadsRef.current.clear();
    setDirectoryStates({});
    setExpandedDirectoryPaths([]);
  }, [activeWorktreeId]);

  const loadDirectory = async (path: string) => {
    if (!activeWorktreeId) {
      return;
    }

    const directoryKey = normalizeDirectoryKey(path);
    const worktreeId = activeWorktreeId;
    const existing = directoryStatesRef.current[directoryKey];
    if (existing?.loaded || pendingDirectoryLoadsRef.current.has(directoryKey)) {
      return;
    }

    pendingDirectoryLoadsRef.current.add(directoryKey);

    setDirectoryStates((current) => ({
      ...current,
      [directoryKey]: {
        error: null,
        items: current[directoryKey]?.items ?? [],
        loaded: false,
        loading: true,
      },
    }));

    try {
      const items = await listWorktreeEntries(
        worktreeId,
        directoryKey === rootDirectoryKey ? "" : directoryKey,
      );
      if (currentWorktreeIdRef.current !== worktreeId) {
        return;
      }

      setDirectoryStates((current) => ({
        ...current,
        [directoryKey]: {
          error: null,
          items,
          loaded: true,
          loading: false,
        },
      }));
    } catch (error) {
      if (currentWorktreeIdRef.current !== worktreeId) {
        return;
      }

      setDirectoryStates((current) => ({
        ...current,
        [directoryKey]: {
          error: String(error),
          items: current[directoryKey]?.items ?? [],
          loaded: false,
          loading: false,
        },
      }));
    } finally {
      pendingDirectoryLoadsRef.current.delete(directoryKey);
    }
  };

  useEffect(() => {
    if (!enabled || !activeWorktreeId) {
      return;
    }
    void loadDirectory(rootDirectoryKey);
  }, [activeWorktreeId, enabled]);

  const toggleDirectory = (path: string) => {
    const directoryKey = normalizeDirectoryKey(path);
    const isExpanded = hasPathValue(directoryKey, expandedDirectoryPaths);

    setExpandedDirectoryPaths((current) =>
      isExpanded
        ? current.filter((currentPath) => currentPath !== directoryKey)
        : [...current, directoryKey],
    );

    if (!isExpanded) {
      void loadDirectory(directoryKey);
    }
  };

  return {
    directoryStates,
    expandedDirectoryPaths,
    loadDirectory,
    rootDirectoryState: directoryStates[rootDirectoryKey] ?? emptyDirectoryState,
    toggleDirectory,
  };
}
