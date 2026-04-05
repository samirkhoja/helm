import { FolderPlusIcon } from "../icons";

type EmptySessionStateProps = {
  onOpenWorkspace: () => void;
};

export function EmptySessionState(props: EmptySessionStateProps) {
  const { onOpenWorkspace } = props;

  return (
    <div className="empty-state">
      <div className="empty-state__copy">
        <div className="eyebrow">Get started</div>
        <h2>No session yet</h2>
        <p>Start with a new workspace</p>
        <button
          className="primary-button"
          type="button"
          onClick={onOpenWorkspace}
        >
          <FolderPlusIcon className="button-icon" size={16} />
          <span>New workspace</span>
        </button>
      </div>
    </div>
  );
}
