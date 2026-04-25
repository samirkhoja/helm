import { useEffect, useState } from "react";
import { LoaderCircle, Minus, Plus } from "lucide-react";

import type {
  CommitDiffState,
  DiffBodyState,
  DiffMode,
  DiffSectionItemViewModel,
  DiffTarget,
  FileDiffState,
} from "../../hooks/useDiffPanel";

type DiffPanelProps = {
  bodyState: DiffBodyState;
  onChangeMode: (mode: DiffMode) => void;
  onCommit: (message: string) => Promise<boolean>;
  onCreateBranch: (branchName: string) => Promise<boolean>;
  onPush: () => void;
  onSelectHistoryBase: (hash: string) => void;
  onSelectHistoryHead: (hash: string) => void;
  onStageAll: () => void;
  onStagePath: (path: string) => void;
  onToggleDiffTarget: (target: DiffTarget) => void;
  onUnstagePath: (path: string) => void;
};

function diffLineClass(line: string) {
  if (line.startsWith("diff --git")) {
    return "diff-line diff-line--meta";
  }
  if (line.startsWith("index ")) {
    return "diff-line diff-line--meta";
  }
  if (line.startsWith("--- ") || line.startsWith("+++ ")) {
    return "diff-line diff-line--file";
  }
  if (line.startsWith("@@")) {
    return "diff-line diff-line--hunk";
  }
  if (line.startsWith("+")) {
    return "diff-line diff-line--add";
  }
  if (line.startsWith("-")) {
    return "diff-line diff-line--remove";
  }
  return "diff-line";
}

function renderPatchBlock(
  patch: string | null | undefined,
  message: string | null | undefined,
  options: {
    emptyText: string;
    error: string | null;
    loading: boolean;
    loadingText: string;
  },
) {
  if (options.error) {
    return <div className="diff-inline__empty is-error">{options.error}</div>;
  }

  if (options.loading && !patch) {
    return <div className="diff-inline__empty">{options.loadingText}</div>;
  }

  const value = patch || message;
  if (!value) {
    return <div className="diff-inline__empty">{options.emptyText}</div>;
  }

  return (
    <div className="diff-inline__patch">
      <div className="diff-inline__content">
        {value.split("\n").map((line, index) => (
          <div className={diffLineClass(line)} key={`${index}-${line}`}>
            {line || " "}
          </div>
        ))}
      </div>
    </div>
  );
}

function renderInlineDiff(fileDiffState: FileDiffState) {
  return renderPatchBlock(fileDiffState.data?.patch, fileDiffState.data?.message, {
    emptyText: "No inline diff available.",
    error: fileDiffState.error,
    loading: fileDiffState.loading,
    loadingText: "Loading file diff...",
  });
}

function renderCommitDiff(compareState: CommitDiffState) {
  return renderPatchBlock(compareState.data?.patch, compareState.data?.message, {
    emptyText: "Select two commits to compare.",
    error: compareState.error,
    loading: compareState.loading,
    loadingText: "Loading commit diff...",
  });
}

function compactCommitLabel(
  commit:
    | {
        shortHash: string;
        subject: string;
      }
    | undefined,
) {
  if (!commit) {
    return "Pick a commit";
  }
  return `${commit.shortHash} • ${commit.subject}`;
}

function renderFileRowAction(
  item: DiffSectionItemViewModel,
  sectionKey: "staged" | "unstaged" | "untracked",
  onStagePath: (path: string) => void,
  onUnstagePath: (path: string) => void,
) {
  const isStageAction = sectionKey !== "staged";
  const busy = item.pathBusy !== null;
  const label = isStageAction ? `Stage ${item.path}` : `Unstage ${item.path}`;

  return (
    <button
      aria-label={label}
      className="diff-file-row__action"
      disabled={busy}
      title={isStageAction ? "Stage file" : "Unstage file"}
      type="button"
      onClick={(event) => {
        event.stopPropagation();
        if (isStageAction) {
          onStagePath(item.path);
        } else {
          onUnstagePath(item.path);
        }
      }}
    >
      {busy ? (
        <LoaderCircle
          aria-hidden="true"
          className="diff-file-row__spinner"
          size={14}
          strokeWidth={2}
        />
      ) : isStageAction ? (
        <Plus aria-hidden="true" size={14} strokeWidth={2.2} />
      ) : (
        <Minus aria-hidden="true" size={14} strokeWidth={2.2} />
      )}
    </button>
  );
}

export function DiffPanel(props: DiffPanelProps) {
  const {
    bodyState,
    onChangeMode,
    onCommit,
    onCreateBranch,
    onPush,
    onSelectHistoryBase,
    onSelectHistoryHead,
    onStageAll,
    onStagePath,
    onToggleDiffTarget,
    onUnstagePath,
  } = props;

  const [branchFormOpen, setBranchFormOpen] = useState(false);
  const [branchName, setBranchName] = useState("");
  const [commitFormOpen, setCommitFormOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState("");

  const readyMode = bodyState.kind === "ready" ? bodyState.mode : null;
  const readyWorktreeId = bodyState.kind === "ready" ? bodyState.worktreeId : null;

  useEffect(() => {
    if (readyMode !== "changes") {
      setBranchFormOpen(false);
      setCommitFormOpen(false);
    }
  }, [readyMode]);

  useEffect(() => {
    setBranchFormOpen(false);
    setBranchName("");
    setCommitFormOpen(false);
    setCommitMessage("");
  }, [readyWorktreeId]);

  switch (bodyState.kind) {
    case "no-worktree":
      return (
        <div className="diff-panel__empty">
          Open a session to track git changes for that worktree.
        </div>
      );
    case "error":
      return <div className="diff-panel__empty is-error">{bodyState.message}</div>;
    case "loading":
      return <div className="diff-panel__empty">Loading diff...</div>;
    case "not-git":
      return (
        <div className="diff-panel__empty">
          This worktree is not inside a git repository.
        </div>
      );
    case "empty":
      return <div className="diff-panel__empty">No diff data yet.</div>;
    case "ready": {
      const actionBusy = bodyState.busyAction !== null;
      const baseCommit = bodyState.history.commits.find(
        (item) => item.hash === bodyState.history.baseRef,
      );
      const headCommit = bodyState.history.commits.find(
        (item) => item.hash === bodyState.history.headRef,
      );

      return (
        <div className="diff-panel__body">
          <div className="diff-mode-tabs" role="tablist" aria-label="Git diff mode">
            <button
              aria-selected={bodyState.mode === "changes"}
              className={`ghost-button diff-mode-tab${bodyState.mode === "changes" ? " is-active" : ""}`}
              role="tab"
              type="button"
              onClick={() => onChangeMode("changes")}
            >
              Changes
            </button>
            <button
              aria-selected={bodyState.mode === "history"}
              className={`ghost-button diff-mode-tab${bodyState.mode === "history" ? " is-active" : ""}`}
              role="tab"
              type="button"
              onClick={() => onChangeMode("history")}
            >
              History
            </button>
          </div>

          {bodyState.mode === "changes" ? (
            <>
              <section className="diff-rail-card">
                <div className="diff-rail-card__header">
                  <div className="diff-rail-card__titles">
                    <div className="eyebrow">Current branch</div>
                    <strong>{bodyState.gitBranch}</strong>
                  </div>
                  <div className="diff-rail-card__summary">
                    <span className="diff-summary-pill">
                      {bodyState.stagedCount} staged
                    </span>
                    <span className="diff-summary-pill">
                      {bodyState.unstagedCount} unstaged
                    </span>
                    <span className="diff-summary-pill">
                      {bodyState.untrackedCount} untracked
                    </span>
                  </div>
                </div>

                <div className="diff-quick-actions">
                  <button
                    className="ghost-button diff-quick-action"
                    disabled={!bodyState.canCreateBranch || actionBusy}
                    type="button"
                    onClick={() => {
                      setBranchFormOpen((current) => !current);
                      if (commitFormOpen) {
                        setCommitFormOpen(false);
                      }
                    }}
                  >
                    New branch
                  </button>
                  <button
                    className="ghost-button diff-quick-action"
                    disabled={!bodyState.canStageAll || actionBusy}
                    type="button"
                    onClick={() => {
                      void onStageAll();
                    }}
                  >
                    {bodyState.busyAction === "stage" ? "Staging..." : "Stage all"}
                  </button>
                  <button
                    className="ghost-button diff-quick-action"
                    disabled={!bodyState.canCommit || actionBusy}
                    type="button"
                    onClick={() => {
                      setCommitFormOpen((current) => !current);
                      if (branchFormOpen) {
                        setBranchFormOpen(false);
                      }
                    }}
                  >
                    Commit
                  </button>
                  <button
                    className="ghost-button diff-quick-action"
                    disabled={!bodyState.canPush || actionBusy}
                    type="button"
                    onClick={() => {
                      void onPush();
                    }}
                  >
                    {bodyState.busyAction === "push" ? "Pushing..." : "Push"}
                  </button>
                </div>

                {branchFormOpen ? (
                  <form
                    className="diff-inline-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      void (async () => {
                        const success = await onCreateBranch(branchName);
                        if (success) {
                          setBranchName("");
                          setBranchFormOpen(false);
                        }
                      })();
                    }}
                  >
                    <input
                      className="field__control diff-inline-form__input"
                      disabled={actionBusy}
                      placeholder="feature/diff-rail"
                      value={branchName}
                      onChange={(event) => setBranchName(event.target.value)}
                    />
                    <div className="diff-inline-form__actions">
                      <button
                        className="ghost-button diff-inline-form__button"
                        disabled={actionBusy || !branchName.trim()}
                        type="submit"
                      >
                        {bodyState.busyAction === "branch" ? "Creating..." : "Create"}
                      </button>
                      <button
                        className="ghost-button diff-inline-form__button"
                        disabled={actionBusy}
                        type="button"
                        onClick={() => setBranchFormOpen(false)}
                      >
                        Cancel
                      </button>
                    </div>
                  </form>
                ) : null}

                {commitFormOpen ? (
                  <form
                    className="diff-inline-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      void (async () => {
                        const success = await onCommit(commitMessage);
                        if (success) {
                          setCommitMessage("");
                          setCommitFormOpen(false);
                        }
                      })();
                    }}
                  >
                    <input
                      className="field__control diff-inline-form__input"
                      disabled={actionBusy}
                      placeholder="Describe this change"
                      value={commitMessage}
                      onChange={(event) => setCommitMessage(event.target.value)}
                    />
                    <div className="diff-inline-form__actions">
                      <button
                        className="ghost-button diff-inline-form__button"
                        disabled={actionBusy || !commitMessage.trim()}
                        type="submit"
                      >
                        {bodyState.busyAction === "commit" ? "Committing..." : "Commit"}
                      </button>
                      <button
                        className="ghost-button diff-inline-form__button"
                        disabled={actionBusy}
                        type="button"
                        onClick={() => setCommitFormOpen(false)}
                      >
                        Cancel
                      </button>
                    </div>
                  </form>
                ) : null}

                {bodyState.actionStatus ? (
                  <div
                    className={`diff-status-banner${bodyState.actionStatus.kind === "error" ? " is-error" : " is-success"}`}
                  >
                    {bodyState.actionStatus.message}
                  </div>
                ) : null}
              </section>

              {bodyState.sections.map((section) => (
                <section className="diff-section" key={section.key}>
                  <div className="diff-section__header">
                    <strong>{section.title}</strong>
                    <span>{section.items.length}</span>
                  </div>

                  {section.items.length ? (
                    <div className="diff-file-list">
                      {section.items.map((item) => (
                        <div className="diff-file-entry" key={item.key}>
                          <div className="diff-file-entry__row">
                            <button
                              className={`diff-file-row${item.isOpen ? " is-active" : ""}`}
                              type="button"
                              onClick={() => onToggleDiffTarget(item.target)}
                            >
                              <span className="diff-file-row__path">{item.path}</span>
                              {item.addedLabel || item.removedLabel ? (
                                <span className="diff-file-row__stats">
                                  {item.addedLabel ? (
                                    <small className="diff-add">{item.addedLabel}</small>
                                  ) : null}
                                  {item.removedLabel ? (
                                    <small className="diff-remove">{item.removedLabel}</small>
                                  ) : null}
                                </span>
                              ) : null}
                            </button>
                            {renderFileRowAction(
                              item,
                              section.key as "staged" | "unstaged" | "untracked",
                              onStagePath,
                              onUnstagePath,
                            )}
                          </div>
                          {item.isOpen ? (
                            <div className="diff-inline">
                              {renderInlineDiff(item.fileDiffState)}
                            </div>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="diff-section__empty">{section.emptyText}</div>
                  )}
                </section>
              ))}
            </>
          ) : (
            <>
              <section className="diff-rail-card">
                <div className="diff-rail-card__header">
                  <div className="diff-rail-card__titles">
                    <div className="eyebrow">Commit compare</div>
                    <strong>{bodyState.gitBranch}</strong>
                  </div>
                  <div className="diff-rail-card__summary">
                    <span className="diff-summary-pill">From</span>
                    <span className="diff-summary-pill diff-summary-pill--wide">
                      {compactCommitLabel(baseCommit)}
                    </span>
                    <span className="diff-summary-pill">To</span>
                    <span className="diff-summary-pill diff-summary-pill--wide">
                      {compactCommitLabel(headCommit)}
                    </span>
                  </div>
                </div>

                {bodyState.actionStatus ? (
                  <div
                    className={`diff-status-banner${bodyState.actionStatus.kind === "error" ? " is-error" : " is-success"}`}
                  >
                    {bodyState.actionStatus.message}
                  </div>
                ) : null}
              </section>

              <section className="diff-section">
                <div className="diff-section__header">
                  <strong>Recent commits</strong>
                  <span>{bodyState.history.commits.length}</span>
                </div>

                {bodyState.history.loading && bodyState.history.commits.length === 0 ? (
                  <div className="diff-section__empty">Loading history...</div>
                ) : bodyState.history.error ? (
                  <div className="diff-section__empty is-error">
                    {bodyState.history.error}
                  </div>
                ) : bodyState.history.commits.length ? (
                  <div className="diff-history-list">
                    {bodyState.history.commits.map((item) => (
                      <div className="diff-history-entry" key={item.hash}>
                        <div className="diff-history-entry__main">
                          <strong className="diff-history-entry__subject">
                            {item.subject}
                          </strong>
                          <div className="diff-history-entry__meta">
                            {item.shortHash} • {item.authorName} • {item.authorDate}
                          </div>
                        </div>
                        <div className="diff-history-entry__actions">
                          <button
                            className={`ghost-button diff-history-entry__picker${bodyState.history.baseRef === item.hash ? " is-active" : ""}`}
                            type="button"
                            onClick={() => onSelectHistoryBase(item.hash)}
                          >
                            From
                          </button>
                          <button
                            className={`ghost-button diff-history-entry__picker${bodyState.history.headRef === item.hash ? " is-active" : ""}`}
                            type="button"
                            onClick={() => onSelectHistoryHead(item.hash)}
                          >
                            To
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="diff-section__empty">
                    No commits available for this worktree yet.
                  </div>
                )}
              </section>

              <section className="diff-section">
                <div className="diff-section__header">
                  <strong>Range diff</strong>
                  <span>
                    {baseCommit && headCommit
                      ? `${baseCommit.shortHash} → ${headCommit.shortHash}`
                      : "Select two commits"}
                  </span>
                </div>

                <div className="diff-inline">
                  {renderCommitDiff(bodyState.history.compareState)}
                </div>
              </section>
            </>
          )}
        </div>
      );
    }
  }
}
