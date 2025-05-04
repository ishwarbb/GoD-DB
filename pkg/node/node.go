package node

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
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
	// Number of virtual nodes per physical node
	VirtualNodesCount int
	// Redis discovery service configuration
	DiscoveryRedisAddr string
}

// DefaultNodeConfig returns a NodeConfig with sensible defaults
func DefaultNodeConfig(port, redisPort int) NodeConfig {
	return NodeConfig{
		Port:               port,
		RedisPort:          redisPort,
		ReplicationFactorN: 3,
		WriteQuorumW:       2,
		ReadQuorumR:        2,
		VirtualNodesCount:  10,               // Default to 10 virtual nodes per physical node
		DiscoveryRedisAddr: "localhost:6379", // Default discovery Redis address
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
	// Virtual nodes configuration
	VirtualNodesCount int
	// Connection manager for outgoing RPC calls
	connMgr *ConnectionManager
	// Mutex to protect meta updates
	metaMutex sync.RWMutex
	// Discovery service configuration
	discoveryRedisAddr string
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

	if err != nil {
		panic(err) // TODO: deal with this better
	}

	node := &Node{
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
		// Set virtual nodes count
		VirtualNodesCount: config.VirtualNodesCount,
		// Initialize connection manager
		connMgr:            NewConnectionManager(),
		discoveryRedisAddr: config.DiscoveryRedisAddr,
	}

	// Register with discovery service and initialize ring with discovered nodes
	node.registerWithDiscovery()

	return node
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

	// Create a context with a more lenient timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			n.ring.AddNode(*receivedNodeMeta, n.VirtualNodesCount)
			changed = true
		} else if receivedNodeMeta.Version > existingNodeMeta.Version {
			// We have this node, but received data is newer
			log.Printf("Node %s: Updating node %s in ring (version %d -> %d)",
				n.meta.NodeId, nodeID, existingNodeMeta.Version, receivedNodeMeta.Version)

			// Remove the old node and add the updated one
			n.ring.RemoveNode(nodeID)
			n.ring.AddNode(*receivedNodeMeta, n.VirtualNodesCount)
			changed = true
		}
	}

	// If we've made changes to our ring, increment our own version
	if changed {
		n.IncrementVersion()
		log.Printf("Node %s: Updated ring based on gossip. New version: %d",
			n.meta.NodeId, n.GetCurrentVersion())
	}

	log.Printf("Node %s: Ring after gossip: %v", n.meta.NodeId, n.ring)
}

// registerWithDiscovery registers this node with the discovery service
// and initializes the ring with all nodes from the discovery service
func (n *Node) registerWithDiscovery() {
	// Connect to the discovery Redis server
	var redisAddr string
	if strings.Contains(n.discoveryRedisAddr, ":") {
		// Address already has host:port format
		redisAddr = n.discoveryRedisAddr
	} else {
		// Address only contains port, prepend localhost
		redisAddr = fmt.Sprintf("localhost:%s", n.discoveryRedisAddr)
	}
	
	// Add retry mechanism for discovery connection
	maxRetries := 5
	retryDelay := 2 * time.Second
	var discoveryStore *storage.Store
	var err error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Connecting to discovery Redis at %s (attempt %d/%d)", redisAddr, attempt, maxRetries)
		discoveryStore, err = storage.NewStore(redisAddr)
		if err == nil {
			log.Printf("Successfully connected to discovery Redis on attempt %d", attempt)
			break
		}
		
		log.Printf("Failed to connect to discovery Redis at %s: %v", redisAddr, err)
		if attempt < maxRetries {
			log.Printf("Retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
			// Increase delay for next attempt
			retryDelay *= 2
		}
	}
	
	if err != nil {
		log.Printf("All connection attempts to discovery Redis failed")
		log.Printf("Continuing with local node only")
		// Add self to ring as fallback with virtual nodes
		n.ring.AddNode(n.meta, n.VirtualNodesCount)
		return
	}

	// Register this node in the discovery service
	nodeKey := fmt.Sprintf("node:%s", n.meta.NodeId)
	nodeData, err := serializeNodeMeta(n.meta)
	if err != nil {
		log.Printf("Failed to serialize node metadata: %v", err)
		n.ring.AddNode(n.meta, n.VirtualNodesCount)
		return
	}

	// Store node information in discovery Redis
	err = discoveryStore.Put(context.Background(), nodeKey, nodeData, time.Now())
	if err != nil {
		log.Printf("Failed to register node in discovery service: %v", err)
		n.ring.AddNode(n.meta, n.VirtualNodesCount)
		return
	}
	log.Printf("Node %s registered with discovery service", n.meta.NodeId)

	// Get all registered nodes using a Redis pattern scan
	nodeKeys, err := scanKeys(discoveryStore, "node:*")
	log.Printf("nodeKeys: %s", nodeKeys)

	if err != nil {
		log.Printf("Failed to get node list from discovery service: %v", err)
		n.ring.AddNode(n.meta, n.VirtualNodesCount)
		return
	}

	// Add all discovered nodes to the ring
	for _, key := range nodeKeys {
		data, _, err := discoveryStore.Get(context.Background(), key)
		if err != nil {
			log.Printf("Failed to get node data for %s: %v", key, err)
			continue
		}

		nodeMeta, err := deserializeNodeMeta(data)
		if err != nil {
			log.Printf("Failed to deserialize node data for %s: %v", key, err)
			continue
		}

		// Add the node to our ring with virtual nodes
		n.ring.AddNode(nodeMeta, n.VirtualNodesCount)
		log.Printf("Added node %s from discovery service", nodeMeta.NodeId)
	}

	log.Printf("Node %s initialized with %d nodes from discovery service",
		n.meta.NodeId, len(nodeKeys))
}

// serializeNodeMeta serializes an rpc.NodeMeta to a byte slice
func serializeNodeMeta(meta rpc.NodeMeta) ([]byte, error) {
	return json.Marshal(meta)
}

// deserializeNodeMeta deserializes a byte slice back to an rpc.NodeMeta
func deserializeNodeMeta(data []byte) (rpc.NodeMeta, error) {
	var meta rpc.NodeMeta
	err := json.Unmarshal(data, &meta)
	return meta, err
}

// scanKeys scans Redis for keys matching the provided pattern
func scanKeys(store *storage.Store, pattern string) ([]string, error) {
	// Use Redis client directly for pattern matching since Store doesn't expose Scan
	ctx := context.Background()
	cmd := store.Client().Keys(ctx, pattern)
	return cmd.Result()
}

// StartHintProcessor starts a background goroutine that periodically checks for and processes hints
func (n *Node) StartHintProcessor(interval time.Duration) {
	go n.hintProcessorLoop(interval)
}

// hintProcessorLoop runs in a separate goroutine and periodically checks for and processes hints
func (n *Node) hintProcessorLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Node %s starting hint processor loop (interval: %v)", n.meta.NodeId, interval)

	for {
		// Wait for the next tick
		<-ticker.C

		// Get all nodes from the ring
		allNodes := n.ring.GetAllNodes()

		// Process hints for each node
		for _, nodeMeta := range allNodes {
			// Skip self
			if nodeMeta.NodeId == n.meta.NodeId {
				continue
			}

			// Process hints for this node
			go n.processHintsForNode(nodeMeta)
		}
	}
}

// processHintsForNode processes all hints stored for a specific node
func (n *Node) processHintsForNode(nodeMeta rpc.NodeMeta) {
	// Get all hints for this node
	hints, err := n.store.GetHintsForNode(context.Background(), nodeMeta.NodeId)
	if err != nil {
		log.Printf("Node %s: Failed to get hints for node %s: %v", n.meta.NodeId, nodeMeta.NodeId, err)
		return
	}

	// If there are no hints, skip
	if len(hints) == 0 {
		return
	}

	log.Printf("Node %s: Found %d hints to process for node %s", n.meta.NodeId, len(hints), nodeMeta.NodeId)

	// Get a connection to the node
	nodeAddr := fmt.Sprintf("%s:%d", nodeMeta.Ip, nodeMeta.Port)
	conn, err := n.connMgr.GetConnection(nodeAddr)
	if err != nil {
		log.Printf("Node %s: Failed to connect to node %s: %v", n.meta.NodeId, nodeMeta.NodeId, err)
		return
	}

	// Create client
	client := rpc.NewNodeServiceClient(conn)

	// Process each hint
	for _, hint := range hints {
		// Create a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Create the request
		req := &rpc.ReplayHintRequest{
			Hint: hint,
		}

		// Make the RPC call
		resp, err := client.ReplayHint(ctx, req)
		if err != nil {
			log.Printf("Node %s: Failed to replay hint for key %s to node %s: %v", 
				n.meta.NodeId, hint.Key, hint.TargetNodeId, err)
			continue
		}

		// Check the response
		if resp.Status != rpc.StatusCode_OK {
			log.Printf("Node %s: Failed to replay hint for key %s to node %s: %s", 
				n.meta.NodeId, hint.Key, hint.TargetNodeId, resp.Message)
			continue
		}

		// Hint was successfully processed, delete it
		err = n.store.DeleteHint(context.Background(), hint.TargetNodeId, hint.Key)
		if err != nil {
			log.Printf("Node %s: Failed to delete hint for key %s to node %s: %v", 
				n.meta.NodeId, hint.Key, hint.TargetNodeId, err)
			continue
		}

		log.Printf("Node %s: Successfully replayed hint for key %s to node %s", 
			n.meta.NodeId, hint.Key, hint.TargetNodeId)
	}
}
