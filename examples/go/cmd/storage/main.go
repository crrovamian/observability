package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

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

var tracer = otel.Tracer("storage")

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
			semconv.ServiceNameKey.String("storage"),
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

// StoreItem godoc
// @Summary      Almacena un item
// @Description  Recibe un item y lo almacena (simula persistencia)
// @Tags         storage
// @Accept       json
// @Produce      json
// @Param        id  query  string  true  "ID del item"
// @Success      200  {object}  ItemResult
// @Router       /store-item [post]
func storeItemHandler(w http.ResponseWriter, r *http.Request) {
	_, span := tracer.Start(r.Context(), "storage.store_item",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		),
	)
	defer span.End()

	itemID := r.URL.Query().Get("id")
	span.SetAttributes(attribute.String("item.id", itemID))

	// Simular persistencia
	log.Printf("[storage] item %s stored", itemID)

	span.AddEvent("item_stored", trace.WithAttributes(
		attribute.String("item.id", itemID),
	))
	span.SetAttributes(attribute.String("db.system", "memory"))

	result := ItemResult{ID: itemID, Status: "stored"}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	tp := initTracer()
	defer tp.Shutdown(context.Background())

	http.HandleFunc("/store-item", storeItemHandler)
	http.HandleFunc("/health", healthHandler)

	log.Println("storage listening on :50052")
	log.Fatal(http.ListenAndServe(":50052", nil))
}
