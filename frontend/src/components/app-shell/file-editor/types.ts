import type { ActiveEditorFile } from "../../../hooks/filesPanelTypes";

export type FileEditorHandle = {
  getCurrentContent: () => string;
};

export type FileEditorProps = {
  activeFile: ActiveEditorFile;
  onDirtyChange: (dirty: boolean) => void;
  onSave: (content?: string) => void;
};
