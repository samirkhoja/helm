import { useEffect, useRef } from "react";

import { EventsOn } from "../../wailsjs/runtime/runtime";

type EventHandler<Args extends unknown[]> = (...args: Args) => void;

export function useWailsEvent<Args extends unknown[]>(
  eventName: string,
  handler: EventHandler<Args>,
  enabled = true,
) {
  const handlerRef = useRef(handler);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    if (!enabled) {
      return;
    }

    const unsubscribe = EventsOn(eventName, (...args: unknown[]) => {
      handlerRef.current(...(args as Args));
    });

    return () => {
      unsubscribe();
    };
  }, [enabled, eventName]);
}
