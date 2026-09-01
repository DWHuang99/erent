#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd -- "${script_dir}/.." && pwd)"
compose_file="${project_dir}/backend/docker-compose.yml"
env_file="${project_dir}/.env"

command -v docker >/dev/null 2>&1 || {
  echo "Docker command was not found. Please install Docker Desktop first."
  exit 1
}

if ! docker info >/dev/null 2>&1; then
  echo "Docker Desktop is not running. Please start Docker Desktop and try again."
  exit 1
fi

echo "Starting PostgreSQL..."
docker compose --env-file "${env_file}" -f "${compose_file}" up -d postgres

echo "Applying pending database migrations..."
docker compose --env-file "${env_file}" -f "${compose_file}" run --rm migrate

echo "Database update completed."
