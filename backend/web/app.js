"use strict";

// Compatibility loader for clients that still request /app.js.
// The current index.html loads these feature scripts directly with defer.
const featureScripts = [
  "core",
  "notes",
  "research",
  "references",
  "authoring",
  "ui",
  "search",
  "events",
  "graph",
  "history",
  "commands",
  "terminal",
  "formatter",
  "settings",
  "maintenance",
];
for (const feature of featureScripts) {
  const script = document.createElement("script");
  script.src = `/js/${feature}.js`;
  script.async = false;
  document.head.append(script);
}
