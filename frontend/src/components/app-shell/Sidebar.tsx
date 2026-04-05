import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Plus,
  X,
} from "lucide-react";

import { AgentIcon, FolderPlusIcon, SpinnerIcon } from "../icons";

export type SidebarSessionViewModel = {
  adapterId: string;
  fullLabel: string;
  id: number;
  isActive: boolean;
  isBusy: boolean;
  label: string;
  metaLabel: string;
  metaTitle: string;
  outstandingPeerCount: number;
  title: string;
};

export type SidebarRepoViewModel = {
  id: number;
  isCollapsed: boolean;
  name: string;
  persistenceKey: string;
  sessions: SidebarSessionViewModel[];
};

type SidebarProps = {
  onActivateSession: (sessionId: number) => void;
  onCloseSession: (sessionId: number) => void;
  onOpenSession: (repoId: number) => void;
  onOpenWorkspace: () => void;
  onToggleRepo: (repoKey: string) => void;
  onToggleSidebar: () => void;
  open: boolean;
  repos: SidebarRepoViewModel[];
};

export function Sidebar(props: SidebarProps) {
  const {
    onActivateSession,
    onCloseSession,
    onOpenSession,
    onOpenWorkspace,
    onToggleRepo,
    onToggleSidebar,
    open,
    repos,
  } = props;

  return (
    <aside className={`sidebar${open ? "" : " is-collapsed"}`}>
      <div className="sidebar__top">
        <button
          aria-label={open ? "Collapse sidebar" : "Expand sidebar"}
          className="icon-button sidebar-toggle"
          type="button"
          onClick={onToggleSidebar}
        >
          {open ? (
            <ChevronLeft aria-hidden="true" size={15} strokeWidth={1.8} />
          ) : (
            <ChevronRight aria-hidden="true" size={15} strokeWidth={1.8} />
          )}
        </button>
        <button
          className="sidebar__new-workspace"
          type="button"
          onClick={onOpenWorkspace}
        >
          <FolderPlusIcon className="button-icon" size={15} />
          <span>New workspace</span>
        </button>
      </div>

      <div className="sidebar__groups">
        {repos.map((repo) => (
          <section className="repo-group" key={repo.id}>
            <div className="repo-row">
              <button
                className="repo-row__main"
                type="button"
                onClick={() => onToggleRepo(repo.persistenceKey)}
              >
                <span className="repo-row__chevron">
                  {repo.isCollapsed ? (
                    <ChevronRight aria-hidden="true" size={12} strokeWidth={1.8} />
                  ) : (
                    <ChevronDown aria-hidden="true" size={12} strokeWidth={1.8} />
                  )}
                </span>
                <span className="repo-row__content">
                  <strong>{repo.name}</strong>
                  <small>
                    {repo.sessions.length} session
                    {repo.sessions.length === 1 ? "" : "s"}
                  </small>
                </span>
              </button>

              <button
                aria-label={`New session in ${repo.name}`}
                className="icon-button repo-row__action"
                type="button"
                onClick={() => onOpenSession(repo.id)}
              >
                <Plus aria-hidden="true" size={20} strokeWidth={1.95} />
              </button>
            </div>

            {!repo.isCollapsed ? (
              <div className="session-list">
                {repo.sessions.map((session) => (
                  <div className="session-entry" key={session.id}>
                    <button
                      className={`session-row${session.isActive ? " is-active" : ""}`}
                      title={`${session.fullLabel}\n${session.metaTitle}`}
                      type="button"
                      onClick={() => onActivateSession(session.id)}
                    >
                      {session.isBusy ? (
                        <SpinnerIcon className="session-row__spinner" size={12} />
                      ) : null}
                      <AgentIcon
                        agentId={session.adapterId}
                        className="session-row__icon"
                        size={14}
                      />
                      <span className="session-row__content">
                        <span className="session-row__label">{session.label}</span>
                        <small className="session-row__meta">
                          {session.metaLabel}
                        </small>
                      </span>
                      {session.outstandingPeerCount > 0 ? (
                        <span className="session-row__peer-badge">
                          {session.outstandingPeerCount}
                        </span>
                      ) : null}
                    </button>

                    <button
                      aria-label={`Close ${session.title} session`}
                      className="icon-button session-entry__close"
                      title={`Close ${session.title}`}
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        onCloseSession(session.id);
                      }}
                    >
                      <X aria-hidden="true" size={20} strokeWidth={1.95} />
                    </button>
                  </div>
                ))}
              </div>
            ) : null}
          </section>
        ))}
      </div>
    </aside>
  );
}
