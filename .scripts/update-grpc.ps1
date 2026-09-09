$ErrorActionPreference = 'Stop'
# Requires protoc 35.1, protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.2 on PATH.
Push-Location (Join-Path $PSScriptRoot '../backend')
try {
    protoc --go_out=. --go_opt=module=erent --go-grpc_out=. --go-grpc_opt=module=erent proto/upstream.proto
    if ($LASTEXITCODE -ne 0) { throw 'Protobuf generation failed.' }
} finally {
    Pop-Location
}
