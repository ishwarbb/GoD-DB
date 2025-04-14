.PHONY: all clean proto worker client generate-proto

# Project details
PROJECT_NAME := GoD-DB
PROTO_DIR := pkg/rpc
PROTO_FILE := $(PROTO_DIR)/dynamo.proto
GO_PKG := no.cap/goddb # Updated to match the actual Go module path
BIN_DIR := bin

all: worker client

# Generate Protocol Buffers and gRPC code
generate-proto:
	mkdir -p $(PROTO_DIR)
	@echo "Generating protobuf code..."
	protoc --proto_path=$(PROTO_DIR) \
		--go_out=. --go_opt=module=$(GO_PKG) \
		--go-grpc_out=. --go-grpc_opt=module=$(GO_PKG) \
		$(PROTO_FILE)
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
# Usage: make run-worker PORT=8081
PORT ?= 8080
run-worker: worker
	$(BIN_DIR)/worker -port $(PORT) -redisport 63079

run-client: client
	$(BIN_DIR)/client -server localhost:8080

start-redis:
	redis-server --port 63079