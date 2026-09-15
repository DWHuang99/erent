#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_dir="$(cd -- "${script_dir}/../backend" && pwd)"

required_commands=(protoc protoc-gen-go protoc-gen-go-grpc)
for required_command in "${required_commands[@]}"; do
  if ! command -v "${required_command}" >/dev/null 2>&1; then
    echo "Required command was not found: ${required_command}" >&2
    echo "Expected versions: protoc 35.1, protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.2." >&2
    exit 1
  fi
done

echo "Generating Go protobuf and gRPC code from backend/proto/upstream.proto..."
cd "${backend_dir}"
protoc \
  --go_out=. \
  --go_opt=module=erent \
  --go-grpc_out=. \
  --go-grpc_opt=module=erent \
  proto/upstream.proto

echo "Updated internal/rpc/upstream/upstream.pb.go and upstream_grpc.pb.go."
