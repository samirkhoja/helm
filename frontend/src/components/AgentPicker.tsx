import { AgentIcon } from "./icons";
import type { AgentDTO, WorkspaceChoice } from "../types";

interface AgentPickerProps {
  target: WorkspaceChoice | null;
  agents: AgentDTO[];
  defaultAgentId: string;
  submitting: boolean;
  onClose: () => void;
  onSelect: (agentId: string) => void;
}

function moveDefaultFirst(agents: AgentDTO[], defaultAgentId: string) {
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

export function AgentPicker(props: AgentPickerProps) {
  const { target, agents, defaultAgentId, submitting, onClose, onSelect } = props;

  if (!target) {
    return null;
  }

  const orderedAgents = moveDefaultFirst(agents, defaultAgentId);
  const heading = "Choose an agent";
  const workspaceName = target.name;

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="agent-picker"
        role="dialog"
        aria-modal="true"
        aria-labelledby="agent-picker-title"
        onClick={(event) => event.stopPropagation()}
        >
        <div className="agent-picker__header">
          <div>
            <div className="eyebrow">New workspace</div>
            <h2 id="agent-picker-title">{heading}</h2>
            <p>{workspaceName}</p>
          </div>
          <button className="ghost-button" type="button" onClick={onClose}>
            Close
          </button>
        </div>

        <div className="agent-picker__list">
          {orderedAgents.map((agent) => (
            <button
              key={agent.id}
              className="agent-option"
              disabled={submitting}
              type="button"
              onClick={() => onSelect(agent.id)}
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
  );
}
