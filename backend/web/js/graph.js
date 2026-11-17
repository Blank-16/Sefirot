function findFirstNote(nodes) {
  for (const n of nodes) {
    if (n.type === "note") return n.path;
    const found = findFirstNote(n.children || []);
    if (found) return found;
  }
  return null;
}
function findPath(nodes, target) {
  for (const n of nodes) {
    if (n.path === target) return n;
    const found = findPath(n.children || [], target);
    if (found) return found;
  }
  return null;
}

// Knowledge graph: keyboard accessible node selection, pan/zoom, draggable nodes and relationship-aware labels.
async function openGraph() {
  if (state.dirty) await saveNote();
  state.graph.returnPage = state.page;
  state.page = "graph";
  $("#workspace").classList.add("hidden");
  $("#homeView").classList.add("hidden");
  $("#documentToolbar").classList.add("hidden");
  $("#rightSidebar").classList.add("hidden-pane");
  $("#emptyState").classList.add("hidden");
  $("#graphView").classList.remove("hidden");
  await loadGraph(state.currentPath);
}
async function loadGraph(focus = "") {
  const data = await api(
    `/api/graph${focus ? "?path=" + encodeURIComponent(focus) : ""}`,
  );
  state.graph.nodes = data.nodes || [];
  state.graph.edges = data.edges || [];
  state.graph.positions = new Map();
  state.graph.panX = 0;
  state.graph.panY = 0;
  state.graph.scale = 1;
  state.graph.selected =
    focus && state.graph.nodes.some((n) => n.id === focus)
      ? focus
      : state.graph.nodes[0]?.id || "";
  drawGraph();
}
function graphLayout() {
  const g = state.graph,
    nodes = g.nodes,
    selected = nodes.find((n) => n.id === g.selected);
  const maxRadius = Math.max(500, 360 + Math.sqrt(nodes.length) * 120),
    W = Math.max(1800, maxRadius * 2 + 300),
    H = Math.max(1200, maxRadius * 1.5 + 300),
    cx = W / 2,
    cy = H / 2,
    positions = new Map();
  if (!nodes.length) return { W, H, positions };
  const neighbors = new Set();
  g.edges.forEach((e) => {
    if (e.source === g.selected) neighbors.add(e.target);
    if (e.target === g.selected) neighbors.add(e.source);
  });
  if (selected) positions.set(selected.id, { x: cx, y: cy });
  const first = [...nodes]
    .filter((n) => n.id !== g.selected && neighbors.has(n.id))
    .sort((a, b) => b.degree - a.degree);
  const rest = [...nodes]
    .filter((n) => n.id !== g.selected && !neighbors.has(n.id))
    .sort((a, b) => b.degree - a.degree || a.title.localeCompare(b.title));
  const placeRing = (items, baseRadius, ringGap) => {
    let index = 0,
      ring = 0;
    while (index < items.length) {
      const radius = baseRadius + ring * ringGap;
      const capacity = Math.max(8, Math.floor((2 * Math.PI * radius) / 88));
      const count = Math.min(capacity, items.length - index);
      for (let i = 0; i < count; i++) {
        const n = items[index++],
          angle = (i / Math.max(count, 1)) * Math.PI * 2 + (ring % 2) * 0.13;
        positions.set(n.id, {
          x: cx + Math.cos(angle) * radius,
          y: cy + Math.sin(angle) * radius,
        });
      }
      ring++;
    }
  };
  placeRing(first, 150, 105);
  placeRing(rest, 330, 115);
  for (const [id, pos] of g.positions) {
    if (positions.has(id)) positions.set(id, pos);
  }
  g.positions = positions;
  return { W, H, positions, cx, cy };
}
function drawGraph() {
  const svg = $("#graphCanvas"),
    g = state.graph,
    nodes = g.nodes,
    edges = g.edges,
    selected = g.selected;
  if (!nodes.length) {
    svg.setAttribute("viewBox", "0 0 1000 700");
    svg.innerHTML =
      '<text x="500" y="340" text-anchor="middle" fill="currentColor">No indexed notes yet. Create notes and add wiki-links to connect them.</text>';
    $("#graphNodeTitle").textContent = "No documents";
    $("#graphNodePath").textContent = "";
    $("#graphNodeSummary").textContent = "";
    $("#graphNeighbors").textContent = "No connections yet.";
    $("#graphOpenNote").disabled = true;
    return;
  }
  const layout = graphLayout(),
    { W, H, positions } = layout,
    cx = W / 2,
    cy = H / 2,
    query = g.query.toLowerCase();
  const selectedEdges = edges.filter(
      (e) => e.source === selected || e.target === selected,
    ),
    neighborIDs = new Set();
  selectedEdges.forEach((e) => {
    neighborIDs.add(e.source);
    neighborIDs.add(e.target);
  });
  const visibleEdges = g.scope === "local" && selected ? selectedEdges : edges;
  const visibleNodes =
    g.scope === "local" && selected
      ? nodes.filter((n) => n.id === selected || neighborIDs.has(n.id))
      : nodes;
  const edgeMarkup = visibleEdges
    .map((e) => {
      const a = positions.get(e.source),
        b = positions.get(e.target);
      if (!a || !b) return "";
      const relation = escapeHTML(
        (e.types || [e.type || "related-to"]).join(" "),
      );
      return `<line class="graph-edge" data-relation="${relation}" x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}"/>`;
    })
    .join("");
  const nodeMarkup = visibleNodes
    .map((n) => {
      const p = positions.get(n.id),
        isSelected = n.id === selected,
        isNeighbor = neighborIDs.has(n.id),
        match =
          !query ||
          n.title.toLowerCase().includes(query) ||
          n.path.toLowerCase().includes(query) ||
          (n.tags || []).some((t) => t.includes(query));
      const fill = isSelected ? "#A88E52" : isNeighbor ? "#75816A" : "#8D8E85";
      const opacity = !match
        ? 0.08
        : selected && !isSelected && !isNeighbor
          ? 0.22
          : 1;
      const radius = isSelected ? 12 : Math.min(9, 5 + n.degree * 0.28);
      const label = n.title.length > 28 ? n.title.slice(0, 26) + "…" : n.title;
      return `<g class="graph-node ${isSelected ? "selected" : ""}" data-graph-id="${escapeHTML(n.id)}" tabindex="0" role="button" aria-label="${escapeHTML(n.title)}" opacity="${opacity}"><title>${escapeHTML(n.summaryShort || n.summary || n.title)} · ${escapeHTML(n.path)} · ${escapeHTML(n.type || "note")}</title><circle cx="${p.x}" cy="${p.y}" r="${radius}" fill="${fill}" stroke="${isSelected ? "#D7C18B" : "#64685F"}"/><text x="${p.x + 14}" y="${p.y + 4}">${escapeHTML(label)}</text></g>`;
    })
    .join("");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
  const tx = g.panX + cx * (1 - g.scale),
    ty = g.panY + cy * (1 - g.scale);
  svg.innerHTML = `<g id="graphWorld" transform="translate(${tx} ${ty}) scale(${g.scale})"><g>${edgeMarkup}</g><g>${nodeMarkup}</g></g>`;
  const chosen = nodes.find((n) => n.id === selected);
  $("#graphNodeTitle").textContent = chosen?.title || "Select a node";
  $("#graphNodePath").textContent = chosen?.path || "";
  $("#graphNodeSummary").textContent =
    chosen?.summary ||
    chosen?.summaryShort ||
    "A short document summary will appear here.";
  $("#graphNodeTags").innerHTML = (chosen?.tags || [])
    .map(
      (t) =>
        `<span class="rounded-full border border-zinc-800 px-2 py-1 text-[10px] text-zinc-500">${escapeHTML(t)}</span>`,
    )
    .join("");
  $("#graphOpenNote").disabled = !chosen;
  const neighborNodes = nodes.filter((n) => neighborIDs.has(n.id));
  $("#graphNeighbors").innerHTML = neighborNodes.length
    ? neighborNodes
        .map(
          (n) =>
            `<button class="block w-full rounded px-2 py-2 text-left hover:bg-zinc-800" data-graph-neighbor="${escapeHTML(n.id)}"><span class="block truncate text-zinc-300">${escapeHTML(n.title)}</span><span class="block truncate text-[10px] text-zinc-600">${escapeHTML(n.path)}</span></button>`,
        )
        .join("")
    : '<p class="text-zinc-600">No linked documents. Add [[wiki-links]] to connect this note.</p>';
}
function selectGraphNode(id) {
  state.graph.selected = id;
  drawGraph();
}
$("#graphOpen").addEventListener("click", () => openGraph().catch(showError));
$("#graphClose").addEventListener("click", () => {
  $("#graphView").classList.add("hidden");
  if (state.graph.returnPage === "home" || !state.currentPath)
    openHome().catch(showError);
  else {
    state.page = "notes";
    $("#documentToolbar").classList.remove("hidden");
    if (state.rightOpen) $("#rightSidebar").classList.remove("hidden-pane");
    $("#workspace").classList.remove("hidden");
  }
});
$("#graphReset").addEventListener("click", () => {
  state.graph.panX = 0;
  state.graph.panY = 0;
  state.graph.scale = 1;
  state.graph.positions = new Map();
  state.graph.query = "";
  $("#graphSearch").value = "";
  drawGraph();
});
$("#graphZoomIn").addEventListener("click", () => {
  state.graph.scale = Math.min(3, state.graph.scale * 1.2);
  drawGraph();
});
$("#graphZoomOut").addEventListener("click", () => {
  state.graph.scale = Math.max(0.25, state.graph.scale / 1.2);
  drawGraph();
});
$("#graphSearch").addEventListener("input", (e) => {
  state.graph.query = e.target.value;
  drawGraph();
});
$("#graphScope").addEventListener("change", (e) => {
  state.graph.scope = e.target.value;
  drawGraph();
});
$("#graphCanvas").addEventListener("click", (e) => {
  const n = e.target.closest("[data-graph-id]");
  if (n) selectGraphNode(n.dataset.graphId);
});
$("#graphCanvas").addEventListener("keydown", (e) => {
  const n = e.target.closest("[data-graph-id]");
  if (n && (e.key === "Enter" || e.key === " ")) {
    e.preventDefault();
    selectGraphNode(n.dataset.graphId);
  }
});
$("#graphNeighbors").addEventListener("click", (e) => {
  const n = e.target.closest("[data-graph-neighbor]");
  if (n) selectGraphNode(n.dataset.graphNeighbor);
});
$("#graphOpenNote").addEventListener("click", async () => {
  const p = state.graph.selected;
  if (!p) return;
  $("#graphView").classList.add("hidden");
  state.page = "notes";
  try {
    await openNote(p);
  } catch (e) {
    showError(e);
  }
});
const graphSVG = $("#graphCanvas");
graphSVG.addEventListener(
  "wheel",
  (e) => {
    e.preventDefault();
    state.graph.scale = Math.max(
      0.25,
      Math.min(3, state.graph.scale * (e.deltaY < 0 ? 1.12 : 0.89)),
    );
    drawGraph();
  },
  { passive: false },
);
graphSVG.addEventListener("pointerdown", (e) => {
  const node = e.target.closest("[data-graph-id]");
  state.graph.drag = {
    kind: node ? "node" : "pan",
    id: node?.dataset.graphId || "",
    x: e.clientX,
    y: e.clientY,
    moved: false,
  };
  graphSVG.setPointerCapture(e.pointerId);
});
graphSVG.addEventListener("pointermove", (e) => {
  const d = state.graph.drag;
  if (!d) return;
  const dx = e.clientX - d.x,
    dy = e.clientY - d.y;
  if (Math.abs(dx) + Math.abs(dy) > 2) d.moved = true;
  const rect = graphSVG.getBoundingClientRect(),
    view = graphSVG.viewBox.baseVal,
    sx = view.width / rect.width,
    sy = view.height / rect.height;
  if (d.kind === "pan") {
    state.graph.panX += dx * sx;
    state.graph.panY += dy * sy;
  } else {
    const pos = state.graph.positions.get(d.id);
    if (pos) {
      pos.x += (dx * sx) / state.graph.scale;
      pos.y += (dy * sy) / state.graph.scale;
    }
  }
  d.x = e.clientX;
  d.y = e.clientY;
  if (d.moved) drawGraph();
});
graphSVG.addEventListener("pointerup", () => {
  state.graph.drag = null;
});
graphSVG.addEventListener("pointercancel", () => {
  state.graph.drag = null;
});

$("#aiContextCopy").addEventListener("click", async () => {
  if (!state.currentPath) {
    showError(new Error("Open a note first."));
    return;
  }
  try {
    const ctx = await api(
      "/api/ai/context?path=" + encodeURIComponent(state.currentPath),
    );
    const text =
      JSON.stringify(ctx, null, 2) +
      "\n\nTask: Use archive summaries and links to identify relevant notes. Read only necessary full documents. When creating a new HTML document, add meaningful [[wiki-links]] to related notes. Save it in the active vault; the watcher will index it automatically.";
    await navigator.clipboard.writeText(text);
    $("#globalStatus").textContent = "Archive context copied";
  } catch (e) {
    showError(e);
  }
});

// Document revision history is stored in SQLite; restoring writes the old HTML back to the vault.
