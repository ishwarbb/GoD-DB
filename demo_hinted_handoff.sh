#!/bin/bash
set -e

echo "=== Hinted Handoff Demonstration ==="

# Ensure we have netcat for the visualization tool
command -v nc >/dev/null 2>&1 || { 
  echo "Error: netcat (nc) is required but not installed. Please install it."
  exit 1
}

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

# Build the visualization tool
echo "Building visualization tool..."
go build -o bin/visualize_hints visualize_hints.go

# Start workers in separate terminals
echo "Starting workers..."

# Start worker 1
gnome-terminal -- bash -c "
echo 'Starting Worker 1 (port 8081)...'
./bin/worker -port 8081 -redisport 63079 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9081
" &

# Start worker 2
gnome-terminal -- bash -c "
echo 'Starting Worker 2 (port 8082)...'
./bin/worker -port 8082 -redisport 63080 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9082
" &

# Start worker 3
gnome-terminal -- bash -c "
echo 'Starting Worker 3 (port 8083)...'
./bin/worker -port 8083 -redisport 63081 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9083
" &

# Give the workers time to start and discover each other
echo "Waiting for workers to start and stabilize..."
sleep 10

# Start visualization in a new terminal
gnome-terminal -- bash -c "
echo 'Starting Hinted Handoff Visualization...'
./bin/visualize_hints
" &

# Interactive demo script
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
echo "The visualization dashboard shows the process in real-time."
echo

read -p "Press Enter to start writing initial data..." x

# Write initial data
echo "Writing initial data to cluster through worker 1..."
for i in {1..5}; do
  KEY="initial-key-$i"
  VALUE="initial-value-$i"
  echo "Writing $KEY = $VALUE"
  curl -s "http://localhost:9081/put?key=$KEY&value=$VALUE" > /dev/null
  sleep 1
done

echo
read -p "Press Enter to kill worker 3 (to simulate node failure)..." x

# Kill worker 3
echo "Killing worker 3..."
pkill -f "bin/worker -port 8083" || true

echo
read -p "Press Enter to write data with worker 3 down (should generate hints)..." x

# Write more data (should generate hints for worker 3)
echo "Writing data with worker 3 down (should generate hints)..."
for i in {1..10}; do
  KEY="hinted-key-$i"
  VALUE="hinted-value-$i"
  echo "Writing $KEY = $VALUE"
  curl -s "http://localhost:9081/put?key=$KEY&value=$VALUE" > /dev/null
  sleep 1
done

echo
echo "Check the visualization dashboard to see the hints being stored."
echo
read -p "Press Enter to restart worker 3..." x

# Restart worker 3
echo "Restarting worker 3..."
gnome-terminal -- bash -c "
echo 'Starting Worker 3 (port 8083)...'
./bin/worker -port 8083 -redisport 63081 -discovery localhost:63179 -replication 3 -writequorum 2 -readquorum 1 -virtualnodes 50 -hintinterval 5s -enablehints true -httpport 9083
" &

echo
echo "Worker 3 is restarting. Watch the visualization dashboard to see hints being delivered."
echo "The hints should be automatically delivered to worker 3 as it rejoins the cluster."

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
echo "Demo complete! You can continue to observe the visualization or press Ctrl+C to exit."
echo "To clean up, run: pkill -f 'bin/worker' && pkill -f 'bin/visualize_hints'"
echo

# Keep the script running until user presses Ctrl+C
trap "echo 'Exiting demo...'" INT
echo "Press Ctrl+C to exit"
while true; do
  sleep 1
done 