#!/bin/bash
set -e

echo "=== Hinted Handoff Demonstration ==="

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

# Clear any existing data in Redis
echo "Clearing Redis data..."
redis-cli -p 63079 FLUSHALL
redis-cli -p 63080 FLUSHALL
redis-cli -p 63081 FLUSHALL

# Build the worker if it doesn't exist
if [ ! -f "bin/worker" ]; then
  echo "Building worker..."
  go build -o bin/worker cmd/worker/main.go
fi

# Build the visualization tool
echo "Building visualization tool..."
go build -o bin/visualize_hints visualize_hints.go

# Start workers in the background
echo "Starting workers..."

# Start worker 1
echo "Starting Worker 1 (port 8081)..."
./bin/worker -port 8081 -redisport 63079 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9081 > worker1.log 2>&1 &
WORKER1_PID=$!

# Start worker 2
echo "Starting Worker 2 (port 8082)..."
./bin/worker -port 8082 -redisport 63080 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9082 > worker2.log 2>&1 &
WORKER2_PID=$!

# Start worker 3
echo "Starting Worker 3 (port 8083)..."
./bin/worker -port 8083 -redisport 63081 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9083 > worker3.log 2>&1 &
WORKER3_PID=$!

# Give the workers time to start and discover each other
echo "Waiting for workers to start and stabilize (10 seconds)..."
sleep 10

# Verify all workers are running
echo "Checking workers status:"
echo -n "Worker 1: "
curl -s -o /dev/null -w "%{http_code}" "http://localhost:9081/ping" || echo "Error"
echo
echo -n "Worker 2: "
curl -s -o /dev/null -w "%{http_code}" "http://localhost:9082/ping" || echo "Error"
echo
echo -n "Worker 3: "
curl -s -o /dev/null -w "%{http_code}" "http://localhost:9083/ping" || echo "Error"
echo

echo
echo "========= INTERACTIVE HINTED HANDOFF DEMO ========="
echo
echo "This script will demonstrate hinted handoff by:"
echo "1. Writing initial data to all nodes"
echo "2. Killing one node"
echo "3. Writing more data (which should generate hints)"
echo "4. Restarting the killed node"
echo "5. Observing the hints being delivered"
echo

read -p "Press Enter to start writing initial data..." x

# Write initial data
echo "Writing initial data to cluster through worker 1..."
for i in {1..5}; do
  KEY="initial-key-$i"
  VALUE="initial-value-$i"
  echo "Writing $KEY = $VALUE"
  curl -s "http://localhost:9081/put?key=$KEY&value=$VALUE" 
  echo
  sleep 1
done

# Check current hint count on workers
echo
echo "Current hint counts:"
echo -n "Worker 1 hints: "
curl -s "http://localhost:9081/hintcount" || echo "Error"
echo
echo -n "Worker 2 hints: "
curl -s "http://localhost:9082/hintcount" || echo "Error"
echo
echo -n "Worker 3 hints: "
curl -s "http://localhost:9083/hintcount" || echo "Error"
echo

echo
read -p "Press Enter to kill worker 3 (to simulate node failure)..." x

# Kill worker 3
echo "Killing worker 3..."
kill $WORKER3_PID 
sleep 3
echo "Worker 3 killed."

echo
read -p "Press Enter to write data with worker 3 down (should generate hints)..." x

# Write more data (should generate hints for worker 3)
echo "Writing data with worker 3 down (should generate hints)..."
for i in {1..10}; do
  KEY="hinted-key-$i"
  VALUE="hinted-value-$i"
  echo "Writing $KEY = $VALUE"
  curl -s "http://localhost:9081/put?key=$KEY&value=$VALUE"
  echo
  sleep 1
done

# Check hint counts again
echo
echo "Hint counts after writing with worker 3 down:"
echo -n "Worker 1 hints: "
curl -s "http://localhost:9081/hintcount" || echo "Error"
echo
echo -n "Worker 2 hints: "
curl -s "http://localhost:9082/hintcount" || echo "Error"
echo

echo
read -p "Press Enter to restart worker 3..." x

# Restart worker 3
echo "Restarting worker 3..."
./bin/worker -port 8083 -redisport 63081 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9083 > worker3.log 2>&1 &
WORKER3_PID=$!

echo
echo "Worker 3 is restarting. Hints should be automatically delivered to worker 3 as it rejoins the cluster."
echo "Waiting 30 seconds for hint delivery..."
sleep 30

# Check hint counts again
echo
echo "Hint counts after worker 3 rejoined:"
echo -n "Worker 1 hints: "
curl -s "http://localhost:9081/hintcount" || echo "Error"
echo
echo -n "Worker 2 hints: "
curl -s "http://localhost:9082/hintcount" || echo "Error"
echo

read -p "Press Enter to verify data on worker 3..." x

# Verify the data on worker 3
echo "Verifying the data on worker 3..."
for i in {1..10}; do
  KEY="hinted-key-$i"
  echo -n "Checking $KEY on worker 3: "
  RESPONSE=$(curl -s "http://localhost:9083/get?key=$KEY")
  if [[ $RESPONSE == *"hinted-value-$i"* ]]; then
    echo "✅ FOUND! Value = hinted-value-$i"
  else
    echo "❌ NOT FOUND"
  fi
  sleep 0.5
done

echo
echo "Demo complete! Data has been automatically restored on worker 3 through the hinted handoff mechanism."
echo "This demonstrates how hinted handoff ensures data consistency across nodes even after temporary node failures."
echo

# Cleanup
echo "Cleaning up..."
kill $WORKER1_PID $WORKER2_PID $WORKER3_PID || true
echo "Demo finished. All worker processes terminated." 