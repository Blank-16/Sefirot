"use strict";

const DEFAULT_MATH_MACROS = {
  R: "\\mathbb{R}",
  N: "\\mathbb{N}",
  Z: "\\mathbb{Z}",
  Q: "\\mathbb{Q}",
  E: "\\mathbb{E}",
  Var: "\\operatorname{Var}",
  Cov: "\\operatorname{Cov}",
  Tr: "\\operatorname{Tr}",
  dd: "\\,\\mathrm{d}",
  norm: ["\\left\\lVert #1 \\right\\rVert", 1],
};
function loadMathMacros() {
  try {
    return {
      ...DEFAULT_MATH_MACROS,
      ...JSON.parse(localStorage.getItem("sefirot-math-macros") || "{}"),
    };
  } catch {
    return { ...DEFAULT_MATH_MACROS };
  }
}

// Research Mode is a per-document presentation preference. It does not alter
// document contents or the vault's Markdown/HTML storage format.
const RESEARCH_MODE_STORAGE_KEY = "sefirot-research-mode-documents";
function loadResearchModeMap() {
  try {
    const value = JSON.parse(
      localStorage.getItem(RESEARCH_MODE_STORAGE_KEY) || "{}",
    );
    return value && typeof value === "object" && !Array.isArray(value)
      ? value
      : {};
  } catch {
    return {};
  }
}
function isResearchMode(notePath) {
  return Boolean(notePath && loadResearchModeMap()[notePath] === true);
}
function persistResearchMode(notePath, enabled) {
  if (!notePath) return;
  const modes = loadResearchModeMap();
  if (enabled) modes[notePath] = true;
  else delete modes[notePath];
  try {
    localStorage.setItem(RESEARCH_MODE_STORAGE_KEY, JSON.stringify(modes));
  } catch (error) {
    console.warn("Could not persist the document mode preference.", error);
  }
  if (typeof queueSettingsSync === "function") queueSettingsSync();
}
function migrateResearchModePath(oldPath, newPath) {
  const modes = loadResearchModeMap();
  if (modes[oldPath] === true) modes[newPath] = true;
  delete modes[oldPath];
  try {
    localStorage.setItem(RESEARCH_MODE_STORAGE_KEY, JSON.stringify(modes));
  } catch {}
  if (typeof queueSettingsSync === "function") queueSettingsSync();
}
function forgetResearchModePath(notePath) {
  const modes = loadResearchModeMap();
  delete modes[notePath];
  try {
    localStorage.setItem(RESEARCH_MODE_STORAGE_KEY, JSON.stringify(modes));
  } catch {}
}
function applyResearchMode(enabled) {
  const active = Boolean(enabled && state.currentPath);
  state.researchMode = active;
  const toggle = $("#researchModeToggle");
  toggle.textContent = active ? "Research mode on" : "Enable research mode";
  toggle.setAttribute("aria-pressed", String(active));
  toggle.title = active
    ? "Hide scientific authoring tools for this document"
    : "Show scientific authoring tools for this document";
  toggle.classList.toggle("border-amber-800", active);
  toggle.classList.toggle("bg-amber-950/30", active);
  toggle.classList.toggle("text-amber-200", active);
  $("#documentTemplate").classList.toggle("hidden", !active);
  $("#mathMacroManage").classList.toggle("hidden", !active);
  $("#mathToolbar").classList.toggle("hidden", !active);
  $("#mathToolbar").classList.toggle("flex", active);
  $("#editor").placeholder = active
    ? "Write a research document, mathematical derivation, or scientific note…"
    : "Write a note…";
}

$("#researchModeToggle").addEventListener("click", () => {
  if (!state.currentPath) return;
  const next = !state.researchMode;
  persistResearchMode(state.currentPath, next);
  applyResearchMode(next);
  if (!state.dirty) renderPreview();
});

const SCIENTIFIC_TEMPLATES = {
  paper: `# Research Paper Title\n\n**Authors:**  \n**Date:**  \n**Keywords:**  \n\n## Abstract\n\nSummarize the problem, approach, principal results, and implications.\n\n## 1. Introduction\n\n### Problem statement\n\n### Research questions and contributions\n\n## 2. Related Work\n\n## 3. Background and Notation\n\nDefine symbols, assumptions, and terminology before using them.\n\n## 4. Methodology\n\n### Model / experimental setup\n\n### Governing equations\n\n\\begin{equation}\n  \\mathcal{L}(\\theta) = \\frac{1}{N}\\sum_{i=1}^{N} \\ell(f_\\theta(x_i), y_i)\n  \\label{eq:objective}\n\\end{equation}\n\nRefer to Equation \\eqref{eq:objective} in the text.\n\n## 5. Results\n\n| Experiment | Metric | Result |\n|---|---:|---:|\n| Baseline |  |  |\n\n## 6. Discussion\n\n## 7. Limitations\n\n## 8. Conclusion\n\n## References\n\n`,
  review: `# Literature Review: Topic\n\n**Scope:**  \n**Search date:**  \n**Databases / sources:**  \n\n## Research question\n\n## Inclusion and exclusion criteria\n\n## Search strategy\n\n## Thematic synthesis\n\n### Theme 1\n\n### Theme 2\n\n## Comparative evidence\n\n| Study | Method | Dataset / sample | Main finding | Limitation |\n|---|---|---|---|---|\n|  |  |  |  |  |\n\n## Conflicting findings\n\n## Research gaps\n\n## Open questions\n\n## References\n\n`,
  math: `# Mathematical Notes: Topic\n\n## Definitions and notation\n\n- **Definition 1.**  \n\n## Main result\n\n**Theorem 1.** State the precise assumptions and conclusion.\n\n$$\n  \\begin{aligned}\n    f(x) &= \\cdots \\\\\n    &= \\cdots\n  \\end{aligned}\n$$\n\n## Proof\n\n1. State the starting assumptions.\n2. Show each transformation and justify it.\n3. Conclude explicitly.\n\n**Corollary 1.**  \n\n## Examples and counterexamples\n\n## Computational verification\n\n## References\n\n`,
  lab: `# Laboratory Report: Experiment\n\n**Date:**  \n**Researcher(s):**  \n**Instrument / software:**  \n\n## Objective\n\n## Hypothesis\n\n## Theory\n\n## Materials and methods\n\n## Data\n\n| Trial | Independent variable | Measurement | Uncertainty |\n|---:|---:|---:|---:|\n| 1 |  |  |  |\n\n## Analysis\n\n$$\n  \\bar{x} = \\frac{1}{n}\\sum_{i=1}^{n}x_i\n$$\n\n## Results and figures\n\n## Error analysis\n\n## Conclusion\n\n## Reproducibility notes\n\n`,
};

function insertAtCaret(text) {
  const editor = $("#editor");
  const start = editor.selectionStart ?? editor.value.length;
  const end = editor.selectionEnd ?? start;
  editor.setRangeText(text, start, end, "end");
  editor.focus();
  editor.dispatchEvent(new Event("input", { bubbles: true }));
}

$("#mathSymbolPalette").addEventListener("click", (event) => {
  const button = event.target.closest("[data-symbol]");
  if (!button) return;
  insertAtCaret(`\\(${button.dataset.symbol}\\)`);
});

$("#mathSymbolSearch").addEventListener("input", (event) => {
  const query = event.target.value.trim().toLowerCase();
  $("#mathSymbolPalette")
    .querySelectorAll("[data-symbol]")
    .forEach((button) => {
      const haystack =
        `${button.title} ${button.dataset.symbol} ${button.textContent}`.toLowerCase();
      button.hidden = query && !haystack.includes(query);
    });
});

$("#documentTemplate").addEventListener("change", (event) => {
  const key = event.target.value;
  event.target.value = "";
  if (!key || !SCIENTIFIC_TEMPLATES[key]) return;
  const editor = $("#editor");
  if (
    editor.value.trim() &&
    !confirm(
      "Replace the current document with this template? Save or copy your current text first if needed.",
    )
  )
    return;
  if (!state.isMarkdown) {
    state.isMarkdown = true;
    $("#authorMode").value = "markdown";
    $("#editorModeLabel").textContent = "Markdown source";
    $("#propFormat").textContent = "Markdown → HTML";
  }
  editor.value = SCIENTIFIC_TEMPLATES[key];
  editor.setSelectionRange(0, 0);
  editor.focus();
  editor.dispatchEvent(new Event("input", { bubbles: true }));
});

$("#mathMacroManage").addEventListener("click", () => {
  const current = loadMathMacros();
  const instruction =
    "Add/update: name=definition; arguments: name/arity=definition (example: myvec/1=\\boldsymbol{#1}). Remove with -name. Built-ins: " +
    Object.keys(DEFAULT_MATH_MACROS).join(", ");
  const value = prompt(instruction, "");
  if (value === null || !value.trim()) return;
  const trimmed = value.trim();
  const remove = trimmed.match(/^-([A-Za-z]+)$/);
  if (remove) {
    if (Object.hasOwn(DEFAULT_MATH_MACROS, remove[1])) {
      alert("Built-in macros cannot be removed.");
      return;
    }
    delete current[remove[1]];
  } else {
    const split = trimmed.indexOf("=");
    if (split < 1) {
      alert("Use name=LaTeX definition.");
      return;
    }
    const spec = trimmed.slice(0, split).trim();
    const definition = trimmed.slice(split + 1).trim();
    const match = spec.match(/^([A-Za-z]+)(?:\/(\d))?$/);
    if (!match || !definition || definition.length > 300) {
      alert(
        "Use a letter-only macro name, optional /0–9 argument count, and a 1–300 character definition.",
      );
      return;
    }
    if (
      Object.keys(current).length >= 50 &&
      !Object.hasOwn(current, match[1])
    ) {
      alert("A maximum of 50 macros is supported.");
      return;
    }
    current[match[1]] =
      match[2] === undefined ? definition : [definition, Number(match[2])];
  }
  localStorage.setItem("sefirot-math-macros", JSON.stringify(current));
  if (typeof queueSettingsSync === "function") queueSettingsSync();
  if (state.currentPath && !state.dirty) renderPreview();
  alert("Macro saved locally. It will be available in rendered math previews.");
});
