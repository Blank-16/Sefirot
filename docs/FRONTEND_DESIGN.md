# SEFIROT frontend design system

## Product intent

SEFIROT is a personal knowledge archive first and a scientific research workspace when requested. The interface must make ordinary note capture feel immediate while keeping research tools discoverable without letting them dominate casual use.

## Visual direction

The visual language takes cues from archival indexes, field notebooks and research margins. The document is the primary object; the file tree is its index; summaries, outlines and backlinks are marginal context. Avoid generic dashboard cards, gradient decoration and unnecessary labels.

## Colour tokens

| Token | Light | Dark | Purpose |
|---|---|---|---|
| Paper | `#F0EEE5` | `#000000` | Application background |
| Reading surface | `#FAF9F3` | `#090B09` | Document and modal surfaces |
| Paper low | `#E8E6DC` | `#070907` | Secondary panel |
| Carbon ink | `#20251F` | `#E9E8DF` | Primary text |
| Index grey | `#77796E` | `#A09F93` | Secondary text |
| Aged brass | `#A88E52` | `#B69B5B` | Selection, focus and research state |
| Moss | `#526356` | `#9CAB91` | Secondary active state |

Dark mode retains a pure-black application background. Accent colour is reserved for meaningful selection and state.

## Typography

- Interface: Avenir Next / Segoe UI / Lato / sans-serif system stack.
- Reading: Iowan Old Style / Palatino Linotype / Book Antiqua / Georgia.
- Source and code: Cascadia Code / SFMono-Regular / Consolas.
- No remote font downloads; the interface remains functional offline.

## Layout

- Masthead: brand, archive search, Papers, Graph and a compact tools menu.
- Left index: vault creation actions, folder tree and vault location.
- Main document: location and view controls, source editor, rendered reading view.
- Right margin: summary, outline, backlinks and document properties.
- Home: brief archive introduction, recently updated notes and local agent activity.
- Graph and library surfaces inherit the same palette, typography and control language.

## Interaction rules

- Casual note mode is the default. Scientific tools are hidden unless Research Mode is explicitly enabled for the current document.
- Research Mode remains a local per-document presentation preference and does not modify note contents or the vault storage format.
- Home and graph views do not show document-only controls or note context panels.
- Keyboard focus is visible. Vault tree entries support Enter and Space. Reduced-motion preferences are respected.
- Mobile collapses the context margin, turns the vault index into an overlay and uses a single document pane.

## Implementation

- `backend/web/visual-system.css` contains design tokens and component styling.
- `backend/web/index.html` contains the semantic application layout and stable interaction IDs.
- `backend/web/js/notes.js` handles home/document view transitions and reading typography.
- `backend/web/js/events.js` handles pointer and keyboard activation in the vault tree.
- `backend/web/js/graph.js` uses the archive palette for graph nodes.

The design stylesheet is loaded as a separate embedded asset, so Tailwind generation is not required to compile it. Existing Tailwind utility assets remain part of the current frontend build for legacy components.
