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

autoclient: proto
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/autoclient cmd/autoclient/main.go

# Clean up build artifacts
clean:
	rm -rf $(BIN_DIR) $(PROTO_DIR)/*.pb.go $(PROTO_DIR)/*_grpc.pb.go

# Example run commands (not part of the build process, but helpful)
# Usage: make run-worker PORT=8081
PORT ?= 8080
REDIS_PORT ?= 63079
DISCOVERY_REDIS_PORT ?= 63179
run-worker: worker
	$(BIN_DIR)/worker -port $(PORT) -redisport $(REDIS_PORT) -discovery $(DISCOVERY_REDIS_PORT)

run-client: client
	$(BIN_DIR)/client -server localhost:8080

run-autoclient: autoclient
	$(BIN_DIR)/autoclient -server localhost:8080

start-redis:
	redis-server --port $(REDIS_PORT)

start-discovery-redis:
	redis-server --port $(DISCOVERY_REDIS_PORT)

start-all-nodes:
	# start in different terminals
	make start-redis REDIS_PORT=63079
	make start-redis REDIS_PORT=63080
	make start-redis REDIS_PORT=63081
	make start-discovery-redis DISCOVERY_REDIS_PORT=63179
	make run-worker PORT=8081 REDIS_PORT=63079 DISCOVERY_REDIS_PORT=63179
	make run-worker PORT=8082 REDIS_PORT=63080 DISCOVERY_REDIS_PORT=63179
	make run-worker PORT=8083 REDIS_PORT=63081 DISCOVERY_REDIS_PORT=63179
	make run-client

# Launch all nodes in separate terminal windows automatically
launch-all-nodes:
	xterm -e make start-redis REDIS_PORT=63079 &
	xterm -e make start-redis REDIS_PORT=63080 &
	xterm -e make start-redis REDIS_PORT=63081 &
	xterm -e make start-discovery-redis DISCOVERY_REDIS_PORT=63179 &
	xterm -e make run-worker PORT=8080 REDIS_PORT=63079 DISCOVERY_REDIS_PORT=63179 &
	xterm -e make run-worker PORT=8081 REDIS_PORT=63080 DISCOVERY_REDIS_PORT=63179 &
	xterm -e make run-worker PORT=8082 REDIS_PORT=63081 DISCOVERY_REDIS_PORT=63179 &
# xterm -e make run-client &

# Clear Redis instances
clear-redis:
	redis-cli -p 63079 flushall
	redis-cli -p 63080 flushall
	redis-cli -p 63081 flushall
	redis-cli -p 63179 flushall
	@echo "All Redis instances cleared"

# Make a command to kill all xterm
kill-xterm:

	pkill -f "make start-redis" || true
	pkill -f "make" || true
	pkill -f "make start-discovery-redis" || true
	pkill -f "make run-worker" || true
	pkill -f "make run-client" || true
	@echo "All processes terminated"

	
