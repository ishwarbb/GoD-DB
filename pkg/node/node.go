package node

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"no.cap/goddb/pkg/chash"
	"no.cap/goddb/pkg/rpc"
	"no.cap/goddb/pkg/storage"
)

// NodeConfig holds configuration parameters for a Node
type NodeConfig struct {
	Port               int
	RedisPort          int
	ReplicationFactorN int
	WriteQuorumW       int
	ReadQuorumR        int
}

// DefaultNodeConfig returns a NodeConfig with sensible defaults
func DefaultNodeConfig(port, redisPort int) NodeConfig {
	return NodeConfig{
		Port:               port,
		RedisPort:          redisPort,
		ReplicationFactorN: 3,
		WriteQuorumW:       2,
		ReadQuorumR:        2,
	}
}

// ConnectionManager manages gRPC connections to other nodes
type ConnectionManager struct {
	connCache map[string]*grpc.ClientConn
	mutex     sync.RWMutex
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connCache: make(map[string]*grpc.ClientConn),
	}
}

// GetConnection returns a gRPC connection to the specified address
// It caches connections to avoid creating new ones for repeated calls
func (cm *ConnectionManager) GetConnection(address string) (*grpc.ClientConn, error) {
	// Check if we have a cached connection
	cm.mutex.RLock()
	conn, exists := cm.connCache[address]
	cm.mutex.RUnlock()

	if exists {
		return conn, nil
	}

	// Create a new connection
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	// Cache the connection
	cm.mutex.Lock()
	cm.connCache[address] = conn
	cm.mutex.Unlock()

	return conn, nil
}

// Close closes all managed connections
func (cm *ConnectionManager) Close() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for addr, conn := range cm.connCache {
		conn.Close()
		delete(cm.connCache, addr)
	}
}

type Node struct {
	rpc.UnimplementedNodeServiceServer
	Port      int
	RedisPort int
	ring      *chash.Ring
	store     *storage.Store
	meta      rpc.NodeMeta
	// Configuration parameters for replication
	ReplicationFactorN int
	WriteQuorumW       int
	ReadQuorumR        int
	// Connection manager for outgoing RPC calls
	connMgr *ConnectionManager
}

func NewNode(config NodeConfig) *Node {
	store, err := storage.NewStore(
		fmt.Sprintf("localhost:%d", config.RedisPort),
	)

	ring := chash.NewRing()
	for i := 0; i < 5; i++ {
		ring.AddNode(rpc.NodeMeta{
			Ip:     "localhost",
			Port:   int32(config.Port),
			NodeId: fmt.Sprintf("localhost:%d", config.Port),
		})
	}

	if err != nil {
		panic(err) // TODO: deal with this better
	}

	return &Node{
		Port:      config.Port,
		RedisPort: config.RedisPort,
		ring:      ring,
		store:     store,
		meta: rpc.NodeMeta{
			NodeId: fmt.Sprintf("localhost:%d", config.Port),
			Ip:     "localhost",
			Port:   int32(config.Port),
		},
		// Set replication parameters from config
		ReplicationFactorN: config.ReplicationFactorN,
		WriteQuorumW:       config.WriteQuorumW,
		ReadQuorumR:        config.ReadQuorumR,
		// Initialize connection manager
		connMgr: NewConnectionManager(),
	}
}
