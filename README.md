# Observability Stack — Prometheus + Loki + Grafana

Stack local de observabilidad con Docker Compose. Proporciona métricas (Prometheus), logs (Loki) y dashboards (Grafana) para tus contenedores, más exporters de sistema.

## Servicios

| Servicio        | Imagen                                       | Puerto  | Propósito                        |
|-----------------|----------------------------------------------|---------|----------------------------------|
| `nginx-demo`    | `nginx:alpine`                               | `80`    | App demo para generar tráfico    |
| `prometheus`    | `prom/prometheus:latest`                     | `9090`  | Métricas y alertas               |
| `loki`          | `grafana/loki:latest`                        | `3100`  | Agregación de logs               |
| `promtail`      | `grafana/promtail:latest`                    | —       | Shipping de logs a Loki          |
| `grafana`       | `grafana/grafana:latest`                     | `3001`  | Dashboards (métricas + logs)     |
| `cadvisor`      | `gcr.io/cadvisor/cadvisor:latest`            | `8080`  | Métricas de contenedores         |
| `node-exporter` | `prom/node-exporter:latest`                  | `9100`  | Métricas del host                |
| `otel-collector`| `otel/opentelemetry-collector-contrib:latest`| `4317`  | Receptor OTLP, reenvío a Jaeger  |
| `jaeger`        | `jaegertracing/all-in-one:latest`            | `16686` | Backend y UI de trazas           |

## Requisitos

- Docker + Compose V2 (plugin `docker compose`)
- Linux (bind-mounts de `/proc`, `/sys`, `/var/run/docker.sock`, etc.)
- 2–4 GB de RAM libre

## Inicio rápido

```bash
cd prometheus-loki-grafana
mkdir -p data/prometheus data/grafana data/loki data/otel-collector
docker compose up -d
```

## Acceso

| UI          | URL                           | Credenciales        |
|-------------|-------------------------------|---------------------|
| Grafana     | http://localhost:3001         | `admin` / `admin`   |
| Prometheus  | http://localhost:9090         | —                   |
| Jaeger      | http://localhost:16686        | —                   |
| cAdvisor    | http://localhost:8080         | —                   |
| Loki API    | http://localhost:3100/ready   | —                   |

## Logs (Loki)

Promtail descubre automáticamente contenedores con la label `logging=promtail` a través del socket de Docker. No importa si el contenedor está en otro `compose.yaml` o es un `docker run` independiente:

```yaml
labels:
  logging: "promtail"
```

```bash
docker run --label logging=promtail mi-imagen
```

No necesita estar en la red `monitoring` — Promtail lee los logs del socket directamente.

## Métricas (Prometheus)

Para que Prometheus escale métricas de un servicio propio:

1. El servicio debe exponer un endpoint `/metrics` con formato Prometheus.
2. Debe estar en la red `monitoring`.
3. Agregarlo en `prometheus/prometheus.yaml`:

```yaml
- job_name: 'mi-servicio'
  static_configs:
    - targets: ['mi-servicio:8080']
```

4. Recargar Prometheus sin reiniciar:

```bash
curl -X POST http://localhost:9090/-/reload
```

## Trazas (Jaeger)

Las aplicaciones envían trazas vía **OTLP** al `otel-collector` (puerto `4317`), que las reenvía a Jaeger para almacenamiento y visualización. Jaeger está configurado como datasource en Grafana para correlacionar métricas, logs y trazas.

### Enviar trazas desde tu aplicación

Agrega el SDK de OpenTelemetry en tu lenguaje y apunta al endpoint OTLP:

| Lenguaje   | Endpoint OTLP                    | SDK                                                        |
|------------|----------------------------------|------------------------------------------------------------|
| Python     | `otel-collector:4317` (gRPC)     | `opentelemetry-exporter-otlp-proto-grpc`                   |
| Go         | `otel-collector:4317` (gRPC)     | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` |
| Java       | `http://otel-collector:4318`     | `opentelemetry-exporter-otlp` (o agente Java)              |
| Node.js    | `otel-collector:4317` (gRPC)     | `@opentelemetry/exporter-trace-otlp-grpc`                  |

> **Requiere**: el contenedor debe estar en la red `monitoring` de Docker.

### Servicio en otro `compose.yaml`

```yaml
networks:
  monitoring:
    external: true

services:
  mi-app:
    image: mi-app
    networks:
      - monitoring
    labels:
      logging: "promtail"
```

## Persistencia

Los datos se almacenan en `./data/` mediante bind-mounts locales. Las carpetas deben existir antes de levantar los servicios (el `mkdir -p` del inicio rápido las crea automáticamente).

| Componente     | Ruta                   | Retención |
|----------------|------------------------|-----------|
| Prometheus     | `./data/prometheus`    | 200h      |
| Grafana        | `./data/grafana`       | —         |
| Loki           | `./data/loki`          | 744h      |
| OTel Collector | `./data/otel-collector`| —         |

### Volúmenes gestionados por Docker

Si prefieres no usar bind-mounts locales y que Docker gestione los volúmenes (sin depender de carpetas en el host):

1. No necesitas ejecutar `mkdir -p`.
2. En `compose.yaml`, cambia los volúmenes de:

```yaml
volumes:
  prometheus_data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: ./data/prometheus
  grafana_data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: ./data/grafana
  loki_data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: ./data/loki
```

a:

```yaml
volumes:
  prometheus_data:
    driver: local
  grafana_data:
    driver: local
  loki_data:
    driver: local
```

Los datos se almacenarán en el directorio de volúmenes de Docker (`/var/lib/docker/volumes/`) sin intervención manual.

## Detener y limpiar

```bash
docker compose down
# Opcional: borrar datos persistentes
rm -rf data/
```

## Personalización

- **Scrape targets**: editar `prometheus/prometheus.yaml`
- **Datasources de Grafana**: editar `grafana/provisioning/datasources/datasources.yaml`
- **Retención y esquema de Loki**: editar `loki/loki-config.yaml`
- **Config de Promtail**: editar `promtail/promtail-config.yaml`
