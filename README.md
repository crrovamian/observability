# Observability Stack — Prometheus + Loki + Grafana

Stack local de observabilidad con Docker Compose. Proporciona métricas (Prometheus), logs (Loki) y dashboards (Grafana) para tus contenedores, más exporters de sistema.

## Servicios

| Servicio       | Imagen                            | Puerto  | Propósito                        |
|----------------|-----------------------------------|---------|----------------------------------|
| `nginx-demo`   | `nginx:alpine`                    | `80`    | App demo para generar tráfico    |
| `prometheus`   | `prom/prometheus:latest`          | `9090`  | Métricas y alertas               |
| `loki`         | `grafana/loki:latest`             | `3100`  | Agregación de logs               |
| `promtail`     | `grafana/promtail:latest`         | —       | Shipping de logs a Loki          |
| `grafana`      | `grafana/grafana:latest`          | `3001`  | Dashboards (métricas + logs)     |
| `cadvisor`     | `gcr.io/cadvisor/cadvisor:latest` | `8080`  | Métricas de contenedores         |
| `node-exporter`| `prom/node-exporter:latest`       | `9100`  | Métricas del host                |

## Requisitos

- Docker + Compose V2 (plugin `docker compose`)
- Linux (bind-mounts de `/proc`, `/sys`, `/var/run/docker.sock`, etc.)
- 2–4 GB de RAM libre

## Inicio rápido

```bash
cd prometheus-loki-grafana
docker compose up -d
```

## Acceso

| UI          | URL                           | Credenciales        |
|-------------|-------------------------------|---------------------|
| Grafana     | http://localhost:3001         | `admin` / `admin`   |
| Prometheus  | http://localhost:9090         | —                   |
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

Los datos se almacenan en `./data/` (bind-mounts locales).

| Componente  | Ruta              | Retención |
|-------------|-------------------|-----------|
| Prometheus  | `./data/prometheus`| 200h      |
| Grafana     | `./data/grafana`  | —         |
| Loki        | `./data/loki`     | 744h      |

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
