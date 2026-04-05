import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
} from "react";

import type { SessionDTO } from "../types";

type TerminalRecord = {
  terminal: Terminal;
  fitAddon: FitAddon;
  resizeObserver: ResizeObserver;
  disposeData: { dispose(): void };
  disposeCwd: { dispose(): void };
  disposeMode: { dispose(): void };
};

const helmSessionModeOsc = 697;

export interface TerminalStageHandle {
  writeOutput: (sessionId: number, data: string) => void;
  focusActive: () => void;
}

interface TerminalStageProps {
  sessions: SessionDTO[];
  activeSessionId: number;
  autoFocusActive: boolean;
  fontSize: number;
  onInput: (sessionId: number, data: string) => void;
  onResize: (sessionId: number, cols: number, rows: number) => void;
  onSessionCwdChange?: (sessionId: number, cwd: string) => void;
  onSessionModeChange?: (sessionId: number, adapterId: string) => void;
}

function parseOsc7Path(data: string) {
  try {
    const url = new URL(data.trim());
    if (url.protocol !== "file:") {
      return null;
    }
    return decodeURIComponent(url.pathname);
  } catch {
    return null;
  }
}

function parseSessionMode(data: string) {
  const cleaned = data.trim();
  if (!cleaned.startsWith("adapter=")) {
    return null;
  }
  const adapterId = cleaned.slice("adapter=".length).trim();
  return adapterId || null;
}

export const TerminalStage = forwardRef<TerminalStageHandle, TerminalStageProps>(function TerminalStage(
  props,
  ref,
) {
  const {
    sessions,
    activeSessionId,
    autoFocusActive,
    fontSize,
    onInput,
    onResize,
    onSessionCwdChange,
    onSessionModeChange,
  } = props;

  const terminalsRef = useRef(new Map<number, TerminalRecord>());
  const hostNodesRef = useRef(new Map<number, HTMLDivElement | null>());
  const pendingWritesRef = useRef(new Map<number, string[]>());
  const pendingWriteCountsRef = useRef(new Map<number, number>());
  const restoreFramesRef = useRef(new Map<number, number>());
  const restoreRequestsRef = useRef(
    new Map<number, { forceFocus: boolean; fit: boolean }>(),
  );
  const resizeTimersRef = useRef(new Map<number, number>());
  const staleViewportSessionsRef = useRef(new Set<number>());
  const scrollRestoreRef = useRef(new Map<number, boolean>());
  const activeSessionIdRef = useRef(activeSessionId);
  const autoFocusActiveRef = useRef(autoFocusActive);
  const inputRef = useRef(onInput);
  const resizeRef = useRef(onResize);
  const cwdChangeRef = useRef(onSessionCwdChange);
  const modeChangeRef = useRef(onSessionModeChange);

  const sessionIds = useMemo(() => sessions.map((session) => session.id), [sessions]);

  useEffect(() => {
    activeSessionIdRef.current = activeSessionId;
  }, [activeSessionId]);

  useEffect(() => {
    autoFocusActiveRef.current = autoFocusActive;
  }, [autoFocusActive]);

  useEffect(() => {
    inputRef.current = onInput;
  }, [onInput]);

  useEffect(() => {
    resizeRef.current = onResize;
  }, [onResize]);

  useEffect(() => {
    cwdChangeRef.current = onSessionCwdChange;
  }, [onSessionCwdChange]);

  useEffect(() => {
    modeChangeRef.current = onSessionModeChange;
  }, [onSessionModeChange]);

  const flushPendingOutput = (sessionId: number) => {
    const pendingOutput = pendingWritesRef.current.get(sessionId);
    if (!pendingOutput || pendingOutput.length === 0) {
      return;
    }
    pendingWritesRef.current.delete(sessionId);
    writeTerminalData(sessionId, pendingOutput.join(""));
  };

  const fitSession = (sessionId: number) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      return;
    }

    const host = hostNodesRef.current.get(sessionId);
    if (!host || host.clientWidth === 0 || host.clientHeight === 0) {
      return;
    }

    terminalRecord.fitAddon.fit();
    window.clearTimeout(resizeTimersRef.current.get(sessionId));
    const timer = window.setTimeout(() => {
      resizeRef.current(sessionId, terminalRecord.terminal.cols, terminalRecord.terminal.rows);
    }, 50);
    resizeTimersRef.current.set(sessionId, timer);
  };

  const scrollSessionToBottom = (sessionId: number) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      return;
    }
    terminalRecord.terminal.scrollToBottom();
  };

  const sessionViewportAtBottom = (sessionId: number) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      return true;
    }
    const { viewportY, baseY } = terminalRecord.terminal.buffer.active;
    return viewportY >= Math.max(baseY - 1, 0);
  };

  const markSessionViewportStale = (sessionId: number) => {
    if (!staleViewportSessionsRef.current.has(sessionId)) {
      scrollRestoreRef.current.set(
        sessionId,
        sessionViewportAtBottom(sessionId),
      );
    }
    staleViewportSessionsRef.current.add(sessionId);
  };

  const pendingScrollRestore = (sessionId: number) => {
    if (!staleViewportSessionsRef.current.has(sessionId)) {
      return false;
    }
    return scrollRestoreRef.current.get(sessionId) ?? true;
  };

  const clearPendingViewportRestore = (sessionId: number) => {
    staleViewportSessionsRef.current.delete(sessionId);
    scrollRestoreRef.current.delete(sessionId);
  };

  const cancelScheduledRestore = (sessionId: number) => {
    const frame = restoreFramesRef.current.get(sessionId);
    if (frame !== undefined) {
      window.cancelAnimationFrame(frame);
      restoreFramesRef.current.delete(sessionId);
    }
    restoreRequestsRef.current.delete(sessionId);
  };

  const pendingTerminalWrites = (sessionId: number) => {
    return pendingWriteCountsRef.current.get(sessionId) ?? 0;
  };

  const completeTerminalWrite = (sessionId: number) => {
    const pending = pendingTerminalWrites(sessionId);
    if (pending <= 1) {
      pendingWriteCountsRef.current.delete(sessionId);
      return;
    }
    pendingWriteCountsRef.current.set(sessionId, pending - 1);
  };

  const restoreSessionViewport = (
    sessionId: number,
    options?: { fit?: boolean; focus?: boolean; scrollToBottom?: boolean },
  ) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      return false;
    }

    if (options?.fit) {
      fitSession(sessionId);
    }
    if (options?.scrollToBottom) {
      scrollSessionToBottom(sessionId);
    }
    terminalRecord.terminal.refresh(
      0,
      Math.max(terminalRecord.terminal.rows - 1, 0),
    );
    if (options?.focus) {
      terminalRecord.terminal.focus();
    }
    return true;
  };

  const restoreSessionIfReady = (
    sessionId: number,
    options?: { fit?: boolean; forceFocus?: boolean },
  ) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      return false;
    }
    if (activeSessionIdRef.current !== sessionId) {
      return false;
    }
    if (document.visibilityState !== "visible" || !document.hasFocus()) {
      return false;
    }

    const shouldFocus = options?.forceFocus || autoFocusActiveRef.current;
    const shouldRestoreViewport = staleViewportSessionsRef.current.has(sessionId);

    if (shouldRestoreViewport && pendingTerminalWrites(sessionId) > 0) {
      if (options?.fit || shouldFocus) {
        restoreSessionViewport(sessionId, {
          fit: options?.fit,
          focus: shouldFocus,
        });
      }
      return false;
    }

    const restored = restoreSessionViewport(sessionId, {
      fit: options?.fit,
      focus: shouldFocus,
      scrollToBottom: shouldRestoreViewport && pendingScrollRestore(sessionId),
    });
    if (restored && shouldRestoreViewport) {
      clearPendingViewportRestore(sessionId);
    }
    return restored;
  };

  const requestSessionRestore = (
    sessionId: number,
    options?: { fit?: boolean; forceFocus?: boolean },
  ) => {
    const existing = restoreRequestsRef.current.get(sessionId) ?? {
      fit: false,
      forceFocus: false,
    };
    restoreRequestsRef.current.set(sessionId, {
      fit: existing.fit || Boolean(options?.fit),
      forceFocus: existing.forceFocus || Boolean(options?.forceFocus),
    });

    if (restoreFramesRef.current.has(sessionId)) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      restoreFramesRef.current.delete(sessionId);
      const request = restoreRequestsRef.current.get(sessionId);
      restoreRequestsRef.current.delete(sessionId);
      restoreSessionIfReady(sessionId, request);
    });
    restoreFramesRef.current.set(sessionId, frame);
  };

  const writeTerminalData = (sessionId: number, data: string) => {
    const terminalRecord = terminalsRef.current.get(sessionId);
    if (!terminalRecord) {
      const previous = pendingWritesRef.current.get(sessionId) ?? [];
      previous.push(data);
      pendingWritesRef.current.set(sessionId, previous);
      return;
    }

    pendingWriteCountsRef.current.set(
      sessionId,
      pendingTerminalWrites(sessionId) + 1,
    );
    terminalRecord.terminal.write(data, () => {
      completeTerminalWrite(sessionId);
      if (
        staleViewportSessionsRef.current.has(sessionId) &&
        pendingTerminalWrites(sessionId) === 0
      ) {
        requestSessionRestore(sessionId);
      }
    });
  };

  const ensureTerminal = (sessionId: number) => {
    if (terminalsRef.current.has(sessionId)) {
      flushPendingOutput(sessionId);
      return;
    }

    const host = hostNodesRef.current.get(sessionId);
    if (!host) {
      return;
    }

    const terminal = new Terminal({
      allowTransparency: true,
      cursorBlink: false,
      cursorInactiveStyle: "outline",
      cursorStyle: "block",
      convertEol: false,
      fontFamily:
        '"SF Mono", "JetBrainsMono Nerd Font", "MesloLGS NF", "Menlo", "Monaco", monospace',
      fontSize,
      lineHeight: 1.25,
      letterSpacing: 0,
      scrollOnUserInput: true,
      scrollback: 10000,
      theme: {
        background: "#0a0b0d",
        foreground: "#e7e5e4",
        cursor: "#78716c",
        cursorAccent: "#0a0b0d",
        selectionBackground: "rgba(115, 115, 115, 0.35)",
        black: "#0a0b0d",
        brightBlack: "#57534e",
        red: "#f87171",
        green: "#4ade80",
        yellow: "#facc15",
        blue: "#93c5fd",
        magenta: "#d8b4fe",
        cyan: "#67e8f9",
        white: "#fafaf9",
        brightWhite: "#ffffff",
      },
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(host);

    const disposeData = terminal.onData((data) => {
      if (activeSessionIdRef.current === sessionId) {
        scrollSessionToBottom(sessionId);
      }
      inputRef.current(sessionId, data);
    });
    const disposeCwd = terminal.parser.registerOscHandler(7, (data) => {
      const cwd = parseOsc7Path(data);
      if (cwd) {
        cwdChangeRef.current?.(sessionId, cwd);
      }
      return true;
    });
    const disposeMode = terminal.parser.registerOscHandler(helmSessionModeOsc, (data) => {
      const adapterId = parseSessionMode(data);
      if (adapterId) {
        if (activeSessionIdRef.current === sessionId) {
          scrollSessionToBottom(sessionId);
        }
        modeChangeRef.current?.(sessionId, adapterId);
      }
      return true;
    });

    const resizeObserver = new ResizeObserver(() => {
      if (activeSessionIdRef.current !== sessionId) {
        return;
      }
      fitSession(sessionId);
    });
    resizeObserver.observe(host);

    terminalsRef.current.set(sessionId, {
      terminal,
      fitAddon,
      resizeObserver,
      disposeData,
      disposeCwd,
      disposeMode,
    });

    flushPendingOutput(sessionId);
  };

  useImperativeHandle(
    ref,
    () => ({
      writeOutput(sessionId: number, data: string) {
        if (
          !terminalsRef.current.has(sessionId) ||
          activeSessionIdRef.current !== sessionId ||
          document.visibilityState !== "visible" ||
          !document.hasFocus()
        ) {
          markSessionViewportStale(sessionId);
        }
        writeTerminalData(sessionId, data);
      },
      focusActive() {
        const sessionId = activeSessionIdRef.current;
        if (!terminalsRef.current.has(sessionId)) {
          return;
        }
        restoreSessionIfReady(sessionId, { forceFocus: true });
      },
    }),
    [],
  );

  useEffect(() => {
    for (const sessionId of sessionIds) {
      ensureTerminal(sessionId);
    }

    const activeSet = new Set(sessionIds);
    for (const [sessionId, terminalRecord] of terminalsRef.current.entries()) {
      if (activeSet.has(sessionId)) {
        continue;
      }
      terminalRecord.disposeData.dispose();
      terminalRecord.disposeCwd.dispose();
      terminalRecord.disposeMode.dispose();
      terminalRecord.resizeObserver.disconnect();
      terminalRecord.terminal.dispose();
      terminalsRef.current.delete(sessionId);
      pendingWritesRef.current.delete(sessionId);
      pendingWriteCountsRef.current.delete(sessionId);
      cancelScheduledRestore(sessionId);
      clearPendingViewportRestore(sessionId);
      hostNodesRef.current.delete(sessionId);
      window.clearTimeout(resizeTimersRef.current.get(sessionId));
      resizeTimersRef.current.delete(sessionId);
    }
  }, [sessionIds]);

  useEffect(() => {
    if (!activeSessionId) {
      return;
    }
    requestSessionRestore(activeSessionId, {
      fit: true,
      forceFocus: autoFocusActive,
    });
    return () => {
      cancelScheduledRestore(activeSessionId);
    };
  }, [activeSessionId, autoFocusActive]);

  useEffect(() => {
    const restoreActiveSession = () => {
      const sessionId = activeSessionIdRef.current;
      if (!sessionId || !autoFocusActiveRef.current) {
        return;
      }
      requestSessionRestore(sessionId, { fit: true, forceFocus: true });
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState !== "visible") {
        return;
      }
      restoreActiveSession();
    };

    window.addEventListener("focus", restoreActiveSession);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.removeEventListener("focus", restoreActiveSession);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  useEffect(() => {
    for (const terminalRecord of terminalsRef.current.values()) {
      if (terminalRecord.terminal.options.fontSize === fontSize) {
        continue;
      }
      terminalRecord.terminal.options.fontSize = fontSize;
    }

    if (!activeSessionIdRef.current) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      fitSession(activeSessionIdRef.current);
    });
    return () => {
      window.cancelAnimationFrame(frame);
    };
  }, [fontSize]);

  useEffect(() => {
    return () => {
      for (const timer of resizeTimersRef.current.values()) {
        window.clearTimeout(timer);
      }
      for (const terminalRecord of terminalsRef.current.values()) {
        terminalRecord.disposeData.dispose();
        terminalRecord.disposeCwd.dispose();
        terminalRecord.disposeMode.dispose();
        terminalRecord.resizeObserver.disconnect();
        terminalRecord.terminal.dispose();
      }
      terminalsRef.current.clear();
      hostNodesRef.current.clear();
      pendingWritesRef.current.clear();
      pendingWriteCountsRef.current.clear();
      for (const frame of restoreFramesRef.current.values()) {
        window.cancelAnimationFrame(frame);
      }
      restoreFramesRef.current.clear();
      restoreRequestsRef.current.clear();
      resizeTimersRef.current.clear();
      staleViewportSessionsRef.current.clear();
      scrollRestoreRef.current.clear();
    };
  }, []);

  return (
    <div className="terminal-stage">
      {sessions.map((session) => (
        <div
          key={session.id}
          className={`terminal-host${session.id === activeSessionId ? " is-active" : ""}`}
          ref={(node) => {
            hostNodesRef.current.set(session.id, node);
            if (node) {
              ensureTerminal(session.id);
            }
          }}
        />
      ))}
    </div>
  );
});
