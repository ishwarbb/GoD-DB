# DynamoMini Go Implementation Plan

This document outlines a structured approach to implement DynamoMini in Go, focusing on clear steps and function responsibilities rather than code implementations.

## System Overview

DynamoMini is a distributed key-value store inspired by Amazon DynamoDB, providing:
- Decentralized architecture with no single point of failure
- Data distribution via consistent hashing
- Replication for fault tolerance
- Vector clocks for conflict resolution
- Tunable consistency (configurable read/write quorums)
- Gossip protocol for membership and failure detection

## Iteration 1: Core Architecture and Basic Components

**Goal**: Establish the project structure, core interfaces, and basic components.

### Components to Implement:

#### 1. Project Structure
- Set up Go modules
- Create directory structure following Go best practices
- Define package boundaries and dependencies

#### 2. Configuration Management
**Package**: `internal/config`
- `LoadConfig(path string) (*Config, error)`: Load configuration from file
  - Input: Path to configuration file
  - Output: Configuration struct, error
  - Edge cases: Missing file, malformed configuration

- `Config` struct should include:
  - Server settings (host, port)
  - Redis connection details
  - Replication factor (N)
  - Read/write quorum values (R, W)
  - Gossip interval
  - Timeouts

#### 3. Storage Interface
**Package**: `internal/storage`
- `Storage` interface with methods:
  - `Set(key string, value []byte, expiry time.Duration) error`: Store a key-value pair
  - `Get(key string) ([]byte, error)`: Retrieve a value by key
  - `Delete(key string) error`: Remove a key-value pair
  - `Keys(pattern string) ([]string, error)`: List keys matching a pattern
  - `Close() error`: Close the storage connection
  - Edge cases: Connection failures, timeouts, concurrent access

- `NewRedisStorage(addr, password string, db int) (Storage, error)`: Redis storage implementation
  - Input: Redis connection parameters
  - Output: Storage implementation, error
  - Edge cases: Connection failures, authentication issues

#### 4. Core Models
**Package**: `internal/models`
- `NodeConfig` struct: Node configuration
- `Node` struct: Node representation in the system
- `KeyRange` struct: Range of keys a node is responsible for
- `NodeStatus` type: Enum for node status (Active, Suspect, Down)

#### 5. Hash Utility
**Package**: `pkg/utils`
- `HashKey(key string) uint64`: Generate a hash for a key
  - Input: String key
  - Output: 64-bit hash value
  - Implementation: MD5 or other consistent algorithm
  - Edge cases: Empty strings, special characters, hash collisions

## Iteration 2: Consistent Hashing Implementation

**Goal**: Implement the consistent hashing algorithm and node management.

### Components to Implement:

#### 1. Hash Ring
**Package**: `internal/hashring`
- `NewRing(vnodeCount, replicaCount int) *Ring`: Create a new consistent hash ring
  - Input: Virtual node count, replica count
  - Output: Initialized Ring structure
  - Edge cases: Invalid input parameters

- `AddNode(node *models.Node) error`: Add a node to the ring
  - Input: Node to add
  - Output: Error if operation fails
  - Side effects: Updates internal hash ring, recalculates key ranges
  - Edge cases: Node already exists, concurrent modifications

- `RemoveNode(nodeID string) error`: Remove a node from the ring
  - Input: ID of node to remove
  - Output: Error if operation fails
  - Side effects: Updates internal ring, recalculates key ranges
  - Edge cases: Node doesn't exist, concurrent modifications

- `GetNodeForKey(key string) (*models.Node, error)`: Find node responsible for key
  - Input: Key string
  - Output: Responsible node, error
  - Edge cases: Empty ring, key hashing issues

- `GetReplicaNodes(key string, count int) ([]*models.Node, error)`: Get replica nodes for key
  - Input: Key string, number of replicas desired
  - Output: Slice of replica nodes, error
  - Edge cases: Not enough nodes for requested replicas

#### 2. Hash Ring Service
**Package**: `internal/hashring`
- `NewHashRingService(config *config.Config) (Service, error)`: Create hash ring service
  - Input: System configuration
  - Output: Hash ring service implementation, error
  - Edge cases: Invalid configuration

- `HandleNodeJoin(req *NodeJoinRequest) (*NodeJoinResponse, error)`: Handle node join requests
  - Input: Join request with node details
  - Output: Join response with assigned ranges, error
  - Side effects: Updates ring state
  - Edge cases: Duplicate nodes, concurrent joins

- `HandleNodeFailure(nodeID string) error`: Handle node failure
  - Input: ID of failed node
  - Output: Error if operation fails
  - Side effects: Updates ring state, triggers rebalancing
  - Edge cases: Node doesn't exist, concurrent failures

- `GetRoutingInfo() (*RoutingInfo, error)`: Get current routing information
  - Input: None
  - Output: Routing information, error
  - Edge cases: Empty ring

## Iteration 3: Vector Clocks and Data Replication

**Goal**: Implement versioning with vector clocks and data replication mechanisms.

### Components to Implement:

#### 1. Vector Clock
**Package**: `internal/models`
- `NewVectorClock() *VectorClock`: Create a new vector clock
  - Output: Initialized vector clock
  - Edge cases: None significant

- `Increment(nodeID string)`: Increment counter for a node
  - Input: Node ID
  - Side effects: Updates counter for node, updates timestamp
  - Edge cases: None significant

- `Compare(other *VectorClock) int`: Compare two vector clocks
  - Input: Other vector clock
  - Output: -1 (before), 0 (concurrent), 1 (after)
  - Edge cases: Empty vector clocks, partially overlapping clocks

- `Merge(other *VectorClock)`: Merge with another vector clock
  - Input: Other vector clock
  - Side effects: Updates counters to maximum values
  - Edge cases: None significant

#### 2. Versioned Value
**Package**: `internal/models`
- `NewVersionedValue(value []byte, nodeID string) *VersionedValue`: Create versioned value
  - Input: Value bytes, node ID
  - Output: Versioned value with initial vector clock
  - Edge cases: Empty value

- `Update(value []byte, nodeID string)`: Update value and vector clock
  - Input: New value, node ID
  - Side effects: Updates value, increments vector clock
  - Edge cases: None significant

- `Merge(other *VersionedValue) bool`: Resolve conflicts between values
  - Input: Other versioned value
  - Output: True if local value was updated
  - Side effects: May update value based on vector clock comparison
  - Edge cases: Concurrent updates (needs careful handling)

- `Serialize() ([]byte, error)`: Serialize to bytes
  - Output: Serialized value, error
  - Edge cases: Serialization errors

- `Deserialize(data []byte) (*VersionedValue, error)`: Deserialize from bytes
  - Input: Serialized bytes
  - Output: Versioned value, error
  - Edge cases: Malformed input, version incompatibility

#### 3. Replication Manager
**Package**: `internal/worker`
- `NewReplicationManager(nodeID string, storage Storage, routingTable *RoutingTable) *ReplicationManager`: Create replication manager
  - Input: Node ID, storage interface, routing table
  - Output: Replication manager
  - Edge cases: Invalid inputs

- `Start()`: Start background replication processes
  - Side effects: Launches goroutines for replication
  - Edge cases: Already started

- `Stop()`: Stop replication processes
  - Side effects: Stops all background goroutines
  - Edge cases: Already stopped

- `AddReplicationTask(key string, value *VersionedValue, targets []*models.Node)`: Queue replication task
  - Input: Key, value, target nodes
  - Side effects: Adds task to queue
  - Edge cases: Queue full

- `ProcessReplicatedPut(key string, data []byte) error`: Handle incoming replication
  - Input: Key, serialized value
  - Output: Error if operation fails
  - Side effects: Updates local storage with replicated value
  - Edge cases: Deserializing errors, storage failures

#### 4. Worker Service (Data Operations)
**Package**: `internal/worker`
- `Put(key string, value []byte) error`: Store key-value pair
  - Input: Key, value
  - Output: Error if operation fails
  - Side effects: Updates local storage, triggers replication
  - Edge cases: Storage failures, replication failures, quorum not met

- `Get(key string) ([]byte, error)`: Retrieve value for key
  - Input: Key
  - Output: Value, error
  - Side effects: May trigger read repair if inconsistencies detected
  - Edge cases: Key not found, storage failures, quorum not met

- `getReplicaNodesForKey(key string) ([]*models.Node, error)`: Get replica nodes for key
  - Input: Key
  - Output: Replica nodes, error
  - Edge cases: Hash ring errors, not enough nodes

## Iteration 4: Gossip Protocol and Fault Tolerance

**Goal**: Implement node membership, failure detection, and recovery mechanisms.

### Components to Implement:

#### 1. Gossip Manager
**Package**: `internal/worker`
- `NewGossipManager(nodeID string, routingTable *RoutingTable, config *config.Config) *GossipManager`: Create gossip manager
  - Input: Node ID, routing table, configuration
  - Output: Gossip manager
  - Edge cases: Invalid inputs

- `Start()`: Start gossip protocol
  - Side effects: Launches background goroutines
  - Edge cases: Already started

- `Stop()`: Stop gossip protocol
  - Side effects: Stops background goroutines
  - Edge cases: Already stopped

- `sendGossip()`: Send gossip to random nodes
  - Side effects: Updates other nodes' routing tables
  - Edge cases: No other nodes, network failures

- `checkNodeFailures()`: Check for failed nodes
  - Side effects: Marks nodes as suspect or down
  - Edge cases: False positives due to network issues

- `markNodeSuspect(nodeID string)`: Mark node as suspect
  - Input: Node ID
  - Side effects: Updates node status in routing table, triggers verification
  - Edge cases: Node already suspect/down

- `markNodeDead(nodeID string)`: Mark node as dead
  - Input: Node ID
  - Side effects: Updates node status, triggers data rebalancing
  - Edge cases: Node already marked dead

#### 2. Recovery Manager
**Package**: `internal/worker`
- `NewRecoveryManager(nodeID string, storage Storage, routingTable *RoutingTable) *RecoveryManager`: Create recovery manager
  - Input: Node ID, storage interface, routing table
  - Output: Recovery manager
  - Edge cases: Invalid inputs

- `Start()`: Start recovery processes
  - Side effects: Launches background goroutines
  - Edge cases: Already started

- `Stop()`: Stop recovery processes
  - Side effects: Stops background goroutines
  - Edge cases: Already stopped

- `checkDownNodes()`: Check if down nodes recovered
  - Side effects: May trigger node recovery processes
  - Edge cases: No down nodes

- `syncDataForRecoveredNode(nodeID string)`: Synchronize data for recovered node
  - Input: Node ID
  - Side effects: Fetches data from other nodes
  - Edge cases: Node not recovered, data transfer failures

#### 3. Client Implementation
**Package**: `pkg/api`
- `NewDynamoClient(config ClientConfig) (*DynamoClient, error)`: Create client
  - Input: Client configuration
  - Output: Client instance, error
  - Edge cases: Invalid configuration

- `Put(key string, value []byte) error`: Store value
  - Input: Key, value
  - Output: Error if operation fails
  - Side effects: Stores value on multiple nodes
  - Edge cases: No nodes available, write quorum not met

- `Get(key string) ([]byte, error)`: Retrieve value
  - Input: Key
  - Output: Value, error
  - Edge cases: Key not found, read quorum not met

- `GetNodeStatus() (*SystemStatus, error)`: Get system status
  - Output: System status, error
  - Edge cases: Cannot contact hash ring

- `refreshNodeCache(ctx context.Context)`: Refresh node information cache
  - Input: Context for cancellation
  - Side effects: Updates node cache
  - Edge cases: Hash ring unavailable

#### 4. Worker Node Service (Main Service)
**Package**: `cmd/worker`
- `StartWorkerService(config *config.Config) error`: Start worker service
  - Input: Configuration
  - Output: Error if startup fails
  - Side effects: Initializes all components, joins hash ring
  - Edge cases: Configuration errors, hash ring unavailable

## Edge Cases and Potential Failures

This section outlines the key failure scenarios and edge cases that need to be handled:

### Network Failures
1. **Temporary Network Partitions**:
   - Nodes may incorrectly mark other nodes as failed
   - Solution: Use suspicion mechanism with multiple confirmations

2. **Network Latency Spikes**:
   - Operations may time out, causing incomplete replication
   - Solution: Background reconciliation, retry mechanisms

3. **Network Asymmetry**:
   - Node A can reach B but B cannot reach A
   - Solution: Gossip protocol with indirect confirmation

### Node Failures
1. **Permanent Node Failure**:
   - Data loss if not properly replicated
   - Solution: Ensure replication factor > 1, hinted handoff

2. **Transient Node Failure**:
   - Rebalancing triggered unnecessarily
   - Solution: Delay rebalancing until node confirmed down

3. **Partial Failure**:
   - Node still responds but cannot access storage
   - Solution: Health checks include storage operations

### Data Consistency Issues
1. **Split Brain**:
   - Network partition causes divergent updates
   - Solution: Vector clocks, read repair, anti-entropy

2. **Stale Reads**:
   - Reading from outdated replicas
   - Solution: Read quorum (R), vector clock reconciliation

3. **Inconsistent Writes**:
   - Partial write completion
   - Solution: Write quorum (W), background reconciliation

4. **Clock Drift**:
   - Physical time used in vector clocks affected by drift
   - Solution: Logical clocks, NTP synchronization

### Operational Issues
1. **Hot Spots**:
   - Uneven data distribution
   - Solution: Consistent hashing with virtual nodes

2. **Flash Crowds**:
   - Sudden traffic spikes overload nodes
   - Solution: Rate limiting, backpressure mechanisms

3. **Resource Exhaustion**:
   - Memory/disk space depletion
   - Solution: Resource monitoring, graceful degradation

4. **Configuration Errors**:
   - Misconfigured nodes (wrong ports, addresses)
   - Solution: Configuration validation, diagnostics API

### Implementation Challenges
1. **Deadlocks**:
   - Improper mutex handling
   - Solution: Strict lock ordering, timeouts on operations

2. **Memory Leaks**:
   - Long-running service accumulates memory
   - Solution: Memory profiling, proper resource cleanup

3. **Goroutine Leaks**:
   - Unfinished goroutines
   - Solution: Context cancellation, WaitGroups

4. **Race Conditions**:
   - Concurrent map access, inconsistent state
   - Solution: Proper synchronization, Go race detector

### Recovery Complications
1. **Failed Recovery**:
   - Node joins but fails during data synchronization
   - Solution: Atomicity in join process, rollback mechanism

2. **Cascading Failures**:
   - One node failure triggers others due to load
   - Solution: Circuit breakers, load shedding

3. **Incomplete Metadata**:
   - Routing information lost or corrupted
   - Solution: Persistent metadata, consensus protocols

4. **Data Corruption**:
   - Disk errors or buggy code corrupts stored data
   - Solution: Checksums, backup mechanisms

## Recommendations for Robustness
1. Implement extensive logging with proper levels
2. Create comprehensive metrics for monitoring
3. Develop thorough testing (unit, integration, chaos)
4. Start with simpler consistency models, then enhance
5. Implement graceful degradation modes
6. Design for operability with admin interfaces
7. Plan for version upgrades and backward compatibility 