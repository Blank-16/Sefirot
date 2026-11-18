# High Level Design (HLD): SEFIROT

## 1. Executive Summary and Principles

SEFIROT is a local-first personal knowledge archive and scientific research workspace. It integrates document authoring, an interactive knowledge graph, SQLite FTS5 search, local semantic retrieval, a bibliographic research engine, and an embedded PTY terminal for autonomous AI coding agents.

### Core Design Principles

- **Local-First and Source-of-Truth**: The filesystem (`vault/`) is the durable source of truth. SQLite is a derived, rebuildable index and cache.
- **Interoperability**: Notes are stored as standard `.html` files. Markdown source is preserved within HTML comments, ensuring external editors and CLI tools can read and write without proprietary lock-in.
- **Zero-Cloud Dependency**: Runs entirely on loopback (`127.0.0.1`) without requiring remote accounts, cloud sync, or external analytics.
- **Process Isolation and Security**: The web interface isolates note preview execution inside sandboxed iframes. HTTP endpoints strictly reject non-local requests.

---

## 2. End-to-End System Architecture

```mermaid
flowchart TD
    subgraph Host ["Client & Runtime Boundary"]
        Tauri["Tauri Desktop Shell (Rust)"]
        Browser["Web Browser (http://127.0.0.1:8080)"]
    end

    subgraph Backend ["Go Application Core"]
        Mux["HTTP Router (localOnly + securityHeaders)"]

        subgraph Subsystems ["Core Modules"]
            NoteSvc["Note Service (os.Root scoped)"]
            IndexSvc["Indexing Pipeline (Goldmark + HTML)"]
            WatchSvc["File Watcher (fsnotify + 30s debouncer)"]
            SearchSvc["Search Engine (FTS5 + Vector RRF)"]
            TermSvc["Terminal Manager (CrossPTY + WS)"]
            ResearchSvc["Research Library (BibTeX/RIS)"]
        end

        subgraph Pools ["SQLite Connection Pools"]
            ReaderPool["Reader Pool (Multi-connection)"]
            WriterPool["Writer Pool (Single-connection, WAL)"]
        end
    end

    subgraph Disk ["Local Storage"]
        VaultDir[("Vault Directory (HTML, Markdown, PDFs)")]
        SQLiteDB[("SQLite Database (index.db + WAL)")]
    end

    subgraph External ["Host CLI & Services"]
        OllamaSvc["Local Ollama API (:11434)"]
        AgentPty["PTY Shells (Claude, Codex, bash, agy)"]
    end

    Tauri -->|Loopback IPC| Mux
    Browser -->|HTTP / WebSocket| Mux

    Mux --> NoteSvc
    Mux --> SearchSvc
    Mux --> TermSvc
    Mux --> ResearchSvc

    NoteSvc -->|Read/Write HTML| VaultDir
    NoteSvc -->|Index Trigger| IndexSvc
    WatchSvc -->|Monitor Events| VaultDir
    WatchSvc -->|Reconcile Changes| IndexSvc

    IndexSvc -->|Persist Metadata| WriterPool
    SearchSvc -->|Query FTS5| ReaderPool
    SearchSvc -->|Vector Search| OllamaSvc
    ResearchSvc -->|Query/Update| WriterPool

    WriterPool --> SQLiteDB
    ReaderPool --> SQLiteDB

    TermSvc -->|Spawn & Stream| AgentPty
```

---

## 3. Subsystem Architecture

### 3.1. Durable Storage and Filesystem Layer

- **Vault Directory (`vault/`)**: Contains all notes, folder structures, and static assets.
- **Document Format**: HTML5 documents. When editing in Markdown mode, Goldmark parses Markdown to HTML, embedding the raw Markdown in a Base64-encoded HTML comment: `<!--markdown-source:...-->`.
- **Atomic Operations and Path Traversal Prevention**: Filesystem access uses Go's `os.Root` to scope all disk operations within the designated vault path, rejecting relative path escapes (`../`) or symlink exploits.
- **Reconciliation Engine**:
  - Real-time indexing via `fsnotify` with a 150ms per-path debounce.
  - 30-second periodic background reconciliation pass to catch missed OS filesystem events.
  - SHA-256 content hashing to skip unchanged files.

### 3.2. Indexing and Search Engine

- **SQLite Engine**: Configured in WAL (Write-Ahead Logging) mode with `synchronous=NORMAL` and `busy_timeout=5000`. Uses a dedicated single-connection writer pool and a multi-connection reader pool to prevent database locking.
- **Lexical Search (FTS5)**: Automatic triggers synchronize document content with an external-content SQLite FTS5 table. Searches support prefix matching, exact phrases, tags (`tag:systems`), note types (`type:concept`), and folder scoping (`folder:notes`).
- **Semantic and Hybrid Search**:
  - Local embedding generation via Ollama on loopback.
  - Vector similarity calculation stored directly in SQLite tables.
  - Hybrid search combines FTS5 BM25 lexical ranking and vector similarity using Reciprocal Rank Fusion (RRF).

### 3.3. Knowledge Graph and Archive Context Protocol

- **Link Resolution**: Notes define connections via Wiki-links (`[[Target Note]]`) or HTML anchors with `data-wikilink="target"`.
- **Typed Semantic Relationships**: Links specify relations via `data-relation` attributes (such as `depends-on`, `implements`, `extends`, `contradicts`, `references`).
- **Archive Context Protocol (`GET /api/ai/context`)**: Returns a compact payload containing the target document's metadata, three-tier summaries (short graph label, note summary, and archive context), heading outlines, backlinks, and typed relationships. Designed specifically for external AI CLI agents.

### 3.4. Research and Scientific Authoring Subsystem

- **Academic Metadata Store**: Dedicated SQLite tables (`research_papers`, `research_citations`, `research_annotations`) separate from the standard note schema.
- **Bibliography Ingestion**: Native parsing for BibTeX and RIS citation formats, DOI lookup, and manual metadata authoring.
- **PDF Vault Integration**: Original PDF papers are stored in `vault/` without modification.
- **Scientific Authoring**: Searchable TeX symbol palette, inline/display mathematical derivations, AMS equation numbering, and client-side rendering via MathJax.

### 3.5. Terminal Subsystem

- **CrossPTY Abstraction**: Uses Unix pseudo-terminals (PTY) on macOS/Linux and ConPTY on Windows 10+. The `wsl` profile is conditionally offered only on Windows hosts, defaulting to native shells (`zsh`, `bash`, `sh`) on Linux/WSL.
- **Server-Owned Sessions & Replay Buffer**: Terminal sessions are owned by the backend process rather than tied to a single transient WebSocket connection. Each session maintains a 256 KB circular output replay buffer, allowing browser tabs to reconnect or reload without losing terminal history.
- **Lifecycle & Idle Termination**: Sessions without an attached browser remain active in the background and automatically terminate after a 10-minute idle timeout.
- **WebSocket Gateway**: Terminal input and output stream over bidirectional WebSockets via `/api/terminal/ws`. Localhost and loopback aliases (`localhost`, `127.0.0.1`, `[::1]`) are validated without cross-origin rejections.
- **Process Management**: Launches user shells and CLI tools (`claude`, `codex`, `gemini`, `aider`, `agy`) in the active vault directory with resize (`SIGWINCH`) synchronization.
- **Security**: WebSocket upgrades require a cryptographically random session token (`X-Vault-Token`). Maximum of 4 concurrent sessions.

### 3.6. Presentation Layer

- **Embedded Web UI**: Static assets served directly from the Go binary via `embed.FS`, with bundled offline vendor dependencies (`xterm.js`, `addon-fit.js`, `MathJax`, `Prettier`).
- **Modular Frontend**: Vanilla JavaScript modules (`core.js`, `notes.js`, `search.js`, `graph.js`, `terminal.js`, `research.js`, `formatter.js`, `ui.js`) without heavy runtime frameworks.
- **Visual Design System**: Defined in `visual-system.css` using CSS custom properties for dual themes: warm off-white light theme and pure-black dark theme. Theme rules and base surfaces are injected into note previews so standalone HTML notes adapt smoothly to light mode.
- **Editor Ergonomics**: Includes an editor word-wrap toggle (`white-space: pre` vs `white-space: pre-wrap`) and an integrated Prettier formatter that isolates LaTeX and AMS mathematical expressions (`$$...$$`, `\(...\)`, `\[...\]`) before formatting and validates byte-for-byte equation integrity upon restoration.

### 3.7. Desktop Shell (Tauri v2)

- **Sidecar Execution**: The Tauri Rust application spawns the compiled Go binary as a sidecar process on startup.
- **Ephemeral Port Negotiation**: Passes `--port 0` and `--ready-file <path>` to the Go backend. Once the backend binds to an open loopback port and writes its URL and token to the ready-file, the Tauri WebView navigates to that address.

---

## 4. Pipeline and Sequence Diagrams

### 4.1. Document Ingestion and Indexing Pipeline

```mermaid
flowchart LR
    Source["File Source (Editor or External CLI)"] --> Format{"Input Format"}

    Format -->|Markdown| GM["Goldmark Engine"]
    Format -->|Raw HTML| Ext["HTML Extractor"]

    GM -->|Render HTML + Embed Base64 Comment| Ext

    Ext --> ParseMeta["Parse meta tags (type, tags, summary)"]
    Ext --> ParseText["Extract text (strip script & style)"]
    Ext --> ParseLinks["Resolve Wiki-links and data-relation"]
    Ext --> HashCalc["Calculate SHA-256 Content Hash"]

    HashCalc --> DBTrans["SQLite Transaction"]
    ParseMeta --> DBTrans
    ParseText --> DBTrans
    ParseLinks --> DBTrans

    DBTrans --> NotesTable["Update notes table"]
    DBTrans --> LinksTable["Update links and relationships tables"]
    NotesTable --> FTS5Trigger["Execute FTS5 synchronization triggers"]
    FTS5Trigger --> FTS5Index[("FTS5 Full-Text Index")]
```

### 4.2. Search and Hybrid Retrieval Architecture

```mermaid
flowchart TD
    Query["Search Query (User / Agent)"] --> Classifier{"Query Type"}

    Classifier -->|Lexical / Filters| LexicalBranch["FTS5 Engine"]
    Classifier -->|Semantic Query| SemanticBranch["Vector Engine"]
    Classifier -->|Hybrid Query| BothBranch["Parallel Execution"]

    BothBranch --> LexicalBranch
    BothBranch --> SemanticBranch

    subgraph Lexical ["Lexical Path"]
        LexicalBranch --> Tokenize["Tokenize & Filter (tag:, type:, folder:)"]
        Tokenize --> BM25["BM25 Ranking & Snippet Generation"]
        BM25 --> LexRank["Lexical Rank List"]
    end

    subgraph Semantic ["Semantic Path"]
        SemanticBranch --> OllamaEmbed["Ollama Local Embedding Generation"]
        OllamaEmbed --> CosineSim["Scan & Compute Cosine Similarity"]
        CosineSim --> VecRank["Vector Rank List"]
    end

    LexRank --> RRF["Reciprocal Rank Fusion (RRF Merger)"]
    VecRank --> RRF

    RRF --> FinalResults["Unified Ranked Results with Snippets"]
```

### 4.3. Terminal Subprocess and WebSocket Streaming

```mermaid
sequenceDiagram
    autonumber
    actor User as User / Frontend UI
    participant WS as WebSocket Endpoint (/api/terminal/ws)
    participant Term as Terminal Controller
    participant PTY as CrossPTY Session
    participant Proc as CLI Subprocess (bash, claude, agy)

    User->>Term: POST /api/terminal/start (Profile: shell)
    Term->>PTY: Allocate PTY (Row: 30, Col: 100)
    PTY->>Proc: Spawn shell process in vault directory
    Term-->>User: Return session ID and token

    User->>WS: Connect WebSocket (id, token)
    WS->>Term: Validate single-use token
    WS-->>User: 101 Switching Protocols

    par User to Subprocess
        User->>WS: Raw keyboard input / JSON resize payload
        WS->>PTY: Write bytes or invoke PTY.Resize()
        PTY->>Proc: Send stdin / SIGWINCH
    and Subprocess to User
        Proc->>PTY: Stdout / Stderr output
        PTY->>WS: Read buffer chunks
        WS->>User: Stream binary WebSocket frames
        User->>User: Render inside xterm.js
    end

    Proc->>PTY: Process exits
    PTY->>Term: Capture exit code
    Term->>Term: Record activity in agent_runs table
    WS-->>User: Close WebSocket connection
```

---

## 5. Database Entity-Relationship Model

```mermaid
erDiagram
    NOTES ||--o{ LINKS : sources
    NOTES ||--o{ RELATIONSHIPS : defines
    NOTES ||--o{ REVISIONS : archives
    NOTES ||--o| EMBEDDINGS : has
    RESEARCH_PAPERS ||--o{ RESEARCH_CITATIONS : contains
    RESEARCH_PAPERS ||--o{ RESEARCH_ANNOTATIONS : contains

    NOTES {
        string path PK
        string title
        string visible_text
        string content_hash
        string summary_short
        string summary_detail
        string note_type
        string tags
        datetime updated_at
    }

    LINKS {
        string source_path FK
        string target_slug
        datetime created_at
    }

    RELATIONSHIPS {
        string source_path FK
        string target_slug
        string relation_type
    }

    REVISIONS {
        int id PK
        string path FK
        string content
        datetime created_at
    }

    EMBEDDINGS {
        string path PK, FK
        blob vector
        datetime updated_at
    }

    RESEARCH_PAPERS {
        string id PK
        string title
        string authors_json
        string abstract
        string doi
        string reading_status
        string source_file
        datetime created_at
    }

    RESEARCH_CITATIONS {
        string id PK
        string paper_id FK
        string citation_text
        string locator
    }

    RESEARCH_ANNOTATIONS {
        string id PK
        string paper_id FK
        int page_number
        string quote
        string note
    }
```

---

## 6. Frontend Layout and Component Structure

```mermaid
flowchart TD
    subgraph UI ["SEFIROT Web Interface Layout"]
        TopBar["Top Navigation Bar (Emblem, Search Ctrl+K, Papers, Graph, Tools, Theme Toggle)"]

        subgraph BodyArea ["Workspace Container"]
            LeftNav["Left Sidebar (Folder Tree, New Note, Trash)"]

            subgraph CenterPanes ["Active Document Area"]
                TabStrip["Document Tab Strip (welcome.html, Concept.html)"]
                EditorSplit["Editor Pane (Raw HTML / Markdown, Research Mode Toolbar)"]
                PreviewSplit["Isolated Preview Iframe (Sandboxed, MathJax, Graph Links)"]
            end

            RightMeta["Right Sidebar (Properties, Outlines, Backlinks, Revisions)"]
        end

        subgraph Modals ["Overlay Layers"]
            SearchModal["Search & Filter Modal (Ctrl + K)"]
            TermModal["Integrated Terminal Modal (xterm.js)"]
            ResearchModal["Research Papers & Annotations Library"]
            GraphModal["Interactive Full-Screen Graph View"]
        end
    end

    TopBar --> BodyArea
    TopBar -.-> SearchModal
    TopBar -.-> TermModal
    TopBar -.-> ResearchModal
    TopBar -.-> GraphModal
```

---

## 7. Security Architecture

1. **Loopback-Only Binding**: Server binds exclusively to `127.0.0.1`. The `localOnly` middleware verifies that the HTTP `Host` header resolves strictly to `127.0.0.1` or `localhost`.
2. **Cross-Origin Protection**: Rejects requests where the `Origin` header does not match the active server port, preventing malicious websites in other browser tabs from querying the API.
3. **HTTP Security Headers**: Every response sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and `Cache-Control: no-store`.
4. **Isolated Note Previews**: User-authored HTML and embedded scripts render within a separate `<iframe id="preview">` with sandboxing constraints to protect the management UI.
5. **Path Sandboxing**: All filesystem reads and writes pass through Go's `os.Root` boundary, blocking symlink escapes and directory traversal attacks.

---

## 8. Technology Stack Summary

| Layer               | Technologies                                                              |
| ------------------- | ------------------------------------------------------------------------- |
| Desktop Container   | Tauri v2, Rust 2021 edition                                               |
| Backend Server      | Go 1.25+ / Go 1.26, standard library `net/http`                           |
| Persistence & Index | SQLite 3 via `modernc.org/sqlite`, FTS5, WAL mode                         |
| Parsing & Watcher   | Goldmark (Markdown), `golang.org/x/net/html`, `fsnotify`                  |
| Terminal & PTY      | CrossPTY (`github.com/Kodecable/crosspty`), Gorilla WebSocket             |
| Local AI Search     | Ollama HTTP loopback API (optional)                                       |
| Frontend Core       | HTML5, Vanilla JavaScript (ES modules), CSS Custom Properties             |
| Frontend Assets     | MathJax 3 (`mathjax-full`), xterm.js (`@xterm/xterm`, `@xterm/addon-fit`) |
