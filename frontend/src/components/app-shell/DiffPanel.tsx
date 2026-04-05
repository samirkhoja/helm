import type {
  DiffBodyState,
  DiffTarget,
  FileDiffState,
} from "../../hooks/useDiffPanel";

type DiffPanelProps = {
  bodyState: DiffBodyState;
  onToggleDiffTarget: (target: DiffTarget) => void;
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

function renderInlineDiff(fileDiffState: FileDiffState) {
  if (fileDiffState.error) {
    return <div className="diff-inline__empty is-error">{fileDiffState.error}</div>;
  }

  if (fileDiffState.loading && !fileDiffState.data) {
    return <div className="diff-inline__empty">Loading file diff...</div>;
  }

  const patch = fileDiffState.data?.patch || fileDiffState.data?.message;
  if (!patch) {
    return <div className="diff-inline__empty">No inline diff available.</div>;
  }

  return (
    <div className="diff-inline__patch">
      <div className="diff-inline__content">
        {patch.split("\n").map((line, index) => (
          <div className={diffLineClass(line)} key={`${index}-${line}`}>
            {line || " "}
          </div>
        ))}
      </div>
    </div>
  );
}

export function DiffPanel(props: DiffPanelProps) {
  const { bodyState, onToggleDiffTarget } = props;

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
    case "ready":
      return (
        <div className="diff-panel__body">
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
        </div>
      );
  }
}
