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

	_ "demo-go/docs"
	httpSwagger "github.com/swaggo/http-swagger"
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

var tracer = otel.Tracer("gateway")

// @title       Go Microservices API
// @version     1.0
// @description API con streaming entre 3 microservicios + OpenTelemetry
// @BasePath    /

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
			semconv.ServiceNameKey.String("gateway"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp
}

// ProcessRequest representa la petición de procesamiento
type ProcessRequest struct {
	Items int `json:"items" example:"5"`
}

// ProcessResponse representa la respuesta del procesamiento
type ProcessResponse struct {
	Status   string        `json:"status" example:"completed"`
	Items    int           `json:"items" example:"5"`
	Results  []ItemResult  `json:"results"`
}

// ItemResult representa el resultado de un item procesado
type ItemResult struct {
	ID     string `json:"id" example:"item-0"`
	Status string `json:"status" example:"stored"`
}

// ProcessItems godoc
// @Summary      Procesa items en streaming
// @Description  Recibe N items y los procesa secuencialmente a través de processor → storage, simulando streaming por partes
// @Tags         gateway
// @Accept       json
// @Produce      json
// @Param        items  query  int  true  "Número de items a procesar"
// @Success      200  {object}  ProcessResponse
// @Router       /process [get]
func processHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "gateway.process",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		),
	)
	defer span.End()

	itemsStr := r.URL.Query().Get("items")
	if itemsStr == "" {
		itemsStr = "5"
	}
	var itemCount int
	fmt.Sscanf(itemsStr, "%d", &itemCount)
	if itemCount <= 0 {
		itemCount = 5
	}

	processorAddr := os.Getenv("PROCESSOR_ADDR")
	if processorAddr == "" {
		processorAddr = "localhost:50051"
	}

	span.SetAttributes(attribute.Int("gateway.items.count", itemCount))

	results := make([]ItemResult, 0, itemCount)
	for i := 0; i < itemCount; i++ {
		itemID := fmt.Sprintf("item-%d", i)

		// Cada item es un span hijo dentro del span de process
		_, itemSpan := tracer.Start(ctx, "gateway.send_item",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("item.id", itemID),
				attribute.Int("item.index", i),
			),
		)

		result := sendToProcessor(ctx, processorAddr, itemID, i)
		results = append(results, result)

		itemSpan.SetAttributes(attribute.String("item.status", result.Status))
		itemSpan.End()
	}

	span.SetAttributes(attribute.Int("gateway.items.processed", len(results)))

	resp := ProcessResponse{
		Status:  "completed",
		Items:   itemCount,
		Results: results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sendToProcessor(ctx context.Context, addr, itemID string, index int) ItemResult {
	url := fmt.Sprintf("http://%s/process-item?id=%s&index=%d", addr, itemID, index)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ItemResult{ID: itemID, Status: "error: " + err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result ItemResult
	if err := json.Unmarshal(body, &result); err != nil {
		return ItemResult{ID: itemID, Status: "error: unmarshal"}
	}

	return result
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	tp := initTracer()
	defer tp.Shutdown(context.Background())

	http.HandleFunc("/process", processHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	log.Println("gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
