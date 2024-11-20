package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"

	"github.com/SigNoz/go-grpc-stream-otel/chat"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serviceName  = os.Getenv("SERVICE_NAME")
	collectorURL = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
)

func initTracer() (*trace.TracerProvider, error) {

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracegrpc.NewClient(
			otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
			otlptracegrpc.WithEndpoint(collectorURL),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewSchemaless(
			semconv.ServiceNameKey.String("ChatClient"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

func main() {
	tp, err := initTracer()
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := chat.NewChatServiceClient(conn)
	// Start a span for the initial stream creation
	ctx, span := otel.Tracer("ChatClient").Start(context.Background(), "Chat")

	stream, err := client.Chat(ctx)
	if err != nil {
		span.RecordError(err)
		log.Fatalf("Error creating stream: %v", err)
	}
	span.End()

	// Start goroutines for sending and receiving messages
	waitc := make(chan struct{})
	go receiveMessages(stream, waitc)
	sendMessages(stream)

	// Close the send direction of the stream
	if err := stream.CloseSend(); err != nil {
		log.Printf("Error closing send stream: %v", err)
	}

	// Wait for the receiving goroutine to finish
	<-waitc

	// Optional: Close the entire stream if necessary (usually not needed)
	// conn.Close() is already deferred

}

func sendMessages(stream chat.ChatService_ChatClient) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		log.Print("Enter message (or 'exit' to quit): ")
		if !scanner.Scan() {
			// EOF or error
			break
		}
		text := scanner.Text()
		if text == "exit" {
			break
		}

		// Start a new root span for each message
		_, msgSpan := otel.Tracer("ChatClient").Start(context.Background(), "SendMessage")
		msgSpan.SetAttributes(semconv.EnduserID("ClientUser"))

		if err := stream.Send(&chat.ChatMessage{
			User:    "ClientUser",
			Message: text,
		}); err != nil {
			msgSpan.RecordError(err)
			log.Printf("Error sending message: %v", err)
			msgSpan.End()
			break
		}
		msgSpan.End()
	}
}

func receiveMessages(stream chat.ChatService_ChatClient, waitc chan struct{}) {
	defer close(waitc)
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Println("Server closed the stream")
			break
		}
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			break
		}

		// Start a new root span for each received message
		_, msgSpan := otel.Tracer("ChatClient").Start(context.Background(), "ReceiveMessage")
		msgSpan.SetAttributes(semconv.EnduserID(msg.User))

		log.Printf("Received message from %s: %s", msg.User, msg.Message)

		msgSpan.End()
	}
}
