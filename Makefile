.PHONY: all clean proto worker client generate-proto

# Project details
PROJECT_NAME := GoD-DB
PROTO_DIR := pkg/rpc
PROTO_FILE := $(PROTO_DIR)/dynamo.proto
GO_PKG := github.com/yourusername/$(PROJECT_NAME) # Replace with your actual Go module path
BIN_DIR := bin

all: worker client

# Generate Protocol Buffers and gRPC code
generate-proto:
	mkdir -p $(PROTO_DIR)
	@echo "Generating protobuf code..."
	protoc --proto_path=$(PROTO_DIR) --go_out=$(PROTO_DIR) --go_opt=paths=source_relative --go-grpc_out=$(PROTO_DIR) --go-grpc_opt=paths=source_relative $(PROTO_FILE)
	@echo "Protobuf code generated successfully."

proto: generate-proto

# Build worker
worker: proto
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/worker cmd/worker/main.go

# Build client
client: proto
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/client cmd/client/main.go

# Clean up build artifacts
clean:
	rm -rf $(BIN_DIR) $(PROTO_DIR)/*.pb.go $(PROTO_DIR)/*_grpc.pb.go

# Example run commands (not part of the build process, but helpful)
run-worker: worker
	$(BIN_DIR)/worker -port 8080 -redisport 6379

run-client: client
	$(BIN_DIR)/client -server localhost:8080
