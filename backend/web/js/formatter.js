"use strict";

const EDITOR_WRAP_KEY = "sefirot-editor-wrap";

function editorWrapEnabled() {
  try {
    return localStorage.getItem(EDITOR_WRAP_KEY) !== "off";
  } catch {
    return true;
  }
}

function applyEditorWrap(wrap) {
  $("#editor").classList.toggle("no-wrap", !wrap);
  const button = $("#editorWrapToggle");
  button.setAttribute("aria-pressed", String(wrap));
  button.title = wrap
    ? "Long lines wrap. Click to scroll horizontally instead."
    : "Long lines scroll horizontally. Click to wrap them.";
}

function setEditorWrap(wrap) {
  try {
    localStorage.setItem(EDITOR_WRAP_KEY, wrap ? "on" : "off");
  } catch {}
  applyEditorWrap(wrap);
  if (typeof queueSettingsSync === "function") queueSettingsSync();
}

$("#editorWrapToggle").addEventListener("click", () =>
  setEditorWrap(!editorWrapEnabled()),
);
applyEditorWrap(editorWrapEnabled());

const PRETTIER_SCRIPTS = [
  "/vendor/prettier/standalone.js",
  "/vendor/prettier/markdown.js",
  "/vendor/prettier/html.js",
];
let prettierLoading = null;

function loadPrettierScript(src) {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = src;
    script.onload = resolve;
    script.onerror = () => {
      script.remove();
      reject(new Error(`Could not load ${src}. Run npm run frontend:build.`));
    };
    document.head.appendChild(script);
  });
}

// Prettier is bundled under /vendor and loaded on first use, so it costs nothing at startup.
function ensurePrettier() {
  if (!prettierLoading) {
    prettierLoading = (async () => {
      for (const src of PRETTIER_SCRIPTS) await loadPrettierScript(src);
      if (
        !window.prettier ||
        !window.prettierPlugins?.markdown ||
        !window.prettierPlugins?.html
      )
        throw new Error("The formatter did not initialise.");
    })().catch((error) => {
      prettierLoading = null;
      throw error;
    });
  }
  return prettierLoading;
}

const MATH_PATTERNS = [
  /\$\$[\s\S]*?\$\$/g,
  /\\\[[\s\S]*?\\\]/g,
  /\\\([^\n]*?\\\)/g,
  /(?<![\\$])\$(?![\s$])(?:\\.|[^$\\\n])*?(?<![\s\\])\$(?!\d)/g,
];

// Math is lifted out before formatting and put back byte for byte, and the result is
// rejected if any span did not survive, so formatting can never alter an equation.
function protectMath(source) {
  const spans = [];
  let text = source;
  for (const pattern of MATH_PATTERNS) {
    text = text.replace(pattern, (match) => {
      spans.push(match);
      return `SEFIROTMATH${spans.length - 1}X`;
    });
  }
  return { text, spans };
}

function restoreMath(text, spans) {
  let restored = text;
  for (let i = 0; i < spans.length; i++) {
    const token = `SEFIROTMATH${i}X`;
    if (restored.split(token).length !== 2)
      throw new Error(
        "Formatting would have changed a math expression, so nothing was changed.",
      );
    restored = restored.replace(token, () => spans[i]);
  }
  return restored;
}

async function formatSource(source, isMarkdown) {
  await ensurePrettier();
  const plugins = [
    window.prettierPlugins.markdown,
    window.prettierPlugins.html,
  ];
  if (!isMarkdown)
    return window.prettier.format(source, {
      parser: "html",
      plugins,
      printWidth: 100,
    });
  const front = source.match(/^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/);
  const head = front ? front[0] : "";
  const { text, spans } = protectMath(source.slice(head.length));
  const formatted = await window.prettier.format(text, {
    parser: "markdown",
    plugins,
    proseWrap: "preserve",
    embeddedLanguageFormatting: "off",
  });
  return head + restoreMath(formatted, spans);
}

async function formatEditor() {
  const editor = $("#editor");
  const button = $("#editorFormat");
  const before = editor.value;
  if (!before.trim()) return;
  button.disabled = true;
  try {
    const after = await formatSource(before, state.isMarkdown);
    if (after === before) {
      setStatus("Already formatted", "text-zinc-500");
      return;
    }
    editor.focus();
    editor.select();
    // insertText keeps the browser undo stack, so Ctrl+Z restores the original source.
    if (!document.execCommand("insertText", false, after)) {
      editor.value = after;
      editor.dispatchEvent(new Event("input", { bubbles: true }));
    }
    setStatus("Formatted (Ctrl+Z to undo)", "text-emerald-400");
  } catch (error) {
    setStatus(`Format failed: ${error.message}`, "text-red-400");
  } finally {
    button.disabled = false;
  }
}

$("#editorFormat").addEventListener("click", formatEditor);
