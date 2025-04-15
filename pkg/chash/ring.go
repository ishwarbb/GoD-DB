package chash

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"no.cap/goddb/pkg/rpc"
)

type Ring struct {
	mu             sync.RWMutex
	hashes         []string                // Sorted list of all hashes (real and virtual)
	nodeMetas      map[string]rpc.NodeMeta // Map from hash to node metadata
	nodeToVirtuals map[string][]string     // Map from node ID to its virtual node hashes
}

func NewRing() *Ring {
	return &Ring{
		hashes:         []string{},
		nodeMetas:      make(map[string]rpc.NodeMeta),
		nodeToVirtuals: make(map[string][]string),
	}
}

func GenerateRandomString() string {
	// Generate a random string to use as a node ID
	// This is a placeholder; in a real implementation, you would generate a unique ID
	return fmt.Sprintf("node-%d", rand.Int())
}

// AddNode adds a node to the ring with the specified number of virtual nodes
func (r *Ring) AddNode(node rpc.NodeMeta, virtualNodesCount int) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate a hash for the real node
	realNodeHash := hashString(node.NodeId)

	// Create a slice to store all hashes for this node (real + virtual)
	nodeHashes := []string{realNodeHash}

	// Create virtual nodes
	for i := 0; i < virtualNodesCount; i++ {
		virtualNodeID := fmt.Sprintf("%s-vnode-%d", node.NodeId, i)
		virtualNodeHash := hashString(virtualNodeID)
		nodeHashes = append(nodeHashes, virtualNodeHash)

		// Store the node metadata for the virtual node hash
		r.nodeMetas[virtualNodeHash] = node
	}

	// Add the real node metadata
	r.nodeMetas[realNodeHash] = node

	// Store the mapping from node ID to its hashes (for easy removal later)
	r.nodeToVirtuals[node.NodeId] = nodeHashes

	// Add all hashes to the ring and sort
	r.hashes = append(r.hashes, nodeHashes...)
	sort.Strings(r.hashes)

	return realNodeHash
}

func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get all hashes (real + virtual) for this node
	nodeHashes, exists := r.nodeToVirtuals[nodeID]
	if !exists {
		return // Node not found
	}

	// Remove node metadata for all hashes
	for _, hash := range nodeHashes {
		delete(r.nodeMetas, hash)
	}

	// Remove node from nodeToVirtuals mapping
	delete(r.nodeToVirtuals, nodeID)

	// Rebuild the sorted hashes list without the removed node's hashes
	newHashes := make([]string, 0, len(r.hashes)-len(nodeHashes))
	for _, hash := range r.hashes {
		// Only keep hashes that are not part of the removed node
		isRemoved := false
		for _, removedHash := range nodeHashes {
			if hash == removedHash {
				isRemoved = true
				break
			}
		}

		if !isRemoved {
			newHashes = append(newHashes, hash)
		}
	}

	r.hashes = newHashes
}

func (r *Ring) GetNode(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.hashes) == 0 {
		return "", errors.New("ring is empty")
	}

	keyHash := hashString(key)
	index := sort.SearchStrings(r.hashes, keyHash)
	if index == len(r.hashes) {
		index = 0
	}

	fmt.Println("index", index)

	nodeID := r.nodeMetas[r.hashes[index]].NodeId
	return nodeID, nil
}

func hashString(s string) string {
	hasher := md5.New()
	hasher.Write([]byte(s))
	return hex.EncodeToString(hasher.Sum(nil))
}

// TODO: do multiple hash functions?

func (r *Ring) GetN(key string, n int) ([]string, error) {
	// Hashes the key, finds the primary node using `GetNode`,
	// and then walks the ring clockwise to find the next `n-1` unique node IDs.
	//  Returns the list of `n` node IDs (hashes).
	// Handle cases where `n` is larger than the number of nodes in the ring.
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.hashes) == 0 {
		return nil, errors.New("ring is empty")
	}

	keyHash := hashString(key)
	index := sort.SearchStrings(r.hashes, keyHash)
	if index == len(r.hashes) {
		index = 0
	}

	// Create a slice to hold the node IDs
	nodeIDs := make([]string, 0, n)
	visited := make(map[string]bool)
	// Start from the primary node and walk clockwise

	for i := 0; i < len(r.hashes) && len(nodeIDs) < n; i++ {
		hash := r.hashes[(index+i)%len(r.hashes)]
		node := r.nodeMetas[hash]
		if !visited[node.NodeId] {
			nodeIDs = append(nodeIDs, node.NodeId)
			visited[node.NodeId] = true
		}
	}

	// If we have fewer unique nodes than requested, return an error
	if len(nodeIDs) < n {
		return nil, fmt.Errorf("not enough unique nodes in the ring: requested %d, found %d", n, len(nodeIDs))
	}

	return nodeIDs, nil
}

func (r *Ring) GetNodeMeta(nodeID string) (rpc.NodeMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, node := range r.nodeMetas {
		if node.NodeId == nodeID {
			return node, true
		}
	}
	return rpc.NodeMeta{}, false
}

func (r *Ring) GetAllNodes() []rpc.NodeMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Use a map to track unique nodes by nodeID
	uniqueNodes := make(map[string]rpc.NodeMeta)

	// Add each node to the map using nodeID as key to ensure uniqueness
	for _, node := range r.nodeMetas {
		uniqueNodes[node.NodeId] = node
	}

	// Convert the map values to a slice
	nodes := make([]rpc.NodeMeta, 0, len(uniqueNodes))
	for _, node := range uniqueNodes {
		nodes = append(nodes, node)
	}

	return nodes
}
