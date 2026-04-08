import { useEffect, useLayoutEffect, useRef } from "react";
import type { MutableRefObject } from "react";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import {
  bracketMatching,
  defaultHighlightStyle,
  indentUnit,
  syntaxHighlighting,
} from "@codemirror/language";
import {
  Compartment,
  EditorSelection,
  EditorState,
  Text,
} from "@codemirror/state";
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightActiveLineGutter,
  keymap,
  lineNumbers,
} from "@codemirror/view";

import { loadLanguageExtensionForPath } from "./languages";
import { fileEditorHighlightStyle, fileEditorTheme } from "./theme";
import type { FileEditorProps } from "./types";

type UseCodeMirrorFileEditorOptions = Pick<
  FileEditorProps,
  "activeFile" | "onDirtyChange" | "onSave"
>;

type FileNavigationTarget = FileEditorProps["activeFile"]["editorSync"]["target"];

function createTabCommand() {
  return ({
    dispatch,
    state,
  }: {
    dispatch?: EditorView["dispatch"];
    state: EditorState;
  }) => {
    dispatch?.(state.replaceSelection("  "));
    return true;
  };
}

function syncDirtyState(
  view: EditorView,
  dirtyRef: MutableRefObject<boolean>,
  onDirtyChangeRef: MutableRefObject<(dirty: boolean) => void>,
  savedDocRef: MutableRefObject<Text>,
) {
  const nextDirty = !savedDocRef.current.eq(view.state.doc);
  if (nextDirty === dirtyRef.current) {
    return;
  }

  dirtyRef.current = nextDirty;
  onDirtyChangeRef.current(nextDirty);
}

function targetOffsetForLine(
  lineText: string,
  lineFrom: number,
  targetColumn: number,
) {
  const column = Math.max(1, targetColumn);
  const codePoints = Array.from(lineText);
  const safeColumn = Math.min(column, codePoints.length + 1);
  const prefix = codePoints.slice(0, safeColumn - 1).join("");
  return lineFrom + prefix.length;
}

function applyNavigationTarget(
  view: EditorView,
  target: FileNavigationTarget,
) {
  if (!target) {
    return;
  }

  const lineNumber = Math.min(
    Math.max(1, target.line),
    Math.max(1, view.state.doc.lines),
  );
  const line = view.state.doc.line(lineNumber);
  const offset = targetOffsetForLine(line.text, line.from, target.column);
  view.dispatch({
    selection: EditorSelection.cursor(offset),
    scrollIntoView: true,
  });
}

export function useCodeMirrorFileEditor(
  options: UseCodeMirrorFileEditorOptions,
) {
  const { activeFile, onDirtyChange, onSave } = options;

  const editorRootRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onDirtyChangeRef = useRef(onDirtyChange);
  const onSaveRef = useRef(onSave);
  const languageCompartmentRef = useRef(new Compartment());
  const readOnlyCompartmentRef = useRef(new Compartment());
  const languageLoadRequestIdRef = useRef(0);
  const savedDocRef = useRef(Text.empty);
  const dirtyRef = useRef(false);
  const appliedSyncTokenRef = useRef(activeFile.editorSync.token);

  useEffect(() => {
    onDirtyChangeRef.current = onDirtyChange;
  }, [onDirtyChange]);

  useEffect(() => {
    onSaveRef.current = onSave;
  }, [onSave]);

  useLayoutEffect(() => {
    const parent = editorRootRef.current;
    if (!parent) {
      return;
    }

    const tabCommand = createTabCommand();

    const view = new EditorView({
      state: EditorState.create({
        doc: activeFile.savedContent,
        selection: EditorSelection.cursor(0),
        extensions: [
          lineNumbers(),
          highlightActiveLineGutter(),
          history(),
          drawSelection(),
          highlightActiveLine(),
          bracketMatching(),
          indentUnit.of("  "),
          EditorState.readOnly.of(activeFile.loading),
          keymap.of([
            {
              key: "Mod-s",
              run: () => {
                void onSaveRef.current(view.state.doc.toString());
                return true;
              },
            },
            {
              key: "Tab",
              run: tabCommand,
            },
            ...defaultKeymap,
            ...historyKeymap,
          ]),
          syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
          syntaxHighlighting(fileEditorHighlightStyle),
          fileEditorTheme,
          languageCompartmentRef.current.of([]),
          readOnlyCompartmentRef.current.of(
            EditorState.readOnly.of(activeFile.loading),
          ),
          EditorView.updateListener.of((update) => {
            if (!update.docChanged) {
              return;
            }

            syncDirtyState(
              update.view,
              dirtyRef,
              onDirtyChangeRef,
              savedDocRef,
            );
          }),
        ],
      }),
      parent,
    });

    savedDocRef.current = view.state.doc;
    dirtyRef.current = false;
    appliedSyncTokenRef.current = activeFile.editorSync.token;
    onDirtyChangeRef.current(false);

    viewRef.current = view;
    view.focus();
    view.scrollDOM.scrollTop = 0;
    view.scrollDOM.scrollLeft = 0;
    applyNavigationTarget(view, activeFile.editorSync.target);

    return () => {
      if (viewRef.current === view) {
        viewRef.current = null;
      }
      view.destroy();
    };
  }, [activeFile.path]);

  useEffect(() => {
    const requestId = languageLoadRequestIdRef.current + 1;
    languageLoadRequestIdRef.current = requestId;

    void loadLanguageExtensionForPath(activeFile.path)
      .then((languageExtension) => {
        if (languageLoadRequestIdRef.current !== requestId) {
          return;
        }

        const view = viewRef.current;
        if (!view) {
          return;
        }

        view.dispatch({
          effects: languageCompartmentRef.current.reconfigure(languageExtension),
        });
      })
      .catch(() => {
        if (languageLoadRequestIdRef.current !== requestId) {
          return;
        }

        const view = viewRef.current;
        if (!view) {
          return;
        }

        view.dispatch({
          effects: languageCompartmentRef.current.reconfigure([]),
        });
      });
  }, [activeFile.path]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }

    const nextSavedDoc = view.state.toText(activeFile.savedContent);
    const currentDoc = view.state.doc;
    const shouldApplySync =
      appliedSyncTokenRef.current !== activeFile.editorSync.token;

    savedDocRef.current = nextSavedDoc;
    appliedSyncTokenRef.current = activeFile.editorSync.token;

    if (
      shouldApplySync &&
      activeFile.editorSync.strategy === "replace-document" &&
      !nextSavedDoc.eq(currentDoc)
    ) {
      let anchor = Math.min(
        view.state.selection.main.anchor,
        activeFile.savedContent.length,
      );
      if (activeFile.editorSync.target) {
        const lineNumber = Math.min(
          Math.max(1, activeFile.editorSync.target.line),
          Math.max(1, nextSavedDoc.lines),
        );
        const line = nextSavedDoc.line(lineNumber);
        anchor = targetOffsetForLine(
          line.text,
          line.from,
          activeFile.editorSync.target.column,
        );
      }
      view.dispatch({
        changes: {
          from: 0,
          insert: activeFile.savedContent,
          to: currentDoc.length,
        },
        selection: EditorSelection.cursor(anchor),
        scrollIntoView: activeFile.editorSync.target != null,
      });
    }

    if (
      shouldApplySync &&
      activeFile.editorSync.strategy === "reveal-location" &&
      activeFile.editorSync.target
    ) {
      applyNavigationTarget(view, activeFile.editorSync.target);
    }

    syncDirtyState(view, dirtyRef, onDirtyChangeRef, savedDocRef);
  }, [
    activeFile.editorSync.target,
    activeFile.editorSync.strategy,
    activeFile.editorSync.token,
    activeFile.savedContent,
  ]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }

    view.dispatch({
      effects: readOnlyCompartmentRef.current.reconfigure(
        EditorState.readOnly.of(activeFile.loading),
      ),
    });
  }, [activeFile.loading]);

  return {
    editorRootRef,
    getCurrentContent: () =>
      viewRef.current?.state.doc.toString() ?? activeFile.savedContent,
  };
}
