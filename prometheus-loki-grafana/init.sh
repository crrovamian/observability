#!/bin/bash
# Crea las carpetas data/ necesarias para los volúmenes bind-mount
# Ejecutar antes de docker compose up

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Creando carpetas de datos..."
mkdir -p "$SCRIPT_DIR/data/prometheus"
mkdir -p "$SCRIPT_DIR/data/grafana"
mkdir -p "$SCRIPT_DIR/data/loki"
mkdir -p "$SCRIPT_DIR/data/tempo"
mkdir -p "$SCRIPT_DIR/data/otel-collector"

echo "Carpetas creadas:"
ls -d "$SCRIPT_DIR/data"/*/
echo ""
echo "Listo. Ahora podés ejecutar: docker compose up -d"
