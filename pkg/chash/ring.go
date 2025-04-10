package chash

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
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

func (r *Ring) AddNode(node rpc.NodeMeta) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeID := fmt.Sprintf("%s:%d", node.Ip, node.Port)
	hash := hashString(nodeID)
	r.hashes = append(r.hashes, hash)
	sort.Strings(r.hashes)
	r.nodeMetas[hash] = node
	return hash
}

func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := hashString(nodeID)
	for i, h := range r.hashes {
		if h == hash {
			r.hashes = append(r.hashes[:i], r.hashes[i+1:]...)
			break
		}
	}
	delete(r.nodeMetas, hash)
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
	return r.hashes[index], nil
}

func hashString(s string) string {
	hasher := md5.New()
	hasher.Write([]byte(s))
	return hex.EncodeToString(hasher.Sum(nil))
}

// TODO: do multiple hash functions?
