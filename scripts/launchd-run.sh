#!/bin/bash
# launchd-run.sh — the entry point com.edvin.tract.plist actually executes.
#
# WHY A WRAPPER EXISTS AT ALL
# ---------------------------
# The plist used to exec bin/tract directly with no TRACT_TOKEN, and its own comment
# said so: "LAN-open by design". Measured 2026-08-10, 2026-08-16 and again 2026-08-20,
# that was not a localhost service — the listener is *:8080 (IPv6 wildcard, i.e. every
# interface), so an anonymous POST /api/items from anywhere on the home network reached
# the handler. tract's POST also performs a server-side fetch of the submitted URL, so
# the open instance was a writable backend AND an SSRF proxy, not merely a readable one.
#
# The token cannot live in the plist: EnvironmentVariables is plaintext in a file that
# gets read, diffed and pasted. It lives in ~/.config/tract/token, mode 0600, outside
# every git tree — the standard shape for an operator token on a single-user host.
#
# REFUSING TO START IS THE POINT
# ------------------------------
# If the token file is missing or empty this script exits WITHOUT starting tract. That
# is deliberate: main.go treats an empty TRACT_TOKEN as "every route is open" and only
# prints a warning, so a fallback-to-unprotected start would silently restore the exact
# exposure this file exists to close — and it would do it quietly, at boot, months later.
# KeepAlive would otherwise spin on the failure, so the sleep throttles the restart loop
# and leaves one legible line per attempt in ~/Library/Logs/tract.log.
set -euo pipefail

TOKEN_FILE="${TRACT_TOKEN_FILE:-$HOME/.config/tract/token}"

if [ ! -s "$TOKEN_FILE" ]; then
  echo "FATAL $(date '+%Y-%m-%d %H:%M:%S'): $TOKEN_FILE is missing or empty — refusing to" \
       "start, because starting without TRACT_TOKEN would reopen POST /api/items to the LAN." \
       "Fix: umask 077 && mkdir -p $(dirname "$TOKEN_FILE") && openssl rand -hex 32 > $TOKEN_FILE"
  sleep 30   # throttle launchd's KeepAlive restart loop
  exit 1
fi

TRACT_TOKEN="$(tr -d '[:space:]' < "$TOKEN_FILE")"
export TRACT_TOKEN
export TRACT_DB="${TRACT_DB:-$HOME/vibin/tract/tract.db}"

exec "$(dirname "$0")/../bin/tract"
