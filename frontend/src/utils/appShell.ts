import type {
  AppSnapshot,
  RepoDTO,
  SessionDTO,
  WorktreeDTO,
  WorkspaceChoice,
} from "../types";

export type UtilityPanelTab = "diff" | "files" | "peers";

export type MenuAction =
  | "new-workspace"
  | "new-session"
  | "close-session"
  | "command-palette"
  | "save-file-editor"
  | "toggle-sidebar"
  | "toggle-diff"
  | "toggle-files"
  | "toggle-peers"
  | "toggle-diff-fullscreen"
  | "focus-terminal"
  | "focus-files-panel"
  | "zoom-out-terminal"
  | "reset-terminal-zoom"
  | "zoom-in-terminal"
  | "refresh-diff"
  | "zoom-out-diff"
  | "reset-diff-zoom"
  | "zoom-in-diff"
  | "previous-session"
  | "next-session"
  | "previous-session-alt"
  | "next-session-alt"
  | "dismiss-overlay";

export type SessionLabel = {
  label: string;
  fullLabel: string;
};

export type WorktreeMeta = {
  label: string;
  fullLabel: string;
};

export type VisibleSession = {
  worktree: WorktreeDTO;
  session: SessionDTO;
};

function normalizedWorktreeSessions(worktree: WorktreeDTO) {
  if (!Array.isArray(worktree.sessions)) {
    return [];
  }
  return worktree.sessions.filter(
    (session): session is SessionDTO => Boolean(session),
  );
}

export function flattenSessions(repos: RepoDTO[]) {
  return repos.flatMap((repo) =>
    repo.worktrees.flatMap((worktree) => normalizedWorktreeSessions(worktree)),
  );
}

export function sessionCycleTarget(
  snapshot: AppSnapshot | null,
  sessions: SessionDTO[],
  direction: 1 | -1,
) {
  if (!snapshot || snapshot.activeSessionId === 0 || sessions.length < 2) {
    return null;
  }

  const currentIndex = sessions.findIndex(
    (session) => session.id === snapshot.activeSessionId,
  );
  if (currentIndex < 0) {
    return null;
  }

  const nextIndex =
    (currentIndex + direction + sessions.length) % sessions.length;
  return sessions[nextIndex];
}

export function isEditableTarget(target: EventTarget | null) {
  const element = target instanceof HTMLElement ? target : null;
  if (!element) {
    return false;
  }
  if (element.isContentEditable) {
    return true;
  }
  const tagName = element.tagName.toLowerCase();
  return (
    tagName === "input" ||
    tagName === "textarea" ||
    tagName === "select" ||
    element.closest("[contenteditable='true']") !== null
  );
}

export function defaultAgentId(snapshot: AppSnapshot | null) {
  if (!snapshot || snapshot.availableAgents.length === 0) {
    return "";
  }

  if (snapshot.lastUsedAgentId) {
    const match = snapshot.availableAgents.find(
      (agent) => agent.id === snapshot.lastUsedAgentId,
    );
    if (match) {
      return match.id;
    }
  }

  const shell = snapshot.availableAgents.find((agent) => agent.id === "shell");
  return shell?.id ?? snapshot.availableAgents[0]?.id ?? "";
}

function normalizePath(value: string) {
  return value.replace(/\\/g, "/").replace(/\/+$/, "");
}

function relativePath(basePath: string, targetPath: string) {
  const normalizedBase = normalizePath(basePath);
  const normalizedTarget = normalizePath(targetPath);
  if (!normalizedBase || !normalizedTarget) {
    return normalizedTarget || normalizedBase;
  }
  if (normalizedTarget === normalizedBase) {
    return ".";
  }
  const withSlash = `${normalizedBase}/`;
  if (normalizedTarget.startsWith(withSlash)) {
    return normalizedTarget.slice(withSlash.length) || ".";
  }
  return normalizedTarget;
}

export function trimPathLabel(value: string, segmentLimit = 3) {
  const normalized = normalizePath(value);
  if (!normalized) {
    return ".";
  }

  const isAbsolute = normalized.startsWith("/");
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length <= segmentLimit) {
    return isAbsolute ? `/${segments.join("/")}` : segments.join("/");
  }
  return `.../${segments.slice(-segmentLimit).join("/")}`;
}

export function repoVisibleSessions(repo: RepoDTO): VisibleSession[] {
  return repo.worktrees.flatMap((worktree) =>
    normalizedWorktreeSessions(worktree).map((session) => ({
      worktree,
      session,
    })),
  );
}

export function describeSessionLabel(
  session: SessionDTO,
  worktree: WorktreeDTO | null,
  cwdPath: string | null,
): SessionLabel {
  if (!worktree) {
    return {
      fullLabel: session.title,
      label: session.title,
    };
  }

  const relativeCwd = relativePath(worktree.rootPath, cwdPath ?? worktree.rootPath);
  if (!relativeCwd) {
    return {
      fullLabel: session.title,
      label: session.title,
    };
  }

  if (relativeCwd === ".") {
    return {
      fullLabel: `${session.label} • ${worktree.rootPath}`,
      label: worktree.name,
    };
  }

  return {
    fullLabel: `${session.label} • ${cwdPath ?? worktree.rootPath}`,
    label: trimPathLabel(relativeCwd),
  };
}

export function describeWorktreeMeta(
  repo: RepoDTO | null,
  worktree: WorktreeDTO | null,
): WorktreeMeta {
  if (!repo || !worktree) {
    return {
      label: "",
      fullLabel: "",
    };
  }

  const relativeRoot =
    relativePath(repo.rootPath, worktree.rootPath) ?? worktree.rootPath;
  const pathLabel =
    relativeRoot === "." ? "repo root" : trimPathLabel(relativeRoot, 2);

  return {
    label: `${worktree.gitBranch} • ${pathLabel}`,
    fullLabel: `${worktree.gitBranch} • ${worktree.rootPath}`,
  };
}

function sanitizeBranchName(value: string) {
  return value
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^A-Za-z0-9._/-]+/g, "-")
    .replace(/\/{2,}/g, "/")
    .replace(/^-+|-+$/g, "");
}

export function suggestWorktreePath(repoRoot: string, branchName: string) {
  const normalizedRoot = normalizePath(repoRoot);
  if (!normalizedRoot) {
    return "";
  }

  const parentPath = normalizedRoot.split("/").slice(0, -1).join("/") || "/";
  const pathSegments = normalizedRoot.split("/").filter(Boolean);
  const repoName = pathSegments[pathSegments.length - 1] ?? "repo";
  const safeBranchName = sanitizeBranchName(branchName).replace(/\//g, "-") || "worktree";
  return `${parentPath}/${repoName}-${safeBranchName}`;
}

export function nextWorktreeDefaults(
  repo: RepoDTO,
  activeWorktree: WorktreeDTO | null,
) {
  const activeBranch = activeWorktree?.gitBranch;
  const hasReusableBranch =
    activeBranch &&
    activeBranch !== "No git branch" &&
    activeBranch !== "HEAD" &&
    activeBranch !== "Detached HEAD";

  const branchName =
    hasReusableBranch ? `${sanitizeBranchName(activeBranch)}-agent` : "feature/agent-run";

  const sourceRef =
    hasReusableBranch ? activeBranch : "HEAD";

  return {
    branchName,
    sourceRef,
  };
}

export type WorkspacePickerState = {
  workspace: WorkspaceChoice;
  defaultAgentId: string;
};

export type SessionLauncherState = {
  repo: RepoDTO;
  defaultAgentId: string;
  defaultBranchName: string;
  defaultSourceRef: string;
  defaultWorktreeId: number;
};
