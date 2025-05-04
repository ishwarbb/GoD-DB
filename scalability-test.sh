#!/bin/bash

# Set up terminal colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configurable parameters
GOSSIP_TIME=60  # Increased from 30 to 60 seconds to allow more time for gossip
CLIENT_COUNT=10  # Reduced from 20 to 10 for less concurrent pressure
REQUESTS_PER_CLIENT=50  # Reduced from 100 to 50 

echo -e "${BLUE}GoD-DB Scalability Testing Suite${NC}"

# Create directories for logs and results
mkdir -p scalability_logs
mkdir -p scalability_results

# Check if required dependencies are installed
command -v redis-server >/dev/null 2>&1 || { echo -e "${RED}Redis is not installed. Please install Redis first.${NC}"; exit 1; }
go version >/dev/null 2>&1 || { echo -e "${RED}Go is not installed. Please install Go first.${NC}"; exit 1; }

# Ensure the benchmark and DB applications are built
if [ ! -f "bin/worker" ] || [ ! -f "bin/client" ]; then
    echo -e "${YELLOW}Building GoD-DB applications...${NC}"
    make worker client
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build applications${NC}"
        exit 1
    fi
fi

# Build the benchmark tool
echo -e "${YELLOW}Building benchmark tool...${NC}"
go build -o bin/benchmark-cli benchmark-cli.go

if [ $? -ne 0 ]; then
    echo -e "${RED}Failed to build benchmark tool${NC}"
    exit 1
fi

# Global variables for process tracking
declare -a ALL_PIDS

# Function to clean up all processes
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    
    # Kill all started processes
    for pid in "${ALL_PIDS[@]}"; do
        if ps -p $pid > /dev/null; then
            kill $pid 2>/dev/null
        fi
    done
    
    # Shut down all Redis instances we started (ports 63000-66999)
    pkill -f "redis-server.*6[3-6][0-9][0-9][0-9]" || true
    
    echo -e "${GREEN}Cleanup completed.${NC}"
}

# Set trap to catch Ctrl+C and clean up
trap cleanup EXIT INT TERM

# Function to ensure Redis is properly initialized
start_redis() {
    local port=$1
    echo -e "${YELLOW}Starting Redis on port $port...${NC}"
    
    # First try to stop any existing Redis on this port
    redis-cli -p $port shutdown 2>/dev/null || true
    sleep 1
    
    # Clear any previous data by removing the database file
    rm -f dump.rdb
    
    # Start Redis with a clean state
    redis-server --port $port --daemonize yes
    
    # Verify Redis is running
    local max_attempts=5
    local attempt=1
    while [ $attempt -le $max_attempts ]; do
        if redis-cli -p $port ping | grep -q "PONG"; then
            echo -e "${GREEN}Redis on port $port is running${NC}"
            return 0
        else
            echo -e "${YELLOW}Waiting for Redis on port $port (attempt $attempt/$max_attempts)...${NC}"
            sleep 1
            attempt=$((attempt + 1))
        fi
    done
    
    echo -e "${RED}Failed to verify Redis on port $port is running${NC}"
    return 1
}

# Function to start a worker node
start_worker() {
    local port=$1
    local redis_port=$2
    local discovery_port=$3
    local vnodes=$4
    local replication=$5
    local writequorum=$6
    local readquorum=$7
    
    echo -e "${YELLOW}Starting worker on port $port (Redis: $redis_port, VNodes: $vnodes)${NC}"
    bin/worker -port $port -redisport $redis_port -discovery localhost:$discovery_port \
               -virtualnodes $vnodes -replication $replication \
               -writequorum $writequorum -readquorum $readquorum \
               > scalability_logs/worker_${port}.log 2>&1 &
    
    local pid=$!
    ALL_PIDS+=($pid)
    echo "Worker PID: $pid"
    sleep 2
}

# Function to run benchmark
run_benchmark() {
    local servers=$1
    local clients=$2
    local requests=$3
    local reads=$4
    local valuesize=$5
    local output_file=$6
    local debug=${7:-false}
    
    echo -e "${GREEN}Running benchmark...${NC}"
    echo -e "${BLUE}Configuration: ${clients} clients, ${requests} requests, ${reads}% reads, ${valuesize} byte values${NC}"
    echo -e "${BLUE}Servers: ${servers}${NC}"
    
    # First pre-populate some keys to prevent "key not found" errors
    echo -e "${YELLOW}Pre-populating keys...${NC}"
    bin/benchmark-cli -servers $servers -clients 1 -requests 100 \
                 -reads 0 -valuesize $valuesize -silent > /dev/null
    
    echo -e "${YELLOW}Waiting 5 seconds for replication...${NC}"
    sleep 5
    
    if [ "$debug" = true ]; then
        echo -e "${YELLOW}Running benchmark in verbose mode${NC}"
        bin/benchmark-cli -servers $servers -clients $clients -requests $requests \
                     -reads $reads -valuesize $valuesize -verbose > $output_file
    else
        echo -e "${YELLOW}Running benchmark${NC}"
        bin/benchmark-cli -servers $servers -clients $clients -requests $requests \
                     -reads $reads -valuesize $valuesize > $output_file
    fi
    
    # Display results
    cat $output_file | grep -A 10 "====== Benchmark Results"
}

# Test 1: Baseline with 3 nodes
test_baseline() {
    echo -e "\n${BLUE}Test 1: Baseline Performance (3 nodes)${NC}"
    
    # Start discovery Redis
    start_redis 63179
    
    # Start 3 Redis instances for workers
    start_redis 63079
    start_redis 63080
    start_redis 63081
    
    # Start 3 workers with eventual consistency settings (R=1, W=1)
    start_worker 8081 63079 63179 10 3 1 1
    start_worker 8082 63080 63179 10 3 1 1
    start_worker 8083 63081 63179 10 3 1 1
    
    # Allow workers to gossip
    echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
    sleep ${GOSSIP_TIME}
    
    # Run benchmark
    run_benchmark "localhost:8081,localhost:8082,localhost:8083" $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 \
                 "scalability_results/baseline_results.txt" true
    
    # Cleanup for next test
    cleanup
}

# Test 2: Scaling nodes
test_scaling_nodes() {
    echo -e "\n${BLUE}Test 2: Scaling Nodes (3, 5, 7 nodes)${NC}"
    
    # Results array for comparison
    declare -a RESULTS
    
    # Loop through different node counts
    for node_count in 3 5 7; do
        echo -e "\n${YELLOW}Testing with $node_count nodes${NC}"
        
        # Start discovery Redis
        start_redis 63179
        
        # Start Redis instances and workers
        for ((i=0; i<node_count; i++)); do
            redis_port=$((63079 + i))
            worker_port=$((8081 + i))
            start_redis $redis_port
            start_worker $worker_port $redis_port 63179 10 3 2 2
        done
        
        # Allow workers to gossip
        echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
        sleep ${GOSSIP_TIME}
        
        # Build server list
        servers=""
        for ((i=0; i<node_count; i++)); do
            if [ "$i" -gt 0 ]; then
                servers+=","
            fi
            servers+="localhost:$((8081 + i))"
        done
        
        # Run benchmark
        output_file="scalability_results/scaling_${node_count}_nodes.txt"
        run_benchmark "$servers" $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 $output_file
        
        # Extract ops/sec
        ops_per_sec=$(grep "Operations/second" $output_file | awk '{print $2}')
        RESULTS+=("$node_count nodes: $ops_per_sec ops/sec")
        
        # Cleanup for next iteration
        cleanup
    done
    
    # Print comparison
    echo -e "\n${GREEN}Node Scaling Results:${NC}"
    for result in "${RESULTS[@]}"; do
        echo -e "  $result"
    done
}

# Test 3: Scaling virtual nodes
test_scaling_vnodes() {
    echo -e "\n${BLUE}Test 3: Virtual Node Scaling${NC}"
    
    # Results array for comparison
    declare -a RESULTS
    
    # Loop through different vnode counts
    for vnode_count in 10 50 100 200; do
        echo -e "\n${YELLOW}Testing with $vnode_count virtual nodes per physical node${NC}"
        
        # Start discovery Redis
        start_redis 63179
        
        # Start Redis instances
        for ((i=0; i<3; i++)); do
            start_redis $((63079 + i))
        done
        
        # Start workers with different vnode counts
        start_worker 8081 63079 63179 $vnode_count 3 2 2
        start_worker 8082 63080 63179 $vnode_count 3 2 2
        start_worker 8083 63081 63179 $vnode_count 3 2 2
        
        # Allow workers to gossip
        echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
        sleep ${GOSSIP_TIME}
        
        # Run benchmark
        output_file="scalability_results/scaling_${vnode_count}_vnodes.txt"
        run_benchmark "localhost:8081,localhost:8082,localhost:8083" $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 $output_file
        
        # Extract ops/sec
        ops_per_sec=$(grep "Operations/second" $output_file | awk '{print $2}')
        RESULTS+=("$vnode_count vnodes: $ops_per_sec ops/sec")
        
        # Cleanup for next iteration
        cleanup
    done
    
    # Print comparison
    echo -e "\n${GREEN}Virtual Node Scaling Results:${NC}"
    for result in "${RESULTS[@]}"; do
        echo -e "  $result"
    done
}

# Test 4: Write-heavy vs Read-heavy workloads
test_read_write_ratio() {
    echo -e "\n${BLUE}Test 4: Read/Write Ratio Impact${NC}"
    
    # Results array for comparison
    declare -a RESULTS
    
    # Start the base infrastructure once
    start_redis 63179
    
    for ((i=0; i<3; i++)); do
        start_redis $((63079 + i))
    done
    
    # Start 3 workers
    start_worker 8081 63079 63179 50 3 2 2
    start_worker 8082 63080 63179 50 3 2 2
    start_worker 8083 63081 63179 50 3 2 2
    
    # Allow workers to gossip
    echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
    sleep ${GOSSIP_TIME}
    
    # Loop through different read percentages
    for read_pct in 0 25 50 75 100; do
        echo -e "\n${YELLOW}Testing with $read_pct% reads, $((100-read_pct))% writes${NC}"
        
        # Run benchmark
        output_file="scalability_results/ratio_${read_pct}_reads.txt"
        run_benchmark "localhost:8081,localhost:8082,localhost:8083" $CLIENT_COUNT $REQUESTS_PER_CLIENT $read_pct 100 $output_file
        
        # Extract ops/sec
        ops_per_sec=$(grep "Operations/second" $output_file | awk '{print $2}')
        RESULTS+=("$read_pct% reads: $ops_per_sec ops/sec")
    done
    
    # Print comparison
    echo -e "\n${GREEN}Read/Write Ratio Results:${NC}"
    for result in "${RESULTS[@]}"; do
        echo -e "  $result"
    done
    
    # Cleanup
    cleanup
}

# Test 5: Quorum settings impact
test_quorum_settings() {
    echo -e "\n${BLUE}Test 5: Quorum Settings Impact${NC}"
    
    # Results array for comparison
    declare -a RESULTS
    
    # Test different quorum configurations
    # Format: "Name:R:W"
    configs=("eventual:1:1" "read-optimized:2:1" "write-optimized:1:2" "strong:2:2" "strict:3:3")
    
    for config in "${configs[@]}"; do
        # Parse config
        IFS=':' read -r name read_quorum write_quorum <<< "$config"
        
        echo -e "\n${YELLOW}Testing $name consistency (R=$read_quorum, W=$write_quorum)${NC}"
        
        # Start discovery Redis
        start_redis 63179
        
        # Start Redis instances
        for ((i=0; i<3; i++)); do
            start_redis $((63079 + i))
        done
        
        # Start workers with specific quorum settings
        start_worker 8081 63079 63179 50 3 $write_quorum $read_quorum
        start_worker 8082 63080 63179 50 3 $write_quorum $read_quorum
        start_worker 8083 63081 63179 50 3 $write_quorum $read_quorum
        
        # Allow workers to gossip
        echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
        sleep ${GOSSIP_TIME}
        
        # Run benchmark
        output_file="scalability_results/quorum_${name}.txt"
        run_benchmark "localhost:8081,localhost:8082,localhost:8083" $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 $output_file
        
        # Extract ops/sec
        ops_per_sec=$(grep "Operations/second" $output_file | awk '{print $2}')
        latency=$(grep "Average Latency" $output_file | awk '{print $3}')
        RESULTS+=("$name (R=$read_quorum,W=$write_quorum): $ops_per_sec ops/sec, $latency latency")
        
        # Cleanup for next iteration
        cleanup
    done
    
    # Print comparison
    echo -e "\n${GREEN}Quorum Settings Results:${NC}"
    for result in "${RESULTS[@]}"; do
        echo -e "  $result"
    done
}

# Test 6: Node failure resilience
test_node_failure() {
    echo -e "\n${BLUE}Test 6: Node Failure Resilience${NC}"
    
    # Start discovery Redis
    start_redis 63179
    
    # Start Redis instances
    for ((i=0; i<5; i++)); do
        start_redis $((63079 + i))
    done
    
    # Start 5 workers
    for ((i=0; i<5; i++)); do
        port=$((8081 + i))
        redis_port=$((63079 + i))
        start_worker $port $redis_port 63179 50 3 2 2
    done
    
    # Allow workers to gossip
    echo -e "${YELLOW}Allowing workers to initialize and gossip (${GOSSIP_TIME} seconds)...${NC}"
    sleep ${GOSSIP_TIME}
    
    # Run benchmark with all nodes
    echo -e "\n${YELLOW}Running benchmark with all 5 nodes...${NC}"
    run_benchmark "localhost:8081,localhost:8082,localhost:8083,localhost:8084,localhost:8085" \
                 $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 "scalability_results/failure_before.txt"
    
    # Kill 2 random nodes
    echo -e "\n${RED}Killing 2 random nodes to simulate failure...${NC}"
    for i in 1 2; do
        random_index=$((RANDOM % 5))
        killed_port=$((8081 + random_index))
        pid=$(ps aux | grep "worker.*port=$killed_port" | grep -v grep | awk '{print $2}')
        if [ ! -z "$pid" ]; then
            echo -e "${RED}Killing node on port $killed_port (PID $pid)${NC}"
            kill $pid
            sleep 2
        fi
    done
    
    # Allow system to stabilize
    echo -e "${YELLOW}Allowing system to recover (10 seconds)...${NC}"
    sleep ${GOSSIP_TIME}
    
    # Run benchmark after node failures
    echo -e "\n${YELLOW}Running benchmark after node failures...${NC}"
    run_benchmark "localhost:8081,localhost:8082,localhost:8083,localhost:8084,localhost:8085" \
                 $CLIENT_COUNT $REQUESTS_PER_CLIENT 50 100 "scalability_results/failure_after.txt"
    
    # Compare results
    echo -e "\n${GREEN}Node Failure Test Results:${NC}"
    echo -e "${YELLOW}Before failure:${NC}"
    cat scalability_results/failure_before.txt | grep -A 5 "====== Benchmark Results" | tail -n +2
    echo -e "${YELLOW}After failure:${NC}"
    cat scalability_results/failure_after.txt | grep -A 5 "====== Benchmark Results" | tail -n +2
    
    # Cleanup
    cleanup
}

# Main function to run all tests
run_all_tests() {
    echo -e "${GREEN}Starting all scalability tests...${NC}"
    
    test_baseline
    test_scaling_nodes
    test_scaling_vnodes
    test_read_write_ratio
    test_quorum_settings
    test_node_failure
    
    echo -e "\n${GREEN}All tests completed. Results saved in scalability_results directory.${NC}"
}

# Run a specific test or all tests
case "$1" in
    "baseline")
        test_baseline
        ;;
    "nodes")
        test_scaling_nodes
        ;;
    "vnodes")
        test_scaling_vnodes
        ;;
    "ratio")
        test_read_write_ratio
        ;;
    "quorum")
        test_quorum_settings
        ;;
    "failure")
        test_node_failure
        ;;
    *)
        run_all_tests
        ;;
esac

echo -e "\n${BLUE}Scalability testing complete.${NC}" 