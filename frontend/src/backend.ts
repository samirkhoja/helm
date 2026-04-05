import type {
  BootstrapResult,
  AppSnapshot,
  FileDiff,
  PeerStateDTO,
  UIStateDTO,
  WorkspaceChoice,
  WorktreeCreateRequest,
  WorktreeDiff,
  WorktreeEntry,
  WorktreeFile,
} from "./types";

type AppBinding = {
  Bootstrap(): Promise<BootstrapResult>;
  ChooseWorkspace(): Promise<WorkspaceChoice | null>;
  CreateWorkspaceSession(rootPath: string, agentId: string): Promise<AppSnapshot>;
  CreateSession(worktreeId: number, agentId: string): Promise<AppSnapshot>;
  CreateWorktreeSession(repoId: number, request: WorktreeCreateRequest): Promise<AppSnapshot>;
  ActivateSession(sessionId: number): Promise<AppSnapshot>;
  KillSession(sessionId: number): Promise<AppSnapshot>;
  SendSessionInput(sessionId: number, data: string): Promise<void>;
  ResizeSession(sessionId: number, cols: number, rows: number): Promise<void>;
  UpdateSessionCWD(sessionId: number, cwdPath: string): Promise<void>;
  UpdateSessionMode(sessionId: number, adapterId: string): Promise<AppSnapshot>;
  SaveUIState(uiState: UIStateDTO): Promise<void>;
  DeletePeerMessage(messageId: number): Promise<PeerStateDTO>;
  ClearPeerMessages(): Promise<PeerStateDTO>;
  GetWorktreeDiff(worktreeId: number): Promise<WorktreeDiff>;
  GetFileDiff(worktreeId: number, path: string, staged: boolean): Promise<FileDiff>;
  ListWorktreeFiles(worktreeId: number): Promise<string[]>;
  ListWorktreeEntries(worktreeId: number, relativeDir: string): Promise<WorktreeEntry[]>;
  ReadWorktreeFile(worktreeId: number, relativePath: string): Promise<WorktreeFile>;
  SaveWorktreeFile(worktreeId: number, relativePath: string, content: string, expectedVersion: string): Promise<WorktreeFile>;
};

function backend(): AppBinding {
  const app = (window as Window & { go?: { main?: { App?: AppBinding } } }).go?.main?.App;
  if (!app) {
    throw new Error("Wails bindings are unavailable");
  }
  return app;
}

export function bootstrap() {
  return backend().Bootstrap();
}

export function chooseWorkspace() {
  return backend().ChooseWorkspace();
}

export function createWorkspaceSession(rootPath: string, agentId: string) {
  return backend().CreateWorkspaceSession(rootPath, agentId);
}

export function createSession(worktreeId: number, agentId: string) {
  return backend().CreateSession(worktreeId, agentId);
}

export function createWorktreeSession(repoId: number, request: WorktreeCreateRequest) {
  return backend().CreateWorktreeSession(repoId, request);
}

export function activateSession(sessionId: number) {
  return backend().ActivateSession(sessionId);
}

export function killSession(sessionId: number) {
  return backend().KillSession(sessionId);
}

export function sendSessionInput(sessionId: number, data: string) {
  return backend().SendSessionInput(sessionId, data);
}

export function resizeSession(sessionId: number, cols: number, rows: number) {
  return backend().ResizeSession(sessionId, cols, rows);
}

export function updateSessionCWD(sessionId: number, cwdPath: string) {
  return backend().UpdateSessionCWD(sessionId, cwdPath);
}

export function updateSessionMode(sessionId: number, adapterId: string) {
  return backend().UpdateSessionMode(sessionId, adapterId);
}

export function saveUIState(uiState: UIStateDTO) {
  return backend().SaveUIState(uiState);
}

export function deletePeerMessage(messageId: number) {
  return backend().DeletePeerMessage(messageId);
}

export function clearPeerMessages() {
  return backend().ClearPeerMessages();
}

export function getWorktreeDiff(worktreeId: number) {
  return backend().GetWorktreeDiff(worktreeId);
}

export function getFileDiff(worktreeId: number, path: string, staged: boolean) {
  return backend().GetFileDiff(worktreeId, path, staged);
}

export function listWorktreeFiles(worktreeId: number) {
  return backend().ListWorktreeFiles(worktreeId);
}

export function listWorktreeEntries(worktreeId: number, relativeDir: string) {
  return backend().ListWorktreeEntries(worktreeId, relativeDir);
}

export function readWorktreeFile(worktreeId: number, relativePath: string) {
  return backend().ReadWorktreeFile(worktreeId, relativePath);
}

export function saveWorktreeFile(
  worktreeId: number,
  relativePath: string,
  content: string,
  expectedVersion: string,
) {
  return backend().SaveWorktreeFile(
    worktreeId,
    relativePath,
    content,
    expectedVersion,
  );
}
