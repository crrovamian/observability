# Observability Stack en Kubernetes (kube-prometheus-stack)

Versión Kubernetes de la misma pila. Todo se instala con **Helm** en el namespace `monitoring`. La mitad de los componentes vienen incluidos en `kube-prometheus-stack`; Loki, Tempo y el OTel Collector se instalan aparte.

## Resumen de componentes

| Componente | Incluido en kube-prometheus-stack | Chart |
|------------|:---:|---|
| Prometheus | ✅ | `prometheus-community/kube-prometheus-stack` |
| Grafana | ✅ | ídem |
| node-exporter | ✅ (subchart) | ídem |
| kube-state-metrics | ✅ (subchart) | ídem |
| Prometheus Operator | ✅ (CRDs: ServiceMonitor, PodMonitor, AlertmanagerConfig) | ídem |
| Alertmanager | ✅ (subchart) | ídem |
| cAdvisor | ❌ no hace falta — el **kubelet** ya expone `/metrics/cadvisor` | — |
| OTel Collector | ❌ | `open-telemetry/opentelemetry-collector` |
| Loki | ❌ | `grafana/loki` (ingesta OTLP nativa) |
| Tempo | ❌ | `grafana/tempo` |

## Requisitos

- `kubectl`, Helm v3
- Un cluster (kind, minikube, k3s, EKS…)

## 1. kube-prometheus-stack

Incluye Prometheus, Grafana, Alertmanager, Prometheus Operator, node-exporter y kube-state-metrics.

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/kube-prometheus-stack \
  -f kubernetes/kube-prometheus-stack/values.yaml \
  --namespace monitoring --create-namespace
```

Los overrides registran en Grafana los datasources de Loki y Tempo (Prometheus ya viene configurado):

```yaml
# kubernetes/kube-prometheus-stack/values.yaml
grafana:
  adminPassword: admin
  additionalDataSources:
    - name: Loki
      type: loki
      access: proxy
      url: http://loki.monitoring.svc:3100
      editable: true
    - name: Tempo
      type: tempo
      access: proxy
      url: http://tempo.monitoring.svc:3200
      editable: true
```

### Añadir métricas de tu app

1. Tu servicio debe exponer `/metrics` y tener la label que seleccione el ServiceMonitor.
2. El Prometheus Operator scrapea a través de un `ServiceMonitor`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: mi-app
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: mi-app
  endpoints:
    - port: http
      path: /metrics
```

## 2. Loki

Loki v3+ ingesta **OTLP nativo** en `/otlp/v1/logs`, así que el collector no necesita el exporter `loki` (que está deprecado/removido).

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
helm install loki grafana/loki \
  -f kubernetes/loki/values.yaml \
  --namespace monitoring
```

```yaml
# kubernetes/loki/values.yaml
# Los defaults del chart (schema tsdb, storage filesystem) se mantienen:
# Helm hace merge con la config del chart, solo overrideamos lo mínimo.
loki:
  config:
    auth_enabled: false
    limits_config:
      allow_structured_metadata: true
      retention_period: 744h
```

## 3. Tempo

```bash
helm install tempo grafana/tempo \
  -f kubernetes/tempo/values.yaml \
  --namespace monitoring
```

```yaml
# kubernetes/tempo/values.yaml
# Asegura los receivers OTLP; el resto (storage local, retención) usa los defaults del chart.
tempo:
  config:
    distributor:
      receivers:
        otlp:
          protocols:
            grpc:
              endpoint: 0.0.0.0:4317
            http:
              endpoint: 0.0.0.0:4318
```

## 4. OpenTelemetry Collector

Corre como **DaemonSet** (un pod por nodo) para leer los logs de todos los pods con el receiver `filelog`, enriquecerlos con metadata de K8s (`k8sattributes`) y enviarlos a Loki; las trazas que llegan por OTLP se reenvían a Tempo.

```bash
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-collector-helm
helm repo update
helm install otel-collector open-telemetry/opentelemetry-collector \
  -f kubernetes/otel-collector/values.yaml \
  --namespace monitoring
```

```yaml
# kubernetes/otel-collector/values.yaml
mode: daemonset

image:
  repository: otel/opentelemetry-collector-contrib

extraVolumes:
  - name: varlog-pods
    hostPath:
      path: /var/log/pods
  - name: otelcol-storage
    emptyDir: {}

extraVolumeMounts:
  - name: varlog-pods
    mountPath: /var/log/pods
    readOnly: true
  - name: otelcol-storage
    mountPath: /var/lib/otelcol

config:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:4317
        http:
          endpoint: 0.0.0.0:4318
    filelog:
      storage: file_storage
      include:
        - /var/log/pods/**/*.log
      start_at: beginning
      include_file_path: true
      operators:
        - type: json_parser
          timestamp:
            parse_from: attributes.time
            layout: '%Y-%m-%dT%H:%M:%S.%L%Z'
        - type: move
          from: attributes.log
          to: body
        - type: regex_parser
          regex: '^/var/log/pods/[^/]*_(?P<pod_uid>[0-9a-fA-F-]{24,36})/'
          parse_from: resource["log.file.path"]
        - type: move
          from: attributes.pod_uid
          to: resource["k8s.pod.uid"]

  processors:
    k8sattributes:
      extract:
        metadata:
          - k8s.pod.name
          - k8s.namespace.name
          - k8s.container.name
          - k8s.deployment.name
    batch:
      timeout: 1s
      send_batch_size: 1024

  extensions:
    file_storage:
      directory: /var/lib/otelcol
      create_directory: true

  exporters:
    otlp/tempo:
      endpoint: tempo.monitoring.svc:4317
      tls:
        insecure: true
    otlphttp/loki:
      endpoint: http://loki.monitoring.svc:3100/otlp/v1/logs
      tls:
        insecure: true
    debug:
      verbosity: basic

  service:
    extensions: [file_storage]
    pipelines:
      traces:
        receivers: [otlp]
        processors: [batch]
        exporters: [otlp/tempo, debug]
      logs:
        receivers: [otlp, filelog]
        processors: [k8sattributes, batch]
        exporters: [otlphttp/loki, debug]
```

## Envío de datos desde tus apps

Dentro del cluster, los endpoints son:

| Dato | Endpoint |
|------|----------|
| Trazas OTLP (gRPC) | `otel-collector.monitoring.svc:4317` |
| Trazas/logs OTLP (HTTP) | `http://otel-collector.monitoring.svc:4318` |
| Métricas (scrape) | `/metrics` + `ServiceMonitor` |

Los logs a `stdout` de cualquier pod se recolectan automáticamente por el DaemonSet (ya no hace falta label ni agente extra).

## Verificación

```bash
kubectl get pods -n monitoring
kubectl logs -n monitoring -l app.kubernetes.io/name=opentelemetry-collector --tail=50
```

Acceso a las UIs:

```bash
kubectl -n monitoring port-forward svc/prometheus-grafana 3001:80
kubectl -n monitoring port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090
kubectl -n monitoring port-forward svc/prometheus-kube-prometheus-alertmanager 9093:9093
kubectl -n monitoring port-forward svc/tempo 3200:3200
```

Credenciales de Grafana: `admin` / `admin` (definidas en el values del chart).

## Persistencia

Loki, Tempo y Prometheus usan PVCs con tu `storageClass` por defecto. En kind/minikube los PVs son efímeros (`emptyDir`); en clusters reales define `loki.persistence`, `tempo.persistence` y `prometheus.prometheusSpec.storageSpec` para usar un StorageClass con volúmenes persistentes.

## Detener

```bash
helm uninstall otel-collector loki tempo prometheus -n monitoring
```
