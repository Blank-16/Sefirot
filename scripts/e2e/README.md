# Browser end-to-end checks

Drives a running SEFIROT server with Chromium (Playwright for Python) and writes screenshots.

    pip install playwright && python -m playwright install chromium
    ./sefirot --data-dir "$(mktemp -d)" --port 18080 &
    SEFIROT_URL=http://127.0.0.1:18080 SHOT_DIR=./shots python scripts/e2e/run.py

Run it against a fresh data directory: it creates and deletes notes and expects the seeded welcome note.
The exit code is non-zero if any check fails or the page logs a console error.
