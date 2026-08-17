# Stage 1: compilar los 3 binarios
FROM golang:1.22 AS builder

RUN go install github.com/swaggo/swag/cmd/swag@latest

WORKDIR /src

# Copiar dependencias primero (mejora cache de Docker)
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fuente
COPY . .

# Generar docs de Swagger desde las anotaciones del gateway
RUN swag init -g cmd/gateway/main.go -o docs

# Compilar los 3 binarios
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/gateway  ./cmd/gateway/
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/processor ./cmd/processor/
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/storage   ./cmd/storage/

# Stage 2: imagen final minimalista
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/bin/ /app/bin/

# El entrypoint se sobreescribe en docker-compose
ENTRYPOINT ["/app/bin/gateway"]
