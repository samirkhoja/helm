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
      const anchor = Math.min(
        view.state.selection.main.anchor,
        activeFile.savedContent.length,
      );
      view.dispatch({
        changes: {
          from: 0,
          insert: activeFile.savedContent,
          to: currentDoc.length,
        },
        selection: EditorSelection.cursor(anchor),
      });
    }

    syncDirtyState(view, dirtyRef, onDirtyChangeRef, savedDocRef);
  }, [
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
