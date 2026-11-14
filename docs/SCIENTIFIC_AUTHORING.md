# Scientific authoring in SEFIROT

SEFIROT keeps the document source as Markdown/HTML in the vault. Mathematical expressions are authored as TeX and rendered in the isolated preview with the locally bundled MathJax 3 assets. The source remains plain text and is not rewritten by the renderer.

## Research Mode

- Every document opens in the normal, minimal note-taking mode by default. Existing notes are not converted or modified.
- Select **Enable research mode** in the editor header to reveal scientific authoring controls, symbol search, TeX insertion tools, macros, and research-document templates.
- The mode preference is stored per document in the current browser profile. Renaming a note migrates the preference; deleting a note clears it.
- Turning Research Mode off hides the scientific controls without deleting equations, templates, macros, or changing the document source.
- The preference is UI state, not document metadata. It is not currently synchronized across devices or browser profiles.

## Editor workflow

- Use **Insert template…** for a research paper, literature review, mathematical notes, or laboratory report scaffold.
- Use the **Insert** toolbar for inline/display math, aligned derivations, matrices, fractions, sums, integrals, vector calculus, quantum notation, and chemistry notation.
- Search the symbol palette by its accessible title or TeX command (for example `partial`, `nabla`, or `alpha`). A selected symbol is inserted at the current caret inside an inline TeX delimiter.
- Use **Macros** to add a local TeX macro as `name=definition`, or a macro with arguments as `name/arity=definition`. Remove custom macros with `-name`. Built-in macros include `R`, `N`, `Z`, `Q`, `E`, `Var`, `Cov`, `Tr`, `dd`, and `norm`.
- Use AMS `equation`, `align`, `gather`, `\label`, `\ref`, and `\eqref` for numbered equations and cross-references. Numbering is managed by MathJax in the preview; the stored source is unchanged.
- Use `\ce{...}` for chemical equations and `\ket{...}` / `\bra{...}` for common quantum notation where the corresponding MathJax extensions are available.

## Source conventions

Inline expressions use `\\( ... \\)`. Display expressions use `\\[ ... \\]` or AMS environments. Keep equations in the source rather than pasting rendered screenshots. Use Markdown headings for section hierarchy, tables for structured data, fenced code blocks for algorithms, and explicit captions/alt text for figures.

## Accuracy and limitations

- MathJax renders TeX; it is not a LaTeX compiler. It does not guarantee that arbitrary LaTeX packages or TikZ source will compile.
- The editor currently provides a source-first Markdown/HTML workflow, not a WYSIWYG equation editor.
- The current paper template uses Markdown sections and table scaffolds. It does not yet enforce journal-specific section rules or semantic theorem/proof numbering.
- Equation labels and references should be checked in Preview after saving. MathJax rendering is client-side and may show a load error if local vendor assets have not been built.
- Custom macros are stored in this browser profile's local storage, not in the vault. Export or copy their definitions separately when moving to another device.
- Chemical/quantum commands depend on MathJax extension availability in the generated vendor bundle. No external CDN is required by the intended production build.
