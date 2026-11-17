let terminal = {
  term: null,
  fit: null,
  socket: null,
  id: "",
  token: "",
  closing: false,
};
function setTheme(theme) {
  if (theme !== "dark" && theme !== "light") theme = "dark";
  document.documentElement.dataset.theme = theme;
  document.documentElement.classList.toggle("dark", theme === "dark");
  document.documentElement.classList.toggle("light", theme === "light");
  const meta = document.querySelector('meta[name="color-scheme"]');
  if (meta) meta.content = theme;
  try {
    localStorage.setItem("sefirot-theme", theme);
  } catch {}
  if (typeof queueSettingsSync === "function") queueSettingsSync();
  if (typeof state !== "undefined" && state.currentPath) renderPreview();
  $("#themeToggle").textContent = theme === "dark" ? "◐" : "◑";
}
setTheme(
  localStorage.getItem("sefirot-theme") ||
    localStorage.getItem("html-vault-theme") ||
    "dark",
);
$("#themeToggle").addEventListener("click", () =>
  setTheme(
    document.documentElement.dataset.theme === "dark" ? "light" : "dark",
  ),
);

$("#backupExport").addEventListener("click", async () => {
  try {
    if (state.dirty) await saveNote();
    window.location.href = "/api/export";
  } catch (e) {
    showError(e);
  }
});
$("#backupImport").addEventListener("click", () => $("#importInput").click());
$("#importInput").addEventListener("change", async (event) => {
  const file = event.target.files?.[0];
  if (!file) return;
  if (
    !confirm(
      `Import ${file.name}? Existing files will not be overwritten. Only import archives you trust: HTML notes may execute JavaScript in the isolated preview.`,
    )
  ) {
    event.target.value = "";
    return;
  }
  try {
    const response = await fetch("/api/import", {
      method: "POST",
      headers: { "Content-Type": "application/zip" },
      body: file,
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok)
      throw new Error(result.error || `Import failed (${response.status})`);
    await refreshTree();
    $("#globalStatus").textContent =
      `Imported ${result.count} files · skipped ${result.skipped?.length || 0}${result.indexErrors?.length ? ` · ${result.indexErrors.length} could not be indexed` : ""}`;
    $("#globalStatus").classList.remove("hidden");
  } catch (e) {
    showError(e);
  } finally {
    event.target.value = "";
  }
});

const TERMINAL_ASSETS = {
  css: [
    "/vendor/xterm.css",
    "https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.css",
  ],
  scripts: [
    [
      "/vendor/xterm.js",
      "https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.js",
    ],
    [
      "/vendor/addon-fit.js",
      "https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/lib/addon-fit.js",
    ],
  ],
};
let terminalAssets = null;

// A failed <script> or <link> cannot be retried by editing its src/href, so each
// candidate gets a fresh element; the remote copy is used only if the bundled one is missing.
function loadElement(create, urls) {
  return new Promise((resolve, reject) => {
    const attempt = (index) => {
      if (index >= urls.length) {
        reject(new Error("could not load " + urls[0]));
        return;
      }
      const element = create(urls[index]);
      element.onload = () => resolve(urls[index]);
      element.onerror = () => {
        element.remove();
        attempt(index + 1);
      };
      document.head.appendChild(element);
    };
    attempt(0);
  });
}

function ensureTerminalAssets() {
  if (window.Terminal && window.FitAddon) return Promise.resolve("bundled");
  if (!terminalAssets) {
    terminalAssets = (async () => {
      const styles = await loadElement(
        (href) =>
          Object.assign(document.createElement("link"), {
            rel: "stylesheet",
            href,
          }),
        TERMINAL_ASSETS.css,
      );
      let source = styles;
      for (const urls of TERMINAL_ASSETS.scripts)
        source = await loadElement(
          (src) => Object.assign(document.createElement("script"), { src }),
          urls,
        );
      return source;
    })().catch((error) => {
      terminalAssets = null;
      throw error;
    });
  }
  return terminalAssets;
}

function applyTerminalProfiles(cfg) {
  const select = $("#terminalProfile");
  const known = new Map(cfg.profiles.map((p) => [p.id, p]));
  [...select.options].forEach((option) => {
    const profile = known.get(option.value);
    if (!profile) {
      option.remove();
      return;
    }
    option.disabled = profile.available !== "true";
    option.title = option.disabled ? "Executable not found on server PATH" : "";
  });
  if (!select.value || select.selectedOptions[0]?.disabled)
    select.value = cfg.defaultProfile || "shell";
}

async function openTerminal() {
  $("#terminalOverlay").classList.remove("hidden");
  $("#terminalOverlay").classList.add("flex");
  const cfg = await api("/api/terminal/config");
  terminal.token = cfg.token;
  applyTerminalProfiles(cfg);
  if (!terminal.term) {
    try {
      const source = await ensureTerminalAssets();
      if (typeof source === "string" && source.startsWith("http"))
        $("#terminalStatus").textContent =
          "Terminal library loaded from cdn.jsdelivr.net (bundled copy missing)";
    } catch {
      $("#terminalStatus").textContent =
        "Terminal library unavailable: the bundled copy is missing (run npm run frontend:build) and the fallback could not be reached.";
      $("#terminalStart").disabled = true;
      return;
    }
    $("#terminalStart").disabled = false;
    terminal.term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
      fontSize: 13,
      theme: {
        background: "#000000",
        foreground: "#f5f5f5",
        cursor: "#f5f5f5",
        selectionBackground: "#333333",
      },
    });
    terminal.term.open($("#terminalOutput"));
    if (window.FitAddon) {
      terminal.fit = new FitAddon.FitAddon();
      terminal.term.loadAddon(terminal.fit);
      terminal.fit.fit();
    }
    terminal.term.onData((data) => {
      if (terminal.socket && terminal.socket.readyState === WebSocket.OPEN)
        terminal.socket.send(new TextEncoder().encode(data));
    });
    terminal.term.onResize(sendTerminalResize);
    terminal.term.writeln(
      "\x1b[90mSEFIROT terminal · Select a profile and start a session.\x1b[0m",
    );
  }
  setTimeout(() => {
    if (terminal.fit) terminal.fit.fit();
    if (terminal.term) terminal.term.focus();
  }, 30);
  if (!terminal.socket) {
    try {
      await reattachTerminal();
    } catch {
      /* no running session; the user can start one */
    }
  }
}
const TERMINAL_SESSION_KEY = "sefirot-terminal-session";
const rememberTerminal = (id) => {
  try {
    id
      ? sessionStorage.setItem(TERMINAL_SESSION_KEY, id)
      : sessionStorage.removeItem(TERMINAL_SESSION_KEY);
  } catch {}
};
const rememberedTerminal = () => {
  try {
    return sessionStorage.getItem(TERMINAL_SESSION_KEY) || "";
  } catch {
    return "";
  }
};

function connectTerminal(id, { reattached = false } = {}) {
  terminal.id = id;
  rememberTerminal(id);
  if (terminal.socket) {
    const old = terminal.socket;
    terminal.socket = null;
    old.close();
  }
  if (reattached && terminal.term) terminal.term.reset();
  const scheme = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(
    `${scheme}//${location.host}/api/terminal/ws?id=${encodeURIComponent(id)}&token=${encodeURIComponent(terminal.token)}`,
  );
  ws.binaryType = "arraybuffer";
  terminal.socket = ws;
  ws.onopen = () => {
    $("#terminalStatus").textContent = reattached
      ? "Reconnected to running session"
      : "Connected";
    terminal.term.focus();
    sendTerminalResize();
  };
  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer)
      terminal.term.write(new Uint8Array(event.data));
    else terminal.term.write(event.data);
  };
  ws.onclose = (event) => {
    if (terminal.socket !== ws) return;
    terminal.socket = null;
    if (event.reason === "process ended") {
      terminal.id = "";
      rememberTerminal("");
      $("#terminalStatus").textContent = "Session ended";
    } else
      $("#terminalStatus").textContent =
        "Disconnected (session keeps running; reopen the terminal to reattach)";
    refreshTerminalIndicator();
    if (!terminal.closing && state.page === "home") openHome().catch(() => {});
  };
  ws.onerror = () => {
    $("#terminalStatus").textContent = "Connection error";
  };
}

async function liveTerminalSessions() {
  if (!terminal.token)
    terminal.token = (await api("/api/terminal/config")).token;
  const data = await api("/api/terminal/sessions", {
    headers: { "X-Vault-Token": terminal.token },
  });
  return data.sessions;
}

async function refreshTerminalIndicator() {
  const button = document.querySelector("#terminalOpen");
  if (!button) return;
  try {
    const sessions = await liveTerminalSessions();
    button.textContent = sessions.length
      ? `Terminal (${sessions.length} running)`
      : "Terminal";
  } catch {
    button.textContent = "Terminal";
  }
}

async function startTerminal() {
  try {
    if (terminal.socket) {
      const old = terminal.socket;
      terminal.socket = null;
      old.close();
    }
    if (terminal.term) terminal.term.clear();
    $("#terminalStatus").textContent = "Starting…";
    const started = await api("/api/terminal/start", {
      method: "POST",
      headers: { "X-Vault-Token": terminal.token },
      body: JSON.stringify({ profile: $("#terminalProfile").value }),
    });
    connectTerminal(started.id);
  } catch (e) {
    $("#terminalStatus").textContent = e.message;
  }
}

async function reattachTerminal() {
  const sessions = await liveTerminalSessions();
  const remembered = rememberedTerminal();
  const target =
    sessions.find((s) => s.id === remembered) ||
    sessions.find((s) => !s.attached) ||
    sessions[sessions.length - 1];
  if (!target) {
    rememberTerminal("");
    return false;
  }
  if ($("#terminalProfile").querySelector(`option[value="${target.profile}"]`))
    $("#terminalProfile").value = target.profile;
  connectTerminal(target.id, { reattached: true });
  return true;
}
function sendTerminalResize() {
  if (terminal.fit) terminal.fit.fit();
  if (
    terminal.socket &&
    terminal.socket.readyState === WebSocket.OPEN &&
    terminal.term
  )
    terminal.socket.send(
      JSON.stringify({
        type: "resize",
        cols: terminal.term.cols,
        rows: terminal.term.rows,
      }),
    );
}
async function closeTerminal() {
  $("#terminalOverlay").classList.add("hidden");
  $("#terminalOverlay").classList.remove("flex");
  const id = terminal.id;
  const socket = terminal.socket;
  terminal.closing = true;
  terminal.socket = null;
  terminal.id = "";
  rememberTerminal("");
  if (socket) socket.close();
  if (id && terminal.token) {
    try {
      await fetch("/api/terminal/stop", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Vault-Token": terminal.token,
        },
        body: JSON.stringify({ id }),
      });
    } catch {}
  }
  $("#terminalStatus").textContent = "Not connected";
  terminal.closing = false;
  if (state.page === "home") openHome().catch(() => {});
}
$("#terminalOpen").addEventListener("click", () =>
  openTerminal().catch(showError),
);
$("#terminalStart").addEventListener("click", startTerminal);
function hideTerminal() {
  $("#terminalOverlay").classList.add("hidden");
  $("#terminalOverlay").classList.remove("flex");
}
$("#terminalClose").addEventListener("click", hideTerminal);
$("#terminalStop").addEventListener("click", () => closeTerminal());
$("#terminalOverlay").addEventListener("click", (event) => {
  if (event.target === $("#terminalOverlay")) hideTerminal();
});
window.addEventListener("resize", sendTerminalResize);

refreshTerminalIndicator();
