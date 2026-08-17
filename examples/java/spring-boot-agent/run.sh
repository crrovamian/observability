#!/bin/bash
# Descarga el agente Java de OpenTelemetry y lo inyecta sin tocar código
AGENT_URL="https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar"
AGENT_FILE="opentelemetry-javaagent.jar"

if [ ! -f "$AGENT_FILE" ]; then
  echo "Descargando opentelemetry-javaagent.jar..."
  curl -L -o "$AGENT_FILE" "$AGENT_URL"
fi

java -javaagent:$AGENT_FILE \
     -Dotel.service.name=demo-spring-agent \
     -Dotel.traces.exporter=otlp \
     -Dotel.exporter.otlp.endpoint=http://otel-collector:4318 \
     -jar my-app.jar
