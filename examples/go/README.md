# Go Microservices — 3 servicios con streaming + OpenTelemetry

Tres microservicios Go que se comunican por HTTP simulando streaming de datos. Cada cruce propaga el contexto de OTel automáticamente.

## Arquitectura

```
Client HTTP
    │
    ▼
gateway(:8080)  ──── HTTP ────▶  processor(:50051)  ──── HTTP ────▶  storage(:50052)
  span: gateway.process           span: processor.process_item        span: storage.store_item
  kind: SERVER                    kind: SERVER                        kind: SERVER
    │                                │                                   │
    │  traceparent via              │  traceparent via                  │
    │  HTTP header                  │  HTTP header                      │
    │                                │                                   │
    └─── response ←──────────────────┴───────────────────────────────────┘
```

Cada item genera un span hijo dentro del span anterior. En Grafana/Tempo verás:

```
gateway.process                          ← span raíz
├── gateway.send_item [item-0]
│   └── processor.process_item           ← hijo, traceparent propagado
│       └── processor.send_to_storage    ← hijo
│           └── storage.store_item       ← nieto
├── gateway.send_item [item-1]
│   └── processor.process_item
│       └── processor.send_to_storage
│           └── storage.store_item
└── ...
```

## Qué hace cada servicio

### gateway (`:8080`)

- **HTTP server** que recibe `GET /process?items=N`
- Crea el **span raíz** (`gateway.process`)
- Por cada item, crea un span hijo (`gateway.send_item`) y llama al processor
- Retorna JSON con los resultados de todos los items

### processor (`:50051`)

- **HTTP server** que recibe `POST /process-item?id=X&index=N`
- El `traceparent` llega en el header HTTP → OTel lo extrae y crea el span hijo automáticamente
- Simula procesamiento (10ms de sleep)
- Envía al storage, propagando el contexto

### storage (`:50052`)

- **HTTP server** que recibe `POST /store-item?id=X`
- Recibe el `traceparent`, crea el span más profundo del árbol
- Simula persistencia (log + span event)

## Librerías OTel que usa

| Librería | Versión | Qué hace |
|----------|---------|----------|
| `go.opentelemetry.io/otel` | v1.45.0 | API core: `Tracer`, `SetTracerProvider`, `SetTextMapPropagator` |
| `go.opentelemetry.io/otel/sdk` | v1.45.0 | `TracerProvider`, `Resource`, `WithBatcher`, `Shutdown` |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | v1.45.0 | Exporta spans al collector vía OTLP gRPC |
| `go.opentelemetry.io/otel/semconv/v1.28.0` | (del módulo otel) | Atributos semánticos: `ServiceNameKey` |
| `go.opentelemetry.io/otel/propagation` | (del módulo otel) | `TraceContext{}` + `Baggage{}` para inyectar/extraer `traceparent` |
| `go.opentelemetry.io/otel/trace` | (del módulo otel) | `SpanKindServer`, `SpanKindClient`, `WithAttributes`, `WithSpanKind` |

### Qué NO usa (pero podrías agregar)

| Librería | Para qué |
|----------|----------|
| `go.opentelemetry.io/contrib/.../otelhttp` | Instrumentación automática de HTTP (crea spans CLIENT/SERVER automáticamente por cada request) |
| `go.opentelemetry.io/contrib/.../otelgrpc` | Instrumentación automática de gRPC |

En este ejemplo, los spans se crean **manualmente** con `tracer.Start()` para que se vea exactamente qué hace cada parte. Con `otelhttp` los spans se crearían solos.

## Variables de entorno

| Variable | Default | Descripción |
|----------|---------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4317` | Endpoint del otel-collector |
| `PROCESSOR_ADDR` | `localhost:50051` | Dirección del processor (gateway la usa) |
| `STORAGE_ADDR` | `localhost:50052` | Dirección del storage (processor la usa) |

## Cómo correr

### Opción 1: Docker Compose (con el stack de observabilidad)

Requiere que el stack principal esté corriendo (`docker compose up` en `prometheus-loki-grafana/`):

```bash
cd examples/go
docker compose up --build
curl http://localhost:8080/process?items=5
docker compose down
```

### Swagger UI

Una vez que el gateway esté corriendo, abre:

```
http://localhost:8080/swagger/
```

El Swagger UI muestra los endpoints documentados con las anotaciones `// @Summary` etc. Desde ahí puedes probar `GET /process?items=5` directamente desde el browser.

Los docs se generan con `swag init -g cmd/gateway/main.go -o docs` (ya incluido en el Dockerfile). Si modificas las anotaciones, vuelve a correr ese comando.

### Opción 2: Local (3 terminales)

```bash
# Terminal 1 — storage
cd examples/go
go run ./cmd/storage/

# Terminal 2 — processor
cd examples/go
STORAGE_ADDR=localhost:50052 go run ./cmd/processor/

# Terminal 3 — gateway
cd examples/go
PROCESSOR_ADDR=localhost:50051 go run ./cmd/gateway/
```

Probar:

```bash
curl http://localhost:8080/process?items=3
```

Respuesta:

```json
{
  "status": "completed",
  "items": 3,
  "results": [
    {"id": "item-0", "status": "stored"},
    {"id": "item-1", "status": "stored"},
    {"id": "item-2", "status": "stored"}
  ]
}
```

### Parar

- Local: `Ctrl+C` en cada terminal
- Docker: `docker compose down`

## Qué verás en Grafana

1. Abrir Grafana → **Tempo** → **Search by service**
2. Buscar por `service.name`:
   - `gateway` — el span raíz y los `send_item`
   - `processor` — los spans `process_item` y `send_to_storage`
   - `storage` — los spans `store_item`
3. Click en cualquier span de `gateway.process` → verás el árbol completo con traceparent propagado

## Estructura

```
examples/go/
├── go.mod
├── go.sum
├── Dockerfile                    # multi-stage build (swag init incluido)
├── docker-compose.yaml           # levanta los 3 servicios
├── cmd/
│   ├── gateway/main.go           # HTTP :8080, span raíz, Swagger UI
│   ├── processor/main.go         # HTTP :50051, procesa items
│   └── storage/main.go           # HTTP :50052, almacena items
├── docs/
│   ├── docs.go                   # generado por swag
│   ├── swagger.json              # generado por swag
│   └── swagger.yaml              # generado por swag
├── main.go                       # ejemplo original (single service)
└── README.md
```

## Compilar binarios

```bash
go build -o bin/gateway  ./cmd/gateway/
go build -o bin/processor ./cmd/processor/
go build -o bin/storage   ./cmd/storage/
```

## Cómo funciona el build (Docker multi-stage)

### El Dockerfile compila los 3 binarios, no uno

```dockerfile
RUN go build -o /app/bin/gateway  ./cmd/gateway/
RUN go build -o /app/bin/processor ./cmd/processor/
RUN go build -o /app/bin/storage   ./cmd/storage/
```

Son 3 líneas `RUN` separadas. La imagen builder genera los 3 binarios en `/app/bin/`.

### Multi-stage: por qué la imagen final es chica

```
Stage 1 (builder, ~800MB):    Go SDK + swag + compila los 3 binarios
Stage 2 (final, ~15MB):       Solo alpine + los 3 binarios
```

La etapa `builder` se descarta completa. La imagen final solo lleva los binarios compilados + alpine minimalista. Los 3 binarios están ahí pero solo se ejecuta 1.

### Cómo sabe cuál ejecutar: `command` sobreescribe `ENTRYPOINT`

El Dockerfile define un ENTRYPOINT por defecto:

```dockerfile
ENTRYPOINT ["/app/bin/gateway"]
```

Pero `docker-compose.yaml` lo sobreescribe con `command` por servicio:

```yaml
gateway:
  command: /app/bin/gateway    # ← sobreescribe ENTRYPOINT

processor:
  command: /app/bin/processor  # ← sobreescribe ENTRYPOINT

storage:
  command: /app/bin/storage    # ← sobreescribe ENTRYPOINT
```

Resultado: **1 imagen, 3 containers distintos**. Cada container ejecuta un binario diferente del mismo artefacto.

### `go.mod` y `go.sum` ≈ `package.json` y `package-lock.json`

| Node.js | Go | Qué hace |
|---------|-----|----------|
| `package.json` | `go.mod` | Lista las dependencias y versiones que **querés** |
| `package-lock.json` | `go.sum` | Lockea versiones exactas + hashes SHA para integridad |

La diferencia: en Go no hay `node_modules/` — las dependencias se descargan a un cache global (`~/go/pkg/mod/`). El comando `go mod tidy` es equivalente a `npm install`.
