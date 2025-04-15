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
	mu        sync.RWMutex
	hashes    []string
	nodeMetas map[string]rpc.NodeMeta
}

func NewRing() *Ring {
	return &Ring{
		hashes:    []string{},
		nodeMetas: make(map[string]rpc.NodeMeta),
	}
}

func GenerateRandomString() string {
	// Generate a random string to use as a node ID
	// This is a placeholder; in a real implementation, you would generate a unique ID
	return fmt.Sprintf("node-%d", rand.Int())
}

func (r *Ring) AddNode(node rpc.NodeMeta) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Pass a random string to hashString to ensure a unique hash for each node
	hash := hashString(node.NodeId)
	r.hashes = append(r.hashes, hash)
	sort.Strings(r.hashes)
	r.nodeMetas[hash] = node
	return hash
}

func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	//look up the nodeMetas for this nide ID and find the corredponding hash
	var unwanted_hashes []string
	for k, v := range r.nodeMetas {
		if v.NodeId == nodeID {
			unwanted_hashes = append(unwanted_hashes, k)
		}
	}

	for _, v := range unwanted_hashes {
		delete(r.nodeMetas, v)
	}
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
		index = (index + 1) % len(r.hashes)
		hash := r.hashes[index]
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

	nodes := make([]rpc.NodeMeta, 0, len(r.nodeMetas))
	for _, node := range r.nodeMetas {
		nodes = append(nodes, node)
	}
	return nodes
}
