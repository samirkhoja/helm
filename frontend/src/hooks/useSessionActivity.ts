import { useEffect, useRef, useState } from "react";

import type {
  SessionDTO,
  SessionLifecycleEvent,
  SessionOutputEvent,
} from "../types";

type SessionMode = "shell" | "agent";
type SessionActivityPhase = "idle" | "awaiting-output" | "busy";

export type SessionActivityState = {
  mode: SessionMode;
  phase: SessionActivityPhase;
  armedAt: number;
  busyUntil: number;
};

const agentActivityArmTimeoutMs = 60_000;
const agentActivityBusyHoldMs = 2_000;

function sessionModeFor(session: SessionDTO | null | undefined): SessionMode {
  return session?.adapterId === "shell" ? "shell" : "agent";
}

function createSessionActivityState(mode: SessionMode): SessionActivityState {
  return {
    mode,
    phase: "idle",
    armedAt: 0,
    busyUntil: 0,
  };
}

function stripTerminalSequences(data: string) {
  return data
    .replace(/\u001b\][^\u0007]*(\u0007|\u001b\\)/g, "")
    .replace(/\u001b\[[0-9;?]*[ -/]*[@-~]/g, "")
    .replace(/\r/g, "")
    .replace(/[^\S\n]+/g, " ")
    .trim();
}

function isMeaningfulActivityOutput(data: string) {
  const cleaned = stripTerminalSequences(data);
  if (!cleaned) {
    return false;
  }
  return !/^[\s:;,.!?'"`()[\]{}<>|/\\_-]+$/.test(cleaned);
}

export function useSessionActivity(sessions: SessionDTO[]) {
  const [sessionActivity, setSessionActivity] = useState<
    Record<number, SessionActivityState>
  >({});
  const activityTimersRef = useRef(new Map<number, number>());
  const sessionByIdRef = useRef(new Map<number, SessionDTO>());
  const sessionActivityRef = useRef<Record<number, SessionActivityState>>({});

  const clearSessionActivity = (sessionId: number) => {
    window.clearTimeout(activityTimersRef.current.get(sessionId));
    activityTimersRef.current.delete(sessionId);

    setSessionActivity((current) => {
      const existing = current[sessionId];
      if (!existing || existing.phase === "idle") {
        return current;
      }

      return {
        ...current,
        [sessionId]: {
          ...existing,
          phase: "idle",
          armedAt: 0,
          busyUntil: 0,
        },
      };
    });
  };

  const updateSessionMode = (sessionId: number, mode: SessionMode) => {
    window.clearTimeout(activityTimersRef.current.get(sessionId));
    activityTimersRef.current.delete(sessionId);

    if (mode === "shell") {
      setSessionActivity((current) => {
        const existing = current[sessionId];
        const next = createSessionActivityState("shell");
        if (
          existing &&
          existing.mode === next.mode &&
          existing.phase === next.phase &&
          existing.armedAt === next.armedAt &&
          existing.busyUntil === next.busyUntil
        ) {
          return current;
        }

        return {
          ...current,
          [sessionId]: next,
        };
      });
      return;
    }

    const armedAt = Date.now();
    setSessionActivity((current) => ({
      ...current,
      [sessionId]: {
        ...(current[sessionId] ?? createSessionActivityState("agent")),
        mode: "agent",
        phase: "awaiting-output",
        armedAt,
        busyUntil: 0,
      },
    }));
    scheduleSessionActivityReset(sessionId, agentActivityArmTimeoutMs);
  };

  const scheduleSessionActivityReset = (sessionId: number, delayMs: number) => {
    window.clearTimeout(activityTimersRef.current.get(sessionId));
    activityTimersRef.current.set(
      sessionId,
      window.setTimeout(() => {
        clearSessionActivity(sessionId);
      }, delayMs),
    );
  };

  const markSessionBusy = (sessionId: number, holdMs: number) => {
    const busyUntil = Date.now() + holdMs;

    setSessionActivity((current) => {
      const existing = current[sessionId];
      if (!existing || existing.mode !== "agent") {
        return current;
      }

      return {
        ...current,
        [sessionId]: {
          ...existing,
          phase: "busy",
          busyUntil,
        },
      };
    });

    scheduleSessionActivityReset(sessionId, holdMs);
  };

  const armSessionActivity = (sessionId: number) => {
    const activity = sessionActivityRef.current[sessionId];
    const session = sessionByIdRef.current.get(sessionId);
    const mode = activity?.mode ?? sessionModeFor(session);
    if ((!session && !activity) || mode !== "agent") {
      return;
    }

    const armedAt = Date.now();
    setSessionActivity((current) => {
      const existing = current[sessionId];
      return {
        ...current,
        [sessionId]: {
          ...(existing ?? createSessionActivityState("agent")),
          mode: "agent",
          phase: "awaiting-output",
          armedAt,
          busyUntil: 0,
        },
      };
    });

    scheduleSessionActivityReset(sessionId, agentActivityArmTimeoutMs);
  };

  const handleSessionOutput = (payload: SessionOutputEvent) => {
    const activity = sessionActivityRef.current[payload.sessionId];
    if (!activity || activity.mode !== "agent" || activity.phase === "idle") {
      return;
    }
    if (!isMeaningfulActivityOutput(payload.data)) {
      return;
    }

    markSessionBusy(payload.sessionId, agentActivityBusyHoldMs);
  };

  const handleSessionLifecycle = (payload: SessionLifecycleEvent) => {
    clearSessionActivity(payload.sessionId);
  };

  useEffect(() => {
    sessionByIdRef.current = new Map(
      sessions.map((session) => [session.id, session]),
    );
  }, [sessions]);

  useEffect(() => {
    sessionActivityRef.current = sessionActivity;
  }, [sessionActivity]);

  useEffect(() => {
    const activeSessionIds = new Set(sessions.map((session) => session.id));

    setSessionActivity((current) => {
      let changed = false;
      const next: Record<number, SessionActivityState> = {};

      for (const session of sessions) {
        const sessionId = session.id;
        const mode = sessionModeFor(session);
        const existing = current[sessionId];

        if (!existing) {
          next[sessionId] = createSessionActivityState(mode);
          changed = true;
          continue;
        }

        if (existing.mode !== mode) {
          next[sessionId] = createSessionActivityState(mode);
          changed = true;
          continue;
        }

        next[sessionId] = existing;
      }

      for (const key of Object.keys(current)) {
        const sessionId = Number(key);
        if (!activeSessionIds.has(sessionId)) {
          window.clearTimeout(activityTimersRef.current.get(sessionId));
          activityTimersRef.current.delete(sessionId);
          changed = true;
        }
      }

      return changed ? next : current;
    });
  }, [sessions]);

  useEffect(() => {
    return () => {
      for (const timer of activityTimersRef.current.values()) {
        window.clearTimeout(timer);
      }
      activityTimersRef.current.clear();
    };
  }, []);

  return {
    armSessionActivity,
    handleSessionModeChange: updateSessionMode,
    handleSessionLifecycle,
    handleSessionOutput,
    sessionActivity,
  };
}
