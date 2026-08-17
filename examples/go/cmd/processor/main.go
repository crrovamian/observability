package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.28.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var tracer = otel.Tracer("processor")

func initTracer() *sdktrace.TracerProvider {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	exporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("processor"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp
}

type ItemResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ProcessItem godoc
// @Summary      Procesa un item
// @Description  Recibe un item, lo procesa y lo envía al storage
// @Tags         processor
// @Accept       json
// @Produce      json
// @Param        id    query  string  true  "ID del item"
// @Param        index query  int     true  "Índice del item"
// @Success      200   {object}  ItemResult
// @Router       /process-item [post]
func processItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "processor.process_item",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		),
	)
	defer span.End()

	itemID := r.URL.Query().Get("id")
	indexStr := r.URL.Query().Get("index")
	var index int
	fmt.Sscanf(indexStr, "%d", &index)

	span.SetAttributes(
		attribute.String("item.id", itemID),
		attribute.Int("item.index", index),
	)

	// Simular procesamiento (transformación de datos)
	time.Sleep(10 * time.Millisecond)

	span.AddEvent("item_processed", trace.WithAttributes(
		attribute.String("item.id", itemID),
	))

	// Enviar al storage
	storageAddr := os.Getenv("STORAGE_ADDR")
	if storageAddr == "" {
		storageAddr = "localhost:50052"
	}

	result := sendToStorage(ctx, storageAddr, itemID)

	span.SetAttributes(attribute.String("item.status", result.Status))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func sendToStorage(ctx context.Context, addr, itemID string) ItemResult {
	_, span := tracer.Start(ctx, "processor.send_to_storage",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("item.id", itemID),
			attribute.String("rpc.system", "http"),
		),
	)
	defer span.End()

	url := fmt.Sprintf("http://%s/store-item?id=%s", addr, itemID)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		span.SetAttributes(attribute.String("error", err.Error()))
		return ItemResult{ID: itemID, Status: "error: " + err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result ItemResult
	if err := json.Unmarshal(body, &result); err != nil {
		return ItemResult{ID: itemID, Status: "error: unmarshal"}
	}

	span.SetAttributes(attribute.String("storage.status", result.Status))
	return result
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	tp := initTracer()
	defer tp.Shutdown(context.Background())

	http.HandleFunc("/process-item", processItemHandler)
	http.HandleFunc("/health", healthHandler)

	log.Println("processor listening on :50051")
	log.Fatal(http.ListenAndServe(":50051", nil))
}
