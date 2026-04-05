import React from "react";
import { createRoot } from "react-dom/client";

import "./style.css";
import App from "./App";

const container = document.getElementById("root");

if (!container) {
  throw new Error("root element was not found");
}

type FatalScreenProps = {
  message: string;
};

type AppErrorBoundaryProps = {
  children: React.ReactNode;
  onError: (message: string) => void;
};

type AppErrorBoundaryState = {
  message: string | null;
};

function formatErrorMessage(value: unknown) {
  if (value instanceof Error) {
    return value.stack || value.message;
  }
  if (typeof value === "string") {
    return value;
  }
  return String(value);
}

function FatalScreen(props: FatalScreenProps) {
  return (
    <div
      style={{
        background: "#0b0c0e",
        color: "#f5f5f4",
        fontFamily:
          '"SF Mono", "JetBrainsMono Nerd Font", "Menlo", "Monaco", monospace',
        inset: 0,
        overflow: "auto",
        padding: "32px",
        position: "fixed",
      }}
    >
      <h1 style={{ fontSize: "18px", margin: "0 0 12px" }}>
        Helm hit a renderer error
      </h1>
      <p style={{ color: "#d6d3d1", lineHeight: 1.5, margin: "0 0 16px" }}>
        Reload the app after copying the message below if it keeps happening.
      </p>
      <pre
        style={{
          background: "#111315",
          border: "1px solid #292524",
          borderRadius: "12px",
          margin: 0,
          padding: "16px",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {props.message}
      </pre>
    </div>
  );
}

class AppErrorBoundary extends React.Component<
  AppErrorBoundaryProps,
  AppErrorBoundaryState
> {
  state: AppErrorBoundaryState = {
    message: null,
  };

  static getDerivedStateFromError(error: unknown): AppErrorBoundaryState {
    return {
      message: formatErrorMessage(error),
    };
  }

  componentDidCatch(error: unknown) {
    this.props.onError(formatErrorMessage(error));
  }

  render() {
    if (this.state.message) {
      return <FatalScreen message={this.state.message} />;
    }
    return this.props.children;
  }
}

function RootApp() {
  const [fatalMessage, setFatalMessage] = React.useState<string | null>(null);

  React.useEffect(() => {
    const handleError = (event: ErrorEvent) => {
      setFatalMessage(formatErrorMessage(event.error ?? event.message));
    };
    const handleRejection = (event: PromiseRejectionEvent) => {
      setFatalMessage(formatErrorMessage(event.reason));
    };

    window.addEventListener("error", handleError);
    window.addEventListener("unhandledrejection", handleRejection);
    return () => {
      window.removeEventListener("error", handleError);
      window.removeEventListener("unhandledrejection", handleRejection);
    };
  }, []);

  if (fatalMessage) {
    return <FatalScreen message={fatalMessage} />;
  }

  return (
    <AppErrorBoundary onError={setFatalMessage}>
      <App />
    </AppErrorBoundary>
  );
}

createRoot(container).render(
  <React.StrictMode>
    <RootApp />
  </React.StrictMode>,
);
