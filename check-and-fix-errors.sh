#!/bin/bash

# Skript zum Überprüfen und Beheben häufiger Fehler in Go-Code

echo "Überprüfe Code auf häufige Fehler..."

# 1. Finde nicht überprüfte Rückgabewerte von Write-Operationen
echo "1. Suche nach nicht überprüften Write-Fehlern..."
files_with_unhandled_writes=$(grep -r --include="*.go" "\.Write(" . | grep -v "if.*err.*:=" | grep -v "if _, err.*:=" | grep -v "return" | grep -v "_.*=" | grep -v "err.*!=.*nil")

if [ -n "$files_with_unhandled_writes" ]; then
  echo "  Nicht überprüfte Write-Operationen gefunden in:"
  echo "$files_with_unhandled_writes"
  echo "  Bitte überprüfe diese Dateien und füge Fehlerbehandlung hinzu, z.B.:"
  echo "    if _, err := buffer.Write(data); err != nil {"
  echo "        return nil, err"
  echo "    }"
fi

# 2. Finde bytes.Compare statt bytes.Equal
echo "2. Suche nach suboptimalen bytes.Compare-Verwendungen..."
files_with_bytes_compare=$(grep -r --include="*.go" "bytes\.Compare.*!= 0" .)

if [ -n "$files_with_bytes_compare" ]; then
  echo "  bytes.Compare statt bytes.Equal gefunden in:"
  echo "$files_with_bytes_compare"
  echo "  Bitte ersetze mit !bytes.Equal(), z.B.:"
  echo "    if !bytes.Equal(a, b) { ... } // statt bytes.Compare(a, b) != 0"
fi

# 3. Prüfe auf ineffektuelle Zuweisungen
echo "3. Ineffektuelle Zuweisungen werden beim Kompilieren geprüft..."
# (lässt sich am besten mit 'go vet' oder 'ineffassign' prüfen)

# 4. Prüfe auf veralteten Code-Stil
echo "4. Suche nach veralteten Code-Konstrukten..."
files_with_old_style=$(grep -r --include="*.go" "new\(bytes\.Buffer\)" .)

if [ -n "$files_with_old_style" ]; then
  echo "  Veraltete Code-Konstrukte gefunden in:"
  echo "$files_with_old_style"
  echo "  Verwende &bytes.Buffer{} oder var buf bytes.Buffer"
fi

# 5. Laufe Go Vet zum Erkennen weiterer Probleme
echo "5. Führe go vet aus..."
go vet ./... || echo "  go vet hat Probleme gefunden. Bitte beheben."

echo ""
echo "Überprüfung abgeschlossen. Behebe gefundene Probleme und führe dann Linting aus:"
echo "  golangci-lint run"
