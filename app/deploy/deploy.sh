#!/usr/bin/env bash
set -euo pipefail

# Very simple deploy script for a server where git/docker are already installed.
REPO_DIR="/opt/featureban"
APP_DIR="${REPO_DIR}/app"

cd "$REPO_DIR"
git config --global --add safe.directory "$REPO_DIR" || true
git fetch origin main
git checkout -f main
git reset --hard origin/main

cd "$APP_DIR"
docker compose up --build -d --remove-orphans

docker compose ps
