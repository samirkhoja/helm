export interface AgentDTO {
  id: string;
  label: string;
}

export interface SessionDTO {
  id: number;
  worktreeId: number;
  role: "primary" | "utility-shell";
  adapterId: string;
  label: string;
  title: string;
  status: string;
  cwdPath: string;
  peerId: string;
  peerCapable: boolean;
  peerSummary: string;
  outstandingPeerCount: number;
}

export interface WorktreeDTO {
  id: number;
  repoId: number;
  name: string;
  rootPath: string;
  gitBranch: string;
  isPrimary: boolean;
  sessions: SessionDTO[];
}

export interface RepoDTO {
  id: number;
  name: string;
  rootPath: string;
  gitCommonDir: string;
  isGitRepo: boolean;
  persistenceKey: string;
  worktrees: WorktreeDTO[];
}

export interface AppSnapshot {
  repos: RepoDTO[];
  activeRepoId: number;
  activeWorktreeId: number;
  activeSessionId: number;
  availableAgents: AgentDTO[];
  lastUsedAgentId: string;
}

export interface UIStateDTO {
  sidebarOpen: boolean;
  sidebarWidth: number;
  diffPanelOpen: boolean;
  diffPanelWidth: number;
  terminalFontSize: number;
  utilityPanelTab: "diff" | "files" | "peers" | "shell";
  collapsedRepoKeys: string[];
}

export interface BootstrapResult {
  snapshot: AppSnapshot;
  uiState: UIStateDTO;
  restoreNotice: string;
  peerState: PeerStateDTO;
}

export interface PeerDTO {
  peerId: string;
  sessionId: number;
  adapterId: string;
  adapterFamily: string;
  label: string;
  title: string;
  summary: string;
  repoKey: string;
  worktreeRootPath: string;
  outstandingCount: number;
  unreadCount: number;
  isSelf: boolean;
  lastHeartbeatUnixMs: number;
}

export interface PeerMessageDTO {
  id: number;
  fromPeerId: string;
  toPeerId: string;
  fromLabel: string;
  fromTitle: string;
  replyToId: number;
  body: string;
  status: string;
  createdAtUnixMs: number;
  noticedAtUnixMs: number;
  readAtUnixMs: number;
  ackedAtUnixMs: number;
  failedAtUnixMs: number;
  failureReason: string;
}

export interface PeerStateDTO {
  peers: PeerDTO[];
  messages: PeerMessageDTO[];
}

export interface WorkspaceChoice {
  rootPath: string;
  name: string;
  gitBranch: string;
}

export interface WorktreeCreateRequest {
  mode: string;
  branchName: string;
  sourceRef: string;
  path: string;
  agentId: string;
}

export interface GitFileChange {
  path: string;
  status: string;
  added: number;
  removed: number;
}

export interface WorktreeDiff {
  worktreeId: number;
  rootPath: string;
  gitBranch: string;
  isGitRepo: boolean;
  staged: GitFileChange[];
  unstaged: GitFileChange[];
  untracked: string[];
  stagedPatch: string;
  unstagedPatch: string;
}

export interface FileDiff {
  worktreeId: number;
  path: string;
  staged: boolean;
  patch: string;
  message: string;
}

export interface GitActionResult {
  message: string;
}

export interface GitCommitSummary {
  hash: string;
  shortHash: string;
  subject: string;
  authorName: string;
  authorDate: string;
}

export interface CommitDiff {
  worktreeId: number;
  baseRef: string;
  headRef: string;
  patch: string;
  message: string;
}

export interface WorktreeEntry {
  name: string;
  path: string;
  kind: "directory" | "file";
  expandable: boolean;
}

export interface WorktreeContentMatch {
  path: string;
  line: number;
  column: number;
  preview: string;
}

export interface WorktreeFile {
  path: string;
  content: string;
  versionToken: string;
}

export interface SessionOutputEvent {
  sessionId: number;
  data: string;
}

export interface SessionLifecycleEvent {
  sessionId: number;
  worktreeId: number;
  status: string;
  exitCode: number;
  error?: string;
}
