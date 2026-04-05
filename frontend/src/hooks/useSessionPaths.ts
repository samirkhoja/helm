import { useEffect, useState } from "react";

import { updateSessionCWD } from "../backend";
import type { SessionDTO } from "../types";

export function useSessionPaths(
  sessions: SessionDTO[],
  onError: (error: unknown) => void,
) {
  const [sessionPaths, setSessionPaths] = useState<Record<number, string>>({});

  useEffect(() => {
    const activeSessionIds = new Set(sessions.map((session) => session.id));

    setSessionPaths((current) => {
      let changed = false;
      const next: Record<number, string> = {};

      for (const session of sessions) {
        const sessionId = session.id;
        const persistedPath = session.cwdPath || "";
        const currentPath = current[sessionId];

        if (persistedPath) {
          next[sessionId] = persistedPath;
          if (currentPath !== persistedPath) {
            changed = true;
          }
          continue;
        }

        if (currentPath) {
          next[sessionId] = currentPath;
        }
      }

      for (const key of Object.keys(current)) {
        const sessionId = Number(key);
        if (!activeSessionIds.has(sessionId)) {
          changed = true;
          continue;
        }
        if (!(sessionId in next) && current[sessionId]) {
          next[sessionId] = current[sessionId];
        }
      }

      if (
        !changed &&
        Object.keys(next).length === Object.keys(current).length
      ) {
        return current;
      }

      return next;
    });
  }, [sessions]);

  const handleSessionCwdChange = (sessionId: number, cwd: string) => {
    setSessionPaths((current) => {
      if (current[sessionId] === cwd) {
        return current;
      }
      return {
        ...current,
        [sessionId]: cwd,
      };
    });

    void updateSessionCWD(sessionId, cwd).catch((error) => {
      onError(error);
    });
  };

  return {
    handleSessionCwdChange,
    sessionPaths,
  };
}
