#!/bin/sh
# Esegue i test e compila dist/ApkTool.exe (funziona anche da Linux/macOS).
set -e
cd "$(dirname "$0")"
go vet ./...
go test ./...
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/ApkTool.exe .
echo "Fatto: dist/ApkTool.exe"
