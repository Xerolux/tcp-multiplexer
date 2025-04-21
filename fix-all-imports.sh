#!/bin/bash

# Verzeichnis und alle Go-Dateien aktualisieren
find . -name "*.go" -type f -exec sed -i 's|github.com/ingmarstein/tcp-multiplexer|github.com/Xerolux/tcp-multiplexer|g' {} \;

# .goreleaser.yml aktualisieren
if [ -f ".goreleaser.yml" ]; then
  sed -i 's|github.com/ingmarstein/tcp-multiplexer|github.com/Xerolux/tcp-multiplexer|g' .goreleaser.yml
fi

# Compose-Datei aktualisieren
if [ -f "compose.yml" ]; then
  sed -i 's|ingmarstein/tcp-multiplexer|xerolux/tcp-multiplexer|g' compose.yml
  sed -i 's|ghcr.io/ingmarstein/tcp-multiplexer|ghcr.io/xerolux/tcp-multiplexer|g' compose.yml
fi

# Überprüfe Dockerfile
if [ -f "Dockerfile" ]; then
  grep -q "tcp-multiplexer" Dockerfile && echo "Beachte: Dockerfile könnte Anpassungen benötigen"
fi

# Aktualisiere go.mod
go mod edit -module github.com/Xerolux/tcp-multiplexer

# Dependencies aktualisieren
go mod tidy

echo "---------------------------------------------"
echo "Importpfade wurden aktualisiert. Bitte überprüfe:"
echo "1. Das Build funktioniert: go build"
echo "2. Die Tests laufen: go test ./..."
echo "---------------------------------------------"
