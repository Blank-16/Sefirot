# Security model and operational precautions

SEFIROT is designed for one trusted user on one machine. It is not a multi-user server.

## Network boundary

- The HTTP server binds to `127.0.0.1` on the configured or automatically selected port.
- Host and Origin checks reject non-local requests and are tied to the selected port.
- Terminal sessions and the desktop shutdown endpoint use a random per-process token and loopback origin checks. The optional desktop ready file is created with owner-only permissions and contains the loopback URL plus that token.
- Do not expose the port through a LAN, reverse proxy, tunnel, or public internet.

## Terminal boundary

The terminal runs commands as the operating-system user that started SEFIROT. Unix-like systems use PTYs; Windows uses ConPTY (Windows 10 1809+). A CLI agent can read or modify files outside the configured vault directory, access environment variables, and use network access. The vault working directory is a convenience, not a sandbox. Use OS-level isolation or a dedicated low-privilege account if stronger containment is required.

CLI tools may transmit prompts, source files, or note content to their respective providers according to their own configuration and privacy policies. SEFIROT does not control that behavior.

## HTML preview boundary

Raw HTML and Markdown raw-HTML blocks are trusted executable content by design. They run inside an iframe with `sandbox="allow-scripts"` and without `allow-same-origin`, limiting access to the application shell. Do not import or preview untrusted HTML unless you understand its scripts and external resource requests.

## Filesystem boundary

- The vault root must be a real directory, not a symlink.
- Note and asset operations use `os.Root` plus validated relative paths.
- ZIP imports reject traversal, absolute paths, symlinks, oversized entries, and existing destinations.
- External processes launched from the terminal are not constrained by `os.Root`.

## Backups and recovery

Export the vault regularly. `index.db` is rebuildable, but it also contains revision history and optional embeddings; deleting it loses those derived records. Stop the application before manually deleting the database and its WAL files.

## Cross-platform data use

The HTML vault is portable, but do not run two SEFIROT processes against the same live vault simultaneously. Keep each installation’s SQLite database on a local filesystem; do not share one live WAL database between Windows and WSL or place it on a network share. WSL users should prefer the distribution’s Linux filesystem for active vaults.
