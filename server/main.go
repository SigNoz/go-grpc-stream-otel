package main

import (
	"context"
	"io"
	"log"
	"net"
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
)

type chatServer struct {
	chat.UnimplementedChatServiceServer
}

var (
	serviceName  = os.Getenv("SERVICE_NAME")
	collectorURL = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	insecure     = os.Getenv("INSECURE_MODE")
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
			semconv.ServiceNameKey.String("ChatService"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func (s *chatServer) Chat(stream chat.ChatService_ChatServer) error {
	// Start a span for the initial stream creation
	_, span := otel.Tracer("ChatService").Start(stream.Context(), "Chat")
	defer span.End()

	log.Println("Client connected for chat")
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Println("Client disconnected")
			return nil
		}
		if err != nil {
			span.RecordError(err)
			log.Printf("Error receiving message: %v", err)
			return err
		}

		// Start a new root span for each message received
		_, msgSpan := otel.Tracer("ChatService").Start(context.Background(), "ReceiveMessage")
		msgSpan.SetAttributes(semconv.EnduserID(msg.User))

		log.Printf("Received message from %s: %s", msg.User, msg.Message)

		// Echo the message back to the client
		if err := stream.Send(&chat.ChatMessage{
			User:    "Server",
			Message: "Echo: " + msg.Message,
		}); err != nil {
			msgSpan.RecordError(err)
			log.Printf("Error sending message: %v", err)
			msgSpan.End()
			return err
		}

		msgSpan.End()
	}
}

func main() {
	tp, err := initTracer()
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	chat.RegisterChatServiceServer(grpcServer, &chatServer{})

	log.Println("Starting gRPC server on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
