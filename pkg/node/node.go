package node

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

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
	// Mutex to protect meta updates
	metaMutex sync.RWMutex
}

// IncrementVersion atomically increments the node's version counter
// and updates its metadata. This should be called whenever the node's
// ring state changes (e.g., when adding/removing/updating nodes)
func (n *Node) IncrementVersion() int64 {
	n.metaMutex.Lock()
	defer n.metaMutex.Unlock()

	n.meta.Version++
	return n.meta.Version
}

// GetCurrentVersion returns the node's current version
func (n *Node) GetCurrentVersion() int64 {
	n.metaMutex.RLock()
	defer n.metaMutex.RUnlock()

	return n.meta.Version
}

func NewNode(config NodeConfig) *Node {
	// Initialize random seed for gossip peer selection
	rand.NewSource(time.Now().UnixNano())

	store, err := storage.NewStore(
		fmt.Sprintf("localhost:%d", config.RedisPort),
	)

	ring := chash.NewRing()
	for i := 0; i < 5; i++ {
		ring.AddNode(rpc.NodeMeta{
			Ip:      "localhost",
			Port:    int32(config.Port),
			NodeId:  fmt.Sprintf("localhost:%d", config.Port),
			Version: 1, // Initialize with version 1
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
			NodeId:  fmt.Sprintf("localhost:%d", config.Port),
			Ip:      "localhost",
			Port:    int32(config.Port),
			Version: 1, // Initialize with version 1
		},
		// Set replication parameters from config
		ReplicationFactorN: config.ReplicationFactorN,
		WriteQuorumW:       config.WriteQuorumW,
		ReadQuorumR:        config.ReadQuorumR,
		// Initialize connection manager
		connMgr: NewConnectionManager(),
	}
}

// StartGossip starts a background goroutine that periodically gossips with other nodes
// interval: how often to gossip
// k: the number of random nodes to gossip with in each round
func (n *Node) StartGossip(interval time.Duration, k int) {
	go n.gossipLoop(interval, k)
}

// gossipLoop runs in a separate goroutine and periodically initiates gossip with other nodes
func (n *Node) gossipLoop(interval time.Duration, k int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Node %s starting gossip loop (interval: %v, peers per round: %d)", n.meta.NodeId, interval, k)

	for {
		// Wait for the next tick
		<-ticker.C

		// Get all nodes from the ring
		allNodes := n.ring.GetAllNodes()

		// Filter out this node
		var otherNodes []rpc.NodeMeta
		for _, node := range allNodes {
			if node.NodeId != n.meta.NodeId {
				otherNodes = append(otherNodes, node)
			}
		}

		// If there are no other nodes, skip this round
		if len(otherNodes) == 0 {
			log.Printf("Node %s: No other nodes to gossip with", n.meta.NodeId)
			continue
		}

		// Determine how many nodes to select (min of k and available nodes)
		numToSelect := k
		if numToSelect > len(otherNodes) {
			numToSelect = len(otherNodes)
		}

		// Select k random nodes
		selectedIndices := make(map[int]bool)
		for len(selectedIndices) < numToSelect {
			idx := rand.Intn(len(otherNodes))
			selectedIndices[idx] = true
		}

		// Launch a goroutine for each selected node
		for idx := range selectedIndices {
			peerMeta := otherNodes[idx]
			go n.doGossipWithPeer(peerMeta)
		}
	}
}

// doGossipWithPeer initiates a gossip exchange with a specific peer node
func (n *Node) doGossipWithPeer(peerMeta rpc.NodeMeta) {
	log.Printf("Node %s: Gossiping with peer %s", n.meta.NodeId, peerMeta.NodeId)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get the peer's address
	peerAddr := fmt.Sprintf("%s:%d", peerMeta.Ip, peerMeta.Port)

	// Get a connection to the peer
	conn, err := n.connMgr.GetConnection(peerAddr)
	if err != nil {
		log.Printf("Node %s: Failed to connect to peer %s: %v", n.meta.NodeId, peerMeta.NodeId, err)
		return
	}

	// Create a client
	client := rpc.NewNodeServiceClient(conn)

	// Prepare the gossip request
	n.metaMutex.RLock()
	senderNode := n.meta
	n.metaMutex.RUnlock()

	// Get all node states from our ring
	allNodes := n.ring.GetAllNodes()
	nodeStates := make(map[string]*rpc.NodeMeta, len(allNodes))
	for _, node := range allNodes {
		// Create a copy to avoid modifying the original
		nodeCopy := node
		nodeStates[node.NodeId] = &nodeCopy
	}

	// Create the request
	request := &rpc.GossipRequest{
		SenderNode: &senderNode,
		NodeStates: nodeStates,
	}

	// Send the gossip request
	response, err := client.Gossip(ctx, request)
	if err != nil {
		log.Printf("Node %s: Gossip RPC to peer %s failed: %v", n.meta.NodeId, peerMeta.NodeId, err)
		return
	}

	// Process the response (updated node states)
	if len(response.UpdatedNodeStates) > 0 {
		log.Printf("Node %s: Received %d updated node states from peer %s",
			n.meta.NodeId, len(response.UpdatedNodeStates), peerMeta.NodeId)

		// Process the updated node states by merging them into our ring
		n.mergeGossipData(response.UpdatedNodeStates)
	} else {
		log.Printf("Node %s: No updates from peer %s", n.meta.NodeId, peerMeta.NodeId)
	}
}

// mergeGossipData merges received node states into the local ring
// It updates the ring with any new or more recent node information
func (n *Node) mergeGossipData(receivedStates map[string]*rpc.NodeMeta) {
	if len(receivedStates) == 0 {
		return
	}

	// We'll track if any changes were made to the ring
	changed := false

	for nodeID, receivedNodeMeta := range receivedStates {
		// Get existing metadata for this node ID, if any
		existingNodeMeta, exists := n.ring.GetNodeMeta(nodeID)

		if !exists {
			// This is a new node we don't know about
			log.Printf("Node %s: Adding new node %s to ring from gossip", n.meta.NodeId, nodeID)
			n.ring.AddNode(*receivedNodeMeta)
			changed = true
		} else if receivedNodeMeta.Version > existingNodeMeta.Version {
			// We have this node, but received data is newer
			log.Printf("Node %s: Updating node %s in ring (version %d -> %d)",
				n.meta.NodeId, nodeID, existingNodeMeta.Version, receivedNodeMeta.Version)

			// Remove the old node and add the updated one
			n.ring.RemoveNode(nodeID)
			n.ring.AddNode(*receivedNodeMeta)
			changed = true
		}
	}

	// If we've made changes to our ring, increment our own version
	if changed {
		n.IncrementVersion()
		log.Printf("Node %s: Updated ring based on gossip. New version: %d",
			n.meta.NodeId, n.GetCurrentVersion())
	}
}
