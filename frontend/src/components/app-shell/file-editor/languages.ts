import { StreamLanguage } from "@codemirror/language";
import type { Extension } from "@codemirror/state";

type LanguageLoader = () => Promise<Extension>;
type FileNameLoader = {
  load: LanguageLoader;
  matches: (baseName: string) => boolean;
};

const fallbackLanguageLoader: LanguageLoader = async () => [];

async function loadCssLanguage() {
  const module = await import("@codemirror/lang-css");
  return module.css();
}

async function loadGoLanguage() {
  const module = await import("@codemirror/lang-go");
  return module.go();
}

async function loadHtmlLanguage() {
  const module = await import("@codemirror/lang-html");
  return module.html();
}

async function loadHtmlLegacyLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/xml");
  return StreamLanguage.define(module.html);
}

async function loadXmlLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/xml");
  return StreamLanguage.define(module.xml);
}

async function loadJavaScriptLanguage() {
  const module = await import("@codemirror/lang-javascript");
  return module.javascript();
}

async function loadTypeScriptLanguage() {
  const module = await import("@codemirror/lang-javascript");
  return module.javascript({ typescript: true });
}

async function loadJsxLanguage() {
  const module = await import("@codemirror/lang-javascript");
  return module.javascript({ jsx: true });
}

async function loadTsxLanguage() {
  const module = await import("@codemirror/lang-javascript");
  return module.javascript({ jsx: true, typescript: true });
}

async function loadJsonLanguage() {
  const module = await import("@codemirror/lang-json");
  return module.json();
}

async function loadMarkdownLanguage() {
  const module = await import("@codemirror/lang-markdown");
  return module.markdown();
}

async function loadYamlLanguage() {
  const module = await import("@codemirror/lang-yaml");
  return module.yaml();
}

async function loadDockerfileLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/dockerfile");
  return StreamLanguage.define(module.dockerFile);
}

async function loadPythonLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/python");
  return StreamLanguage.define(module.python);
}

async function loadRubyLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/ruby");
  return StreamLanguage.define(module.ruby);
}

async function loadRustLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/rust");
  return StreamLanguage.define(module.rust);
}

async function loadSwiftLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/swift");
  return StreamLanguage.define(module.swift);
}

async function loadShellLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/shell");
  return StreamLanguage.define(module.shell);
}

async function loadSqlLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/sql");
  return StreamLanguage.define(module.standardSQL);
}

async function loadTomlLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/toml");
  return StreamLanguage.define(module.toml);
}

async function loadLuaLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/lua");
  return StreamLanguage.define(module.lua);
}

async function loadPerlLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/perl");
  return StreamLanguage.define(module.perl);
}

async function loadSassLanguage() {
  const module = await import("@codemirror/legacy-modes/mode/sass");
  return StreamLanguage.define(module.sass);
}

async function loadClikeLanguage(
  variant:
    | "c"
    | "cpp"
    | "csharp"
    | "java"
    | "kotlin"
    | "objectiveC"
    | "objectiveCpp"
    | "scala",
) {
  const module = await import("@codemirror/legacy-modes/mode/clike");
  return StreamLanguage.define(module[variant]);
}

function registerExtensions(
  extensionLoaders: Map<string, LanguageLoader>,
  extensions: string[],
  load: LanguageLoader,
) {
  for (const extension of extensions) {
    extensionLoaders.set(extension, load);
  }
}

function parsePathParts(path: string) {
  const normalizedPath = path.toLowerCase();
  const baseName = normalizedPath.split("/").pop() ?? normalizedPath;
  const parts = baseName.split(".");

  return {
    baseName,
    extension: parts.length > 1 ? parts[parts.length - 1] : "",
  };
}

const fileNameLoaders: FileNameLoader[] = [
  {
    load: loadDockerfileLanguage,
    matches: (baseName) =>
      baseName === "dockerfile" || baseName.endsWith(".dockerfile"),
  },
];

const extensionLoaders = new Map<string, LanguageLoader>();

registerExtensions(extensionLoaders, ["css"], loadCssLanguage);
registerExtensions(extensionLoaders, ["go"], loadGoLanguage);
registerExtensions(extensionLoaders, ["html"], loadHtmlLanguage);
registerExtensions(extensionLoaders, ["htm", "xhtml"], loadHtmlLegacyLanguage);
registerExtensions(extensionLoaders, ["js"], loadJavaScriptLanguage);
registerExtensions(extensionLoaders, ["json"], loadJsonLanguage);
registerExtensions(extensionLoaders, ["jsx"], loadJsxLanguage);
registerExtensions(extensionLoaders, ["md", "markdown"], loadMarkdownLanguage);
registerExtensions(extensionLoaders, ["py", "pyi", "pyw"], loadPythonLanguage);
registerExtensions(extensionLoaders, ["rb", "rake", "gemspec"], loadRubyLanguage);
registerExtensions(extensionLoaders, ["java"], () => loadClikeLanguage("java"));
registerExtensions(extensionLoaders, ["rs"], loadRustLanguage);
registerExtensions(extensionLoaders, ["c", "h"], () => loadClikeLanguage("c"));
registerExtensions(
  extensionLoaders,
  ["cc", "cpp", "cxx", "hh", "hpp", "hxx"],
  () => loadClikeLanguage("cpp"),
);
registerExtensions(extensionLoaders, ["cs"], () => loadClikeLanguage("csharp"));
registerExtensions(extensionLoaders, ["kt", "kts"], () =>
  loadClikeLanguage("kotlin"),
);
registerExtensions(extensionLoaders, ["scala"], () =>
  loadClikeLanguage("scala"),
);
registerExtensions(extensionLoaders, ["m"], () =>
  loadClikeLanguage("objectiveC"),
);
registerExtensions(extensionLoaders, ["mm"], () =>
  loadClikeLanguage("objectiveCpp"),
);
registerExtensions(extensionLoaders, ["swift"], loadSwiftLanguage);
registerExtensions(
  extensionLoaders,
  ["bash", "fish", "sh", "zsh"],
  loadShellLanguage,
);
registerExtensions(extensionLoaders, ["ts"], loadTypeScriptLanguage);
registerExtensions(extensionLoaders, ["tsx"], loadTsxLanguage);
registerExtensions(extensionLoaders, ["yaml", "yml"], loadYamlLanguage);
registerExtensions(extensionLoaders, ["xml", "xsd", "xsl", "svg"], loadXmlLanguage);
registerExtensions(extensionLoaders, ["sql"], loadSqlLanguage);
registerExtensions(extensionLoaders, ["toml"], loadTomlLanguage);
registerExtensions(extensionLoaders, ["lua"], loadLuaLanguage);
registerExtensions(extensionLoaders, ["pl", "pm"], loadPerlLanguage);
registerExtensions(extensionLoaders, ["sass", "scss"], loadSassLanguage);

export async function loadLanguageExtensionForPath(path: string) {
  const { baseName, extension } = parsePathParts(path);
  const fileNameLoader = fileNameLoaders.find((loader) =>
    loader.matches(baseName),
  );

  if (fileNameLoader) {
    return fileNameLoader.load();
  }

  const loadExtension = extensionLoaders.get(extension) ?? fallbackLanguageLoader;
  return loadExtension();
}
