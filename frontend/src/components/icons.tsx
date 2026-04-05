import type { CSSProperties } from "react";
import type { LucideIcon, LucideProps } from "lucide-react";
import {
  Bot,
  FileCode2,
  FolderPlus,
  GitCompareArrows,
  LoaderCircle,
  Network,
  SquareTerminal,
} from "lucide-react";

import claudeCodeLogo from "../assets/images/claudecode-color.svg";
import cursorLogo from "../assets/images/cursor.svg";
import geminiLogo from "../assets/images/gemini-color.svg";
import githubCopilotLogo from "../assets/images/githubcopilot.svg";
import openAILogo from "../assets/images/openai.svg";
import openCodeLogo from "../assets/images/opencode.svg";

type IconProps = {
  className?: string;
  size?: number;
  style?: CSSProperties;
};

function joinClassNames(...values: Array<string | undefined>) {
  return values.filter(Boolean).join(" ");
}

function renderIcon(
  Icon: LucideIcon,
  props: IconProps,
  defaults?: Partial<LucideProps>,
) {
  const {
    className,
    size = 16,
    style,
    ...rest
  } = props;

  return (
    <Icon
      aria-hidden="true"
      absoluteStrokeWidth
      className={className}
      size={size}
      strokeWidth={defaults?.strokeWidth ?? 1.6}
      style={style}
      {...defaults}
      {...rest}
    />
  );
}

function renderBrandIcon(src: string, props: IconProps) {
  const {
    className,
    size = 16,
    style,
  } = props;

  const imageStyle: CSSProperties = {
    width: size,
    height: size,
    display: "block",
    flex: "0 0 auto",
    ...style,
  };

  return (
    <img
      alt=""
      aria-hidden="true"
      className={joinClassNames(className, "agent-brand-icon")}
      draggable={false}
      src={src}
      style={imageStyle}
    />
  );
}

export function FolderPlusIcon(props: IconProps) {
  return renderIcon(FolderPlus, props);
}

export function DiffPanelIcon(props: IconProps) {
  return renderIcon(GitCompareArrows, props, { strokeWidth: 1.65 });
}

export function FilesPanelIcon(props: IconProps) {
  return renderIcon(FileCode2, props, { strokeWidth: 1.65 });
}

export function PeersIcon(props: IconProps) {
  return renderIcon(Network, props, { strokeWidth: 1.65 });
}

export function TerminalIcon(props: IconProps) {
  return renderIcon(SquareTerminal, props);
}

export function CodexIcon(props: IconProps) {
  return renderBrandIcon(openAILogo, props);
}

export function ClaudeIcon(props: IconProps) {
  return renderBrandIcon(claudeCodeLogo, props);
}

export function GeminiIcon(props: IconProps) {
  return renderBrandIcon(geminiLogo, props);
}

export function GitHubCopilotIcon(props: IconProps) {
  return renderBrandIcon(githubCopilotLogo, props);
}

export function CursorIcon(props: IconProps) {
  return renderBrandIcon(cursorLogo, props);
}

export function KiroIcon(props: IconProps) {
  return renderIcon(Bot, props);
}

export function AiderIcon(props: IconProps) {
  return renderIcon(Bot, props);
}

export function OpenCodeIcon(props: IconProps) {
  return renderBrandIcon(openCodeLogo, props);
}

export function SpinnerIcon(props: IconProps) {
  return renderIcon(LoaderCircle, props);
}

export function AgentIcon(props: IconProps & { agentId: string }) {
  const { agentId, ...rest } = props;

  switch (agentId) {
    case "shell":
      return <TerminalIcon {...rest} />;
    case "aider":
      return <AiderIcon {...rest} />;
    case "codex":
      return <CodexIcon {...rest} />;
    case "claude-code":
      return <ClaudeIcon {...rest} />;
    case "cursor-agent":
      return <CursorIcon {...rest} />;
    case "gemini":
      return <GeminiIcon {...rest} />;
    case "github-copilot":
      return <GitHubCopilotIcon {...rest} />;
    case "kiro":
      return <KiroIcon {...rest} />;
    case "opencode":
      return <OpenCodeIcon {...rest} />;
    default:
      return renderIcon(Bot, rest);
  }
}
