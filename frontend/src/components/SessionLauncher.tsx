import { useEffect, useMemo, useState } from "react";

import type { AgentDTO, RepoDTO } from "../types";
import { AgentIcon } from "./icons";

export type SessionLaunchSelection =
  | {
      mode: "existing";
      worktreeId: number;
      agentId: string;
    }
  | {
      mode: "new";
      branchName: string;
      agentId: string;
    };

interface SessionLauncherProps {
  agents: AgentDTO[];
  defaultAgentId: string;
  defaultBranchName: string;
  repo: RepoDTO | null;
  defaultWorktreeId: number;
  onClose: () => void;
  onSelect: (selection: SessionLaunchSelection) => void;
  submitting: boolean;
}

function sortAgents(agents: AgentDTO[], defaultAgentId: string) {
  const items = [...agents];
  items.sort((left, right) => {
    if (left.id === defaultAgentId) {
      return -1;
    }
    if (right.id === defaultAgentId) {
      return 1;
    }
    return left.label.localeCompare(right.label);
  });
  return items;
}

function trimPath(value: string) {
  const normalized = value.replace(/\\/g, "/").replace(/\/+$/, "");
  if (!normalized) {
    return ".";
  }
  const segments = normalized.split("/").filter(Boolean);
  if (segments.length <= 3) {
    return normalized.startsWith("/") ? `/${segments.join("/")}` : segments.join("/");
  }
  return `…/${segments.slice(-3).join("/")}`;
}

export function SessionLauncher(props: SessionLauncherProps) {
  const { agents, defaultAgentId, defaultBranchName, repo, defaultWorktreeId, onClose, onSelect, submitting } = props;
  const orderedAgents = useMemo(() => sortAgents(agents, defaultAgentId), [agents, defaultAgentId]);
  const [selectedTarget, setSelectedTarget] = useState(String(defaultWorktreeId));
  const [branchName, setBranchName] = useState(defaultBranchName);

  useEffect(() => {
    setSelectedTarget(String(defaultWorktreeId));
  }, [defaultWorktreeId, repo?.id]);

  useEffect(() => {
    setBranchName(defaultBranchName);
  }, [defaultBranchName, repo?.id]);

  if (!repo) {
    return null;
  }

  const creatingNewWorktree = selectedTarget === "new";

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        aria-labelledby="session-launcher-title"
        aria-modal="true"
        className="modal-card modal-card--wide"
        role="dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-card__header">
          <div>
            <div className="eyebrow">New session</div>
            <h2 id="session-launcher-title">Choose a worktree</h2>
            <p>{repo.name}</p>
          </div>
          <button className="ghost-button" type="button" onClick={onClose}>
            Close
          </button>
        </div>

        <div className="modal-card__section">
          <label className="field">
            <span className="field__label">Worktree</span>
            <select
              className="field__control"
              disabled={submitting}
              value={selectedTarget}
              onChange={(event) => setSelectedTarget(event.target.value)}
            >
              {repo.worktrees.map((worktree) => (
                <option key={worktree.id} value={String(worktree.id)}>
                  {worktree.gitBranch} • {trimPath(worktree.rootPath)}
                </option>
              ))}
              {repo.isGitRepo ? <option value="new">New worktree…</option> : null}
            </select>
          </label>
        </div>

        {creatingNewWorktree ? (
          <div className="modal-card__section">
            <label className="field">
              <span className="field__label">Branch name</span>
              <input
                className="field__control"
                disabled={submitting}
                placeholder="feature/agent-run"
                value={branchName}
                onChange={(event) => setBranchName(event.target.value)}
              />
            </label>
          </div>
        ) : null}

        <div className="modal-card__section">
          <div className="field__label session-launcher__agent-label">Agent</div>
          <div className="agent-picker__list">
            {orderedAgents.map((agent) => (
              <button
                key={agent.id}
                className="agent-option"
                disabled={submitting || (creatingNewWorktree && !branchName.trim())}
                type="button"
                onClick={() =>
                  creatingNewWorktree
                    ? onSelect({
                        mode: "new",
                        branchName,
                        agentId: agent.id,
                      })
                    : onSelect({
                        mode: "existing",
                        worktreeId: Number(selectedTarget),
                        agentId: agent.id,
                      })
                }
              >
                <span className="agent-option__main">
                  <AgentIcon agentId={agent.id} className="agent-option__icon" size={15} />
                  <span>{agent.label}</span>
                </span>
                {agent.id === defaultAgentId ? <small>Default</small> : null}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
