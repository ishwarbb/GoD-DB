# GoD-DB Scalability Testing Suite

This directory contains a comprehensive suite of tools for testing and demonstrating the scalability of the GoD-DB distributed key-value store.

## Overview

The scalability testing suite consists of:

1. A benchmark tool for performance testing
2. A testing script for automating various scalability test scenarios
3. Prometheus metrics integration for monitoring
4. Grafana dashboards for visualization
5. Docker Compose setup for containerized testing
6. Visualization tools for generating reports

## Getting Started

### Prerequisites

- Go 1.20+
- Redis
- Docker and Docker Compose (for containerized testing)
- Python 3.6+ with matplotlib and numpy (for visualization)

### Running Basic Tests

To run all scalability tests:

```bash
./scalability-test.sh
```

To run a specific test:

```bash
./scalability-test.sh baseline   # Run baseline test
./scalability-test.sh nodes      # Test scaling number of nodes
./scalability-test.sh vnodes     # Test virtual node scaling
./scalability-test.sh ratio      # Test read/write ratio impact
./scalability-test.sh quorum     # Test different quorum settings
./scalability-test.sh failure    # Test node failure resilience
```

### Visualization

After running tests, you can generate visual reports:

```bash
python3 visualize_results.py
```

## Docker Deployment

To run scalability tests in a containerized environment:

```bash
# Build and start the infrastructure
docker-compose up -d

# Run benchmark against containerized cluster
go run benchmark.go -servers localhost:8081,localhost:8082,localhost:8083 -clients 20 -requests 5000
```

Access Grafana dashboard at: http://localhost:3000 (username: admin, password: admin)

## Test Scenarios

### 1. Baseline Performance

Tests the basic performance of the system with a fixed configuration (3 nodes, 10 virtual nodes per physical node, replication factor of 3, write quorum of 2, read quorum of 2).

### 2. Node Scaling

Tests how the system scales as you add more physical nodes. Tests with 3, 5, and 7 nodes to measure how throughput and latency change.

### 3. Virtual Node Scaling

Tests the impact of virtual node count on performance and distribution. Compares configurations with 10, 50, 100, and 200 virtual nodes per physical node.

### 4. Read/Write Ratio Impact

Tests how different workload patterns (read-heavy vs. write-heavy) affect system performance.

### 5. Quorum Settings

Tests different consistency levels by varying read and write quorum settings:

- Eventual consistency (R=1, W=1)
- Read-optimized (R=2, W=1)
- Write-optimized (R=1, W=2)
- Strong consistency (R=2, W=2)
- Strict consistency (R=3, W=3)

### 6. Node Failure Resilience

Tests how the system behaves when nodes fail and measures the impact on performance and availability.

## Metrics

The worker nodes expose Prometheus metrics at `/metrics` endpoint, including:

- Operation throughput (ops/sec)
- Operation latency (p95, p99)
- Key count and data size
- Replication operations
- Quorum failures
- Gossip protocol metrics
- Hash ring changes

## Extending the Tests

To add new test scenarios:

1. Add a new test function in `scalability-test.sh`
2. Add corresponding visualization support in `visualize_results.py`
3. Update this documentation

## Troubleshooting

If you encounter issues:

- Check logs in the `scalability_logs` directory
- Verify Redis instances are running properly
- Ensure ports are not already in use
- Check that Go version is compatible 