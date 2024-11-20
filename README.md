# go-grpc-stream-otel

Start go grpc server and grpc client using below commands

1. Grpc-Server
```
cd server
SERVICE_NAME=goAppServer OTEL_EXPORTER_OTLP_HEADERS=signoz-access-token=YOUR_SIGNOZ_INGESTION_TOKEN OTEL_EXPORTER_OTLP_ENDPOINT=YOUR_COLLECTOR_ENDPOINT go run main.go
```

2. Grpc-Client
```
cd client
SERVICE_NAME=goAppClient OTEL_EXPORTER_OTLP_HEADERS=signoz-access-token=YOUR_SIGNOZ_INGESTION_TOKEN OTEL_EXPORTER_OTLP_ENDPOINT=YOUR_COLLECTOR_ENDPOINT  go run main.go
```

SigNoz Ingestion Token in only required for cloud users. Self hosted users only need to specify OTEL_EXPORTER_OTLP_ENDPOINT and SERVICE_NAME.
