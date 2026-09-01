#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd -- "${script_dir}/.." && pwd)"
frontend_dir="${project_dir}/front"

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
  echo "Frontend dependencies are missing. Run 'npm install' in the front directory first."
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker Desktop is not running. Please start Docker Desktop and try again."
  exit 1
fi

echo "Starting backend services..."
cd "${project_dir}"
docker compose up -d --build

echo "Gateway started: http://127.0.0.1:8080"

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
