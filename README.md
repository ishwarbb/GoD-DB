# GoD-DB (Go DynamoDB-like System)

A Go-based implementation of a DynamoDB-like distributed key-value store, featuring consistent hashing, replication, gossip-based membership, and quorum-based reads and writes.

## Features

- **Consistent Hashing**: Keys are distributed across nodes using consistent hashing for scalability and fault tolerance.
- **Virtual Nodes**: Each physical node is represented by multiple virtual nodes on the hash ring for better distribution.
- **Dynamic Membership**: Nodes can join and leave the cluster dynamically via gossip protocol.
- **Tunable Consistency**: Configurable read quorum (R) and write quorum (W) for different consistency levels.
- **Data Replication**: Keys are replicated across N nodes for fault tolerance.
- **Read Repair**: Inconsistent replicas are repaired during read operations.

## Virtual Nodes in GoD-DB

Virtual nodes (or "vnodes") are a technique used in GoD-DB's consistent hashing implementation to improve key distribution and balance load across physical nodes, following the approach used in Amazon's DynamoDB.

### How Virtual Nodes Work

Rather than representing each physical node as a single point on the hash ring, virtual nodes allow each physical node to be represented by multiple points on the ring. This provides several benefits:

1. **Better Distribution**: Virtual nodes result in more uniform distribution of keys across physical nodes.
2. **Smoother Rebalancing**: When nodes join or leave, virtual nodes ensure that only a small fraction of keys need to be redistributed, spread across multiple physical nodes.
3. **Heterogeneous Nodes**: By assigning more virtual nodes to more powerful hardware, the system can account for heterogeneous node capabilities.

### Configuration

The number of virtual nodes can be configured per node:

```bash
# Start a worker with 100 virtual nodes per physical node
go run cmd/worker/main.go -virtualnodes=100
```

The default is 10 virtual nodes per physical node.

### Implementation Details

In the implementation:

- Each physical node is mapped to multiple positions on the hash ring by creating multiple hash values
- For node identification, virtual node IDs are created by appending a suffix to the physical node ID
- The mapping from virtual nodes to physical nodes is maintained internally by the consistent hashing algorithm
- Key lookups are performed using the standard consistent hashing algorithm, treating virtual nodes as regular nodes

## Usage

### Starting a Worker Node

```bash
go run cmd/worker/main.go -port=50051 -redisport=63079 -virtualnodes=50
```

### Starting a Client

```bash
go run cmd/client/main.go -server=localhost:50051
```

## Configuration Options

Worker nodes accept the following configuration options:

- `-port`: Port to listen on for RPC requests
- `-redisport`: Port for the Redis storage backend
- `-virtualnodes`: Number of virtual nodes per physical node (default: 10)
- `-replication`: Replication factor (N)
- `-writequorum`: Write quorum (W)
- `-readquorum`: Read quorum (R)
- `-gossip`: Gossip interval (e.g., "10s")
- `-peers`: Number of peers to gossip with in each round
- `-enablegossip`: Enable/disable gossip protocol
- `-discovery`: Redis address for service discovery

## Running Tests

```bash
go test ./...
```

> sudo apt-get install redis-server