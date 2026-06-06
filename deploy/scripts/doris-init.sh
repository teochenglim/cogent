#!/bin/sh
# Doris one-time bootstrap: wait for FE ready, wait for BE alive, apply schema.
# Modelled on doris-play/scripts/00_wait.sh — 80 × 3 s = 240 s max per stage.
set -e

MYSQL="mysql -h doris-fe -P 9030 -u root --connect-timeout 2"
MAX=80

# ── 1. Wait for FE MySQL port ─────────────────────────────────────────────
echo "Waiting for Doris FE at doris-fe:9030 ..."
i=0
until $MYSQL -e "SELECT 1" >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge "$MAX" ]; then
    echo "ERROR: FE did not become ready after $((MAX * 3))s" >&2
    exit 1
  fi
  sleep 3
done
echo "FE ready (attempt $i)."

# ── 2. Register BE — fallback; dyrnq image auto-registers via RUN_MODE=BE ─
$MYSQL -e "ALTER SYSTEM ADD BACKEND 'doris-be:9050';" 2>/dev/null || true

# ── 3. Wait for at least one BE alive ────────────────────────────────────
echo "Waiting for BE to come alive ..."
i=0
while true; do
  alive=$($MYSQL -sNe 'SHOW BACKENDS' 2>/dev/null | awk -F'\t' '{print $10}' | grep -c 'true' || echo 0)
  [ "$alive" -ge 1 ] && break
  i=$((i + 1))
  if [ "$i" -ge "$MAX" ]; then
    echo "ERROR: No backend became alive after $((MAX * 3))s" >&2
    exit 1
  fi
  sleep 3
done
echo "BE alive (attempt $i)."

# ── 4. Apply schema ───────────────────────────────────────────────────────
$MYSQL < /schema/doris.sql
echo "Doris schema applied. Init complete."
