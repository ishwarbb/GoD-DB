package chash

import (
	"fmt"
	"testing"

	"no.cap/goddb/pkg/rpc"
)

func TestVirtualNodes(t *testing.T) {
	// Create a new ring
	ring := NewRing()

	// Create a test node
	node := rpc.NodeMeta{
		NodeId: "node1",
		Ip:     "localhost",
		Port:   5000,
	}

	// Add the node with 10 virtual nodes
	virtualNodesCount := 10
	ring.AddNode(node, virtualNodesCount)

	// Verify that the ring has 11 hashes (1 real + 10 virtual)
	if len(ring.hashes) != virtualNodesCount+1 {
		t.Errorf("Expected %d hashes, got %d", virtualNodesCount+1, len(ring.hashes))
	}

	// Verify that the nodeToVirtuals map has 1 entry
	if len(ring.nodeToVirtuals) != 1 {
		t.Errorf("Expected 1 entry in nodeToVirtuals, got %d", len(ring.nodeToVirtuals))
	}

	// Verify that the node is properly stored in the nodeToVirtuals map
	virtualHashes, exists := ring.nodeToVirtuals[node.NodeId]
	if !exists {
		t.Errorf("Node %s not found in nodeToVirtuals map", node.NodeId)
	}

	// Verify that the node has the correct number of virtual nodes
	if len(virtualHashes) != virtualNodesCount+1 {
		t.Errorf("Expected %d virtual hashes for node, got %d", virtualNodesCount+1, len(virtualHashes))
	}

	// Add another node with a different number of virtual nodes
	node2 := rpc.NodeMeta{
		NodeId: "node2",
		Ip:     "localhost",
		Port:   5001,
	}
	virtualNodesCount2 := 5
	ring.AddNode(node2, virtualNodesCount2)

	// Verify that the ring now has 11 + 6 = 17 hashes
	if len(ring.hashes) != (virtualNodesCount+1)+(virtualNodesCount2+1) {
		t.Errorf("Expected %d hashes, got %d", (virtualNodesCount+1)+(virtualNodesCount2+1), len(ring.hashes))
	}

	// Test key distribution to ensure virtual nodes are working
	// We'll hash a bunch of keys and count how many land on each node
	keyCount := 1000
	nodeCounts := make(map[string]int)

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		nodeID, err := ring.GetNode(key)
		if err != nil {
			t.Fatalf("Error getting node for key %s: %v", key, err)
		}
		nodeCounts[nodeID]++
	}

	// With virtual nodes, we should have a more even distribution
	// between the two nodes compared to without virtual nodes
	t.Logf("Node distribution with virtual nodes: %v", nodeCounts)

	// Each node should have at least some keys, but we won't be too strict
	// on the exact distribution since it depends on the hash function
	for nodeID, count := range nodeCounts {
		if count == 0 {
			t.Errorf("Node %s has no keys assigned to it", nodeID)
		}
	}

	// Test removing a node
	ring.RemoveNode(node.NodeId)

	// Verify that the ring now has only the second node's hashes
	if len(ring.hashes) != virtualNodesCount2+1 {
		t.Errorf("Expected %d hashes after removal, got %d", virtualNodesCount2+1, len(ring.hashes))
	}

	// Verify that the nodeToVirtuals map no longer has the removed node
	if _, exists := ring.nodeToVirtuals[node.NodeId]; exists {
		t.Errorf("Node %s should have been removed from nodeToVirtuals map", node.NodeId)
	}

	// Verify that all keys now go to the remaining node
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		nodeID, err := ring.GetNode(key)
		if err != nil {
			t.Fatalf("Error getting node for key %s: %v", key, err)
		}
		if nodeID != node2.NodeId {
			t.Errorf("Key %s was assigned to node %s, but expected node %s", key, nodeID, node2.NodeId)
		}
	}
}

// TestCompareDistributionWithWithoutVirtualNodes compares key distribution
// between a ring with virtual nodes and one without
func TestCompareDistributionWithWithoutVirtualNodes(t *testing.T) {
	// Create two identical nodes
	node1 := rpc.NodeMeta{
		NodeId: "node1",
		Ip:     "localhost",
		Port:   5000,
	}
	node2 := rpc.NodeMeta{
		NodeId: "node2",
		Ip:     "localhost",
		Port:   5001,
	}

	// Create a ring without virtual nodes
	ringWithoutVirtual := NewRing()
	ringWithoutVirtual.AddNode(node1, 0) // No virtual nodes
	ringWithoutVirtual.AddNode(node2, 0) // No virtual nodes

	// Create a ring with virtual nodes
	ringWithVirtual := NewRing()
	ringWithVirtual.AddNode(node1, 50) // 50 virtual nodes per physical node
	ringWithVirtual.AddNode(node2, 50) // 50 virtual nodes per physical node

	// Test distribution of 10000 keys
	keyCount := 10000

	// Count keys for the ring without virtual nodes
	nodeCountsWithout := make(map[string]int)
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		nodeID, err := ringWithoutVirtual.GetNode(key)
		if err != nil {
			t.Fatalf("Error getting node for key %s: %v", key, err)
		}
		nodeCountsWithout[nodeID]++
	}

	// Count keys for the ring with virtual nodes
	nodeCountsWith := make(map[string]int)
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		nodeID, err := ringWithVirtual.GetNode(key)
		if err != nil {
			t.Fatalf("Error getting node for key %s: %v", key, err)
		}
		nodeCountsWith[nodeID]++
	}

	t.Logf("Distribution without virtual nodes: %v", nodeCountsWithout)
	t.Logf("Distribution with virtual nodes: %v", nodeCountsWith)

	// Calculate the deviation from perfect distribution (50%)
	perfectShare := keyCount / 2
	deviationWithout := 0
	deviationWith := 0

	for _, count := range nodeCountsWithout {
		deviation := count - perfectShare
		if deviation < 0 {
			deviation = -deviation
		}
		deviationWithout += deviation
	}

	for _, count := range nodeCountsWith {
		deviation := count - perfectShare
		if deviation < 0 {
			deviation = -deviation
		}
		deviationWith += deviation
	}

	t.Logf("Deviation from perfect distribution without virtual nodes: %d", deviationWithout)
	t.Logf("Deviation from perfect distribution with virtual nodes: %d", deviationWith)

	// The deviation with virtual nodes should be less than the deviation without
	// This shows that virtual nodes improve distribution
	if deviationWith >= deviationWithout {
		t.Errorf("Expected virtual nodes to improve distribution, but deviation with virtual nodes (%d) is not less than without (%d)",
			deviationWith, deviationWithout)
	}
}
