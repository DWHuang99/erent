#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd -- "${script_dir}/.." && pwd)"
frontend_dir="${project_dir}/frontend"
compose_file="${project_dir}/backend/docker-compose.yml"
debug_compose_file="${script_dir}/docker-compose.debug.yml"
env_file="${project_dir}/.env"

usage() {
  echo "Usage: bash .scripts/start.sh [debug|--debug]"
  echo "  no option  Start the complete Docker stack and the Vite dev server."
  echo "  debug      Start only PostgreSQL, Redis, migrations, and Vite; run Go services from the IDE."
}

mode="normal"
if [[ $# -gt 1 ]]; then
  usage >&2
  exit 2
fi

case "${1:-}" in
  "") ;;
  debug|--debug) mode="debug" ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    echo "Unknown option: $1" >&2
    usage >&2
    exit 2
    ;;
esac

command -v docker >/dev/null 2>&1 || {
  echo "Docker command was not found. Please install Docker Desktop first."
  exit 1
}

is_wsl=false
if [[ -r /proc/sys/kernel/osrelease ]] \
  && grep -qi microsoft /proc/sys/kernel/osrelease; then
  is_wsl=true
fi

# The repository can be shared by Windows and WSL. When node_modules was
# installed by Windows npm, keep using the Windows runtime from WSL so Vite's
# native optional dependencies match the installed platform.
if [[ "${is_wsl}" == true ]] \
  && command -v node.exe >/dev/null 2>&1 \
  && command -v npm.cmd >/dev/null 2>&1 \
  && command -v cmd.exe >/dev/null 2>&1; then
  use_windows_node=true
elif command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
  use_windows_node=false
elif command -v node.exe >/dev/null 2>&1 \
  && command -v npm.cmd >/dev/null 2>&1 \
  && command -v cmd.exe >/dev/null 2>&1; then
  use_windows_node=true
else
  echo "Node.js and npm commands were not found. Please install them first."
  exit 1
fi

if [[ ! -d "${frontend_dir}/node_modules" ]]; then
  echo "Frontend dependencies are missing. Run 'npm install' in the frontend directory first."
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker Desktop is not running. Please start Docker Desktop and try again."
  exit 1
fi

compose=(docker compose --env-file "${env_file}" -f "${compose_file}")

if [[ "${mode}" == "debug" ]]; then
  debug_compose=("${compose[@]}" -f "${debug_compose_file}")

  echo "Preparing backend infrastructure for IDE debugging..."
  "${compose[@]}" stop web gateway api upstream
  "${debug_compose[@]}" up -d postgres redis
  "${debug_compose[@]}" run --rm migrate

  echo "Backend infrastructure is ready: PostgreSQL 127.0.0.1:5432, Redis 127.0.0.1:6379"
  echo "Start backend/cmd/upstream and backend/cmd/api in the IDE debugger."
  echo "The local API should listen on 127.0.0.1:8080 and connect to upstream at 127.0.0.1:50051."
else
  echo "Starting backend services..."
  "${compose[@]}" up -d --build

  echo "Gateway started: http://127.0.0.1:8080"
  echo "Nginx frontend started (default: http://127.0.0.1:8088)"
fi

if command -v curl >/dev/null 2>&1 \
  && curl --noproxy "*" --connect-timeout 2 --max-time 3 --fail --silent \
    http://127.0.0.1:5173/ >/dev/null 2>&1; then
  echo "Frontend is already running: http://127.0.0.1:5173"
  exit 0
fi

echo "Starting frontend: http://127.0.0.1:5173"
echo "Press Ctrl+C to stop the frontend dev server. Backend containers will keep running."

if [[ "${use_windows_node}" == true ]]; then
  frontend_windows_dir="$(wslpath -w "${frontend_dir}")"
  exec cmd.exe /d /c "npm.cmd --prefix \"${frontend_windows_dir}\" run dev"
else
  cd "${frontend_dir}"
  exec npm run dev
fi
