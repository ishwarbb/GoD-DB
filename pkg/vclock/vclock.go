// Package vclock provides vector clock functionality for causal ordering in distributed systems
package vclock

import (
	"no.cap/goddb/pkg/rpc"
)

// VectorClock represents a vector clock implementation
type VectorClock struct {
	Counters map[string]int64 // node_id -> counter
}

// New creates a new empty vector clock
func New() *VectorClock {
	return &VectorClock{
		Counters: make(map[string]int64),
	}
}

// FromProto converts from protobuf VectorClock to internal VectorClock
func FromProto(proto *rpc.VectorClock) *VectorClock {
	vc := New()
	if proto != nil {
		for nodeID, counter := range proto.Counters {
			vc.Counters[nodeID] = counter
		}
	}
	return vc
}

// ToProto converts internal VectorClock to protobuf VectorClock
func (vc *VectorClock) ToProto() *rpc.VectorClock {
	proto := &rpc.VectorClock{
		Counters: make(map[string]int64),
	}
	for nodeID, counter := range vc.Counters {
		proto.Counters[nodeID] = counter
	}
	return proto
}

// Increment increments the counter for the given node
func (vc *VectorClock) Increment(nodeID string) {
	counter := vc.Counters[nodeID]
	vc.Counters[nodeID] = counter + 1
}

// Compare compares two vector clocks and returns:
//
//	-1 if vc < other (vc happens-before other)
//	 0 if vc and other are concurrent
//	 1 if vc > other (other happens-before vc)
func (vc *VectorClock) Compare(other *VectorClock) int {
	// Check if vc < other
	lessThan := false
	// Check if other < vc
	greaterThan := false

	// Check all keys in vc
	for nodeID, counter := range vc.Counters {
		otherCounter, exists := other.Counters[nodeID]
		if !exists {
			// If other doesn't have this node, treat it as 0
			if counter > 0 {
				greaterThan = true
			}
		} else if counter < otherCounter {
			lessThan = true
		} else if counter > otherCounter {
			greaterThan = true
		}
	}

	// Check keys in other that are not in vc
	for nodeID, otherCounter := range other.Counters {
		_, exists := vc.Counters[nodeID]
		if !exists && otherCounter > 0 {
			lessThan = true
		}
	}

	// Determine relationship
	if lessThan && !greaterThan {
		return -1 // vc happens-before other
	} else if !lessThan && greaterThan {
		return 1 // other happens-before vc
	}
	return 0 // concurrent
}

// Merge combines two vector clocks, taking the maximum value for each node
func (vc *VectorClock) Merge(other *VectorClock) *VectorClock {
	result := New()

	// Copy all values from vc
	for nodeID, counter := range vc.Counters {
		result.Counters[nodeID] = counter
	}

	// Merge with other, taking max values
	for nodeID, otherCounter := range other.Counters {
		myCounter, exists := result.Counters[nodeID]
		if !exists || otherCounter > myCounter {
			result.Counters[nodeID] = otherCounter
		}
	}

	return result
}

// Clone creates a deep copy of the vector clock
func (vc *VectorClock) Clone() *VectorClock {
	clone := New()
	for nodeID, counter := range vc.Counters {
		clone.Counters[nodeID] = counter
	}
	return clone
}
