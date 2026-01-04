#!/bin/bash
set -e

# Script to generate protobuf code

echo "Generating protobuf code..."

# Ensure proto directory exists
mkdir -p proto/fs

# Generate Go code from proto files
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/filesystem.proto

echo "✓ Protobuf code generated successfully"
echo "  Generated files:"
echo "    - proto/fs/filesystem.pb.go"
echo "    - proto/fs/filesystem_grpc.pb.go"

