#!/bin/bash
set -e

echo "=== Hinted Handoff Test Runner ==="

# Stop existing workers if any
echo "Stopping any existing workers..."
pkill -f "bin/worker" || true
sleep 2

# Check if Redis is running and start it if not
echo "Ensuring Redis instances are running..."

# Discovery Redis
redis-cli -p 63179 ping >/dev/null 2>&1 || {
  echo "Starting discovery Redis on port 63179..."
  redis-server --port 63179 --daemonize yes
  sleep 1
}

# Worker Redis instances
for port in 63079 63080 63081; do
  redis-cli -p $port ping >/dev/null 2>&1 || {
    echo "Starting Redis on port $port..."
    redis-server --port $port --daemonize yes
    sleep 1
  }
done

# Start workers
echo "Starting workers..."
./bin/worker -port 8081 -redisport 63079 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 2 -virtualnodes 50 -hintinterval 5s -enablehints true &
WORKER1_PID=$!
sleep 2

./bin/worker -port 8082 -redisport 63080 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 2 -virtualnodes 50 -hintinterval 5s -enablehints true &
WORKER2_PID=$!
sleep 2

./bin/worker -port 8083 -redisport 63081 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 2 -virtualnodes 50 -hintinterval 5s -enablehints true &
WORKER3_PID=$!
sleep 5

# Cleanup function to kill workers on exit
cleanup() {
  echo "Cleaning up..."
  kill $WORKER1_PID $WORKER2_PID $WORKER3_PID 2>/dev/null || true
}

# Register the cleanup function to run on exit
trap cleanup EXIT

# Run the test
echo "Running hinted handoff test..."
./bin/test_handoff

echo "Test complete!" 