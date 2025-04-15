#!/bin/bash

# Array to store all PIDs
declare -a PIDS

# Create a log directory
mkdir -p logs

# Trap SIGINT (Ctrl+C) to kill all processes and exit
trap 'echo "Terminating all processes..."; for pid in "${PIDS[@]}"; do kill -9 $pid 2>/dev/null; done; exit' SIGINT

# Function to run a command and save its PID
run_command() {
    local cmd="$1"
    local log_file="$2"
    
    echo "Starting: $cmd (logs: $log_file)"
    $cmd > "logs/$log_file" 2>&1 &
    local pid=$!
    PIDS+=($pid)
    echo "  → PID: $pid"
}

# Start Redis instances
run_command "make start-redis REDIS_PORT=63079" "redis-63079.log"
run_command "make start-redis REDIS_PORT=63080" "redis-63080.log"
run_command "make start-redis REDIS_PORT=63081" "redis-63081.log"

# Start discovery Redis
run_command "make start-discovery-redis DISCOVERY_REDIS_PORT=63179" "discovery-redis-63179.log"

# Give Redis instances a moment to start
sleep 2

# Start workers
run_command "make run-worker PORT=8080 REDIS_PORT=63079 DISCOVERY_REDIS_PORT=63179" "worker-8080.log"
sleep 3
run_command "make run-worker PORT=8081 REDIS_PORT=63080 DISCOVERY_REDIS_PORT=63179" "worker-8081.log"
sleep 3
run_command "make run-worker PORT=8082 REDIS_PORT=63081 DISCOVERY_REDIS_PORT=63179" "worker-8082.log"

echo ""
echo "All processes started. Press Ctrl+C to terminate."
echo "Logs are saved in the logs directory."
echo ""
echo "PIDs: ${PIDS[@]}"

# Wait for all processes to finish (or until interrupted)
wait 