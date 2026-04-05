import { useEffect, useRef } from "react";

import type { MenuAction } from "../utils/appShell";
import { useWailsEvent } from "./useWailsEvent";

type UseGlobalCommandsOptions = {
  performAppAction: (action: MenuAction) => void | Promise<void>;
};

export function useGlobalCommands(options: UseGlobalCommandsOptions) {
  const { performAppAction } = options;
  const performAppActionRef = useRef(performAppAction);

  useEffect(() => {
    performAppActionRef.current = performAppAction;
  }, [performAppAction]);

  useWailsEvent<[MenuAction]>("menu:action", (payload) => {
    void performAppActionRef.current(payload);
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat) {
        return;
      }
      if (!event.metaKey && !event.ctrlKey) {
        return;
      }
      const key = event.key.toLowerCase();
      let action: MenuAction | null = null;

      if (key === "w" && !event.altKey) {
        action = "close-session";
      } else if (key === "-" || key === "_") {
        action = event.altKey ? "zoom-out-diff" : "zoom-out-terminal";
      } else if (key === "0") {
        action = event.altKey ? "reset-diff-zoom" : "reset-terminal-zoom";
      } else if (key === "=" || key === "+") {
        action = event.altKey ? "zoom-in-diff" : "zoom-in-terminal";
      }

      if (!action) {
        return;
      }

      event.preventDefault();
      void performAppActionRef.current(action);
    };

    window.addEventListener("keydown", handleKeyDown, true);
    return () => {
      window.removeEventListener("keydown", handleKeyDown, true);
    };
  }, []);
}
