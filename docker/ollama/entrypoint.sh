#!/bin/sh
# Ollama entrypoint: start server, auto-pull the embedding model on first run.
set -e

ollama serve &
OLLAMA_PID=$!

# Wait for Ollama to become ready (max 60s) using the CLI (no curl dependency).
i=0
while [ $i -lt 60 ]; do
  if ollama list > /dev/null 2>&1; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

# Pull the embedding model if not already present (persisted in volume).
if ! ollama list 2>/dev/null | grep -q "nomic-embed-text"; then
  echo "[ollama] pulling nomic-embed-text (~274MB)..."
  ollama pull nomic-embed-text
fi

wait $OLLAMA_PID
