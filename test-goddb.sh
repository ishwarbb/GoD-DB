#!/bin/bash

# Set up terminal colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Starting GoD-DB Test Script${NC}"

# Check if Redis is installed
if ! command -v redis-server &> /dev/null; then
    echo -e "${RED}Redis is not installed. Please install Redis first.${NC}"
    exit 1
fi

# Check if the app was built
if [ ! -f "bin/worker" ] || [ ! -f "bin/client" ]; then
    echo -e "${YELLOW}Building GoD-DB applications...${NC}"
    make worker client
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build applications${NC}"
        exit 1
    fi
fi

# Start Redis instance
echo -e "${YELLOW}Starting Redis server on port 63079...${NC}"
redis-server --port 63079 --daemonize yes

# Define worker ports
PORT1=8081
PORT2=8082
PORT3=8083

# Create temp directory for logs
mkdir -p temp_logs

# Start the worker nodes in the background
echo -e "${YELLOW}Starting three worker nodes...${NC}"

# Worker 1 - Use a replication factor of 3, write quorum of 2, read quorum of 2
echo -e "${YELLOW}Starting Worker 1 on port $PORT1${NC}"
bin/worker -port $PORT1 -redisport 63079 -replication 3 -writequorum 2 -readquorum 2 > temp_logs/worker1.log 2>&1 &
WORKER1_PID=$!

# Wait a moment for the first worker to initialize
sleep 2

# Worker 2 - Connect to Worker 1
echo -e "${YELLOW}Starting Worker 2 on port $PORT2${NC}"
bin/worker -port $PORT2 -redisport 63079 -replication 3 -writequorum 2 -readquorum 2 > temp_logs/worker2.log 2>&1 &
WORKER2_PID=$!

# Worker 3 - Connect to Worker 1
echo -e "${YELLOW}Starting Worker 3 on port $PORT3${NC}"
bin/worker -port $PORT3 -redisport 63079 -replication 3 -writequorum 2 -readquorum 2 > temp_logs/worker3.log 2>&1 &
WORKER3_PID=$!

# Give the workers time to start and gossip with each other
echo -e "${YELLOW}Allowing workers to initialize and gossip (10 seconds)...${NC}"
sleep 10

# Run automated tests with the client
echo -e "${GREEN}Starting automated client tests...${NC}"

# Test 1: PUT a value and GET it
echo -e "${BLUE}Test 1: Basic PUT and GET${NC}"
# First connection to Worker 1
echo -e "${YELLOW}Connecting to Worker 1 to PUT a key-value pair${NC}"
CMD1="put testkey1 testvalue1"
echo -e "${BLUE}Command: $CMD1${NC}"
echo "$CMD1" | bin/client -server localhost:$PORT1

# Connect to Worker 2 to GET the value (tests replication)
echo -e "${YELLOW}Connecting to Worker 2 to GET the same key${NC}"
CMD2="get testkey1"
echo -e "${BLUE}Command: $CMD2${NC}"
echo "$CMD2" | bin/client -server localhost:$PORT2

# Test 2: PUT multiple values to different workers
echo -e "\n${BLUE}Test 2: PUT multiple values to different workers${NC}"
# Put to Worker 2
echo -e "${YELLOW}Connecting to Worker 2 to PUT a key-value pair${NC}"
CMD3="put testkey2 testvalue2"
echo -e "${BLUE}Command: $CMD3${NC}"
echo "$CMD3" | bin/client -server localhost:$PORT2

# Put to Worker 3
echo -e "${YELLOW}Connecting to Worker 3 to PUT a key-value pair${NC}"
CMD4="put testkey3 testvalue3"
echo -e "${BLUE}Command: $CMD4${NC}"
echo "$CMD4" | bin/client -server localhost:$PORT3

# Test 3: GET from different worker than where PUT
echo -e "\n${BLUE}Test 3: GET values from different workers${NC}"
# Get testkey2 from Worker 1
echo -e "${YELLOW}Connecting to Worker 1 to GET testkey2${NC}"
CMD5="get testkey2"
echo -e "${BLUE}Command: $CMD5${NC}"
echo "$CMD5" | bin/client -server localhost:$PORT1

# Get testkey3 from Worker 2
echo -e "${YELLOW}Connecting to Worker 2 to GET testkey3${NC}"
CMD6="get testkey3"
echo -e "${BLUE}Command: $CMD6${NC}"
echo "$CMD6" | bin/client -server localhost:$PORT2

# Test 4: Update value and check replication
echo -e "\n${BLUE}Test 4: Update a value and check replication${NC}"
# Update testkey1 via Worker 1
echo -e "${YELLOW}Connecting to Worker 1 to update testkey1${NC}"
CMD7="put testkey1 updated-value"
echo -e "${BLUE}Command: $CMD7${NC}"
echo "$CMD7" | bin/client -server localhost:$PORT1

# Get updated value from Worker 3
echo -e "${YELLOW}Connecting to Worker 3 to GET updated testkey1${NC}"
CMD8="get testkey1"
echo -e "${BLUE}Command: $CMD8${NC}"
echo "$CMD8" | bin/client -server localhost:$PORT3

# Clean up
echo -e "\n${YELLOW}Tests completed. Cleaning up...${NC}"

# Kill worker processes
kill $WORKER1_PID
kill $WORKER2_PID
kill $WORKER3_PID

# Stop Redis
redis-cli -p 63079 shutdown

# Optional: Remove logs
# rm -rf temp_logs

echo -e "${GREEN}Test script completed.${NC}"
echo -e "${YELLOW}Check temp_logs directory for worker logs if needed.${NC}" 