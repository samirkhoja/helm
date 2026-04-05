import { forwardRef, useImperativeHandle } from "react";

import { useCodeMirrorFileEditor } from "./file-editor/useCodeMirrorFileEditor";
import type {
  FileEditorHandle,
  FileEditorProps,
} from "./file-editor/types";

export const FileEditor = forwardRef<FileEditorHandle, FileEditorProps>(
  function FileEditor(props, ref) {
    const { activeFile, onDirtyChange, onSave } = props;
    const { editorRootRef, getCurrentContent } = useCodeMirrorFileEditor({
      activeFile,
      onDirtyChange,
      onSave,
    });

    useImperativeHandle(
      ref,
      () => ({
        getCurrentContent,
      }),
      [getCurrentContent],
    );

    return (
      <div
        aria-busy={activeFile.loading}
        className="files-panel files-panel--editor"
      >
        {activeFile.error ? (
          <div className="files-panel__message files-panel__message--error">
            {activeFile.error}
          </div>
        ) : null}

        <div className="files-panel__editor-shell">
          <div
            ref={editorRootRef}
            aria-label={`Edit ${activeFile.path || "file"}`}
            className="files-panel__editor"
          />
        </div>
      </div>
    );
  },
);

FileEditor.displayName = "FileEditor";
