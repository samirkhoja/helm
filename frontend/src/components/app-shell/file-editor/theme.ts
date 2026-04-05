import { HighlightStyle } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";

export const fileEditorTheme = EditorView.theme(
  {
    "&": {
      height: "100%",
      backgroundColor: "#0a0b0d",
      color: "#e7e5e4",
    },
    "&.cm-focused": {
      outline: "none",
    },
    ".cm-scroller": {
      fontFamily:
        '"SF Mono", "JetBrainsMono Nerd Font", "MesloLGS NF", "Menlo", "Monaco", monospace',
      fontSize: "12.5px",
      lineHeight: "1.55",
      overscrollBehavior: "contain",
    },
    ".cm-gutters": {
      backgroundColor: "rgba(255, 255, 255, 0.02)",
      color: "rgba(168, 162, 158, 0.72)",
      borderRight: "1px solid var(--border)",
    },
    ".cm-lineNumbers .cm-gutterElement": {
      minWidth: "30px",
      padding: "0 10px 0 8px",
      textAlign: "right",
    },
    ".cm-content": {
      padding: "12px 0 14px",
      caretColor: "var(--fg)",
    },
    ".cm-line": {
      padding: "0 14px",
    },
    ".cm-activeLine": {
      backgroundColor: "rgba(255, 255, 255, 0.02)",
    },
    ".cm-activeLineGutter": {
      backgroundColor: "rgba(255, 255, 255, 0.02)",
    },
    "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection":
      {
        backgroundColor: "rgba(96, 165, 250, 0.28)",
      },
    ".cm-cursor, .cm-dropCursor": {
      borderLeftColor: "#f5f5f4",
    },
  },
  { dark: true },
);

export const fileEditorHighlightStyle = HighlightStyle.define([
  {
    tag: [tags.comment, tags.blockComment, tags.lineComment, tags.docComment],
    color: "#6b7280",
  },
  {
    tag: [
      tags.keyword,
      tags.controlKeyword,
      tags.operatorKeyword,
      tags.moduleKeyword,
    ],
    color: "#93c5fd",
  },
  {
    tag: [tags.punctuation, tags.operator, tags.separator],
    color: "#cbd5e1",
  },
  {
    tag: [
      tags.function(tags.variableName),
      tags.function(tags.propertyName),
      tags.className,
    ],
    color: "#67e8f9",
  },
  {
    tag: [tags.string, tags.special(tags.string), tags.regexp],
    color: "#86efac",
  },
  {
    tag: [tags.number, tags.bool, tags.null, tags.atom],
    color: "#fcd34d",
  },
  {
    tag: [tags.propertyName, tags.attributeName, tags.tagName, tags.deleted],
    color: "#fda4af",
  },
  {
    tag: [tags.typeName, tags.namespace, tags.self, tags.annotation],
    color: "#c4b5fd",
  },
]);
