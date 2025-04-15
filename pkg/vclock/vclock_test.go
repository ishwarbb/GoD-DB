package vclock

import (
	"testing"
)

func TestVectorClockCompare(t *testing.T) {
	tests := []struct {
		name     string
		vc1      map[string]int64
		vc2      map[string]int64
		expected int
	}{
		{
			name:     "Equal",
			vc1:      map[string]int64{"node1": 1, "node2": 2},
			vc2:      map[string]int64{"node1": 1, "node2": 2},
			expected: 0, // Neither happens-before, equal
		},
		{
			name:     "Happens-before",
			vc1:      map[string]int64{"node1": 1, "node2": 1},
			vc2:      map[string]int64{"node1": 2, "node2": 2},
			expected: -1, // vc1 happens-before vc2
		},
		{
			name:     "Happens-after",
			vc1:      map[string]int64{"node1": 3, "node2": 3},
			vc2:      map[string]int64{"node1": 1, "node2": 2},
			expected: 1, // vc1 happens-after vc2
		},
		{
			name:     "Concurrent - Some greater, some less",
			vc1:      map[string]int64{"node1": 3, "node2": 1},
			vc2:      map[string]int64{"node1": 1, "node2": 3},
			expected: 0, // Concurrent modifications
		},
		{
			name:     "Concurrent - Extra keys",
			vc1:      map[string]int64{"node1": 1, "node2": 2, "node3": 1},
			vc2:      map[string]int64{"node1": 1, "node2": 2, "node4": 1},
			expected: 0, // Concurrent modifications
		},
		{
			name:     "One empty",
			vc1:      map[string]int64{},
			vc2:      map[string]int64{"node1": 1, "node2": 2},
			expected: -1, // Empty happens-before non-empty
		},
		{
			name:     "Both empty",
			vc1:      map[string]int64{},
			vc2:      map[string]int64{},
			expected: 0, // Both empty, equal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc1 := New()
			vc2 := New()

			// Initialize vector clocks
			for k, v := range tt.vc1 {
				vc1.Counters[k] = v
			}
			for k, v := range tt.vc2 {
				vc2.Counters[k] = v
			}

			result := vc1.Compare(vc2)
			if result != tt.expected {
				t.Errorf("Compare() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestVectorClockMerge(t *testing.T) {
	tests := []struct {
		name     string
		vc1      map[string]int64
		vc2      map[string]int64
		expected map[string]int64
	}{
		{
			name:     "Basic merge",
			vc1:      map[string]int64{"node1": 1, "node2": 2},
			vc2:      map[string]int64{"node1": 3, "node3": 1},
			expected: map[string]int64{"node1": 3, "node2": 2, "node3": 1},
		},
		{
			name:     "Empty with non-empty",
			vc1:      map[string]int64{},
			vc2:      map[string]int64{"node1": 1, "node2": 2},
			expected: map[string]int64{"node1": 1, "node2": 2},
		},
		{
			name:     "Both empty",
			vc1:      map[string]int64{},
			vc2:      map[string]int64{},
			expected: map[string]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc1 := New()
			vc2 := New()

			// Initialize vector clocks
			for k, v := range tt.vc1 {
				vc1.Counters[k] = v
			}
			for k, v := range tt.vc2 {
				vc2.Counters[k] = v
			}

			merged := vc1.Merge(vc2)

			// Verify the merged result
			if len(merged.Counters) != len(tt.expected) {
				t.Errorf("Merge() resulted in %d keys, want %d", len(merged.Counters), len(tt.expected))
			}

			for k, expectedVal := range tt.expected {
				if mergedVal, ok := merged.Counters[k]; !ok || mergedVal != expectedVal {
					t.Errorf("Merge() for key %s = %v, want %v", k, mergedVal, expectedVal)
				}
			}
		})
	}
}

func TestVectorClockIncrement(t *testing.T) {
	vc := New()

	// First increment
	vc.Increment("node1")
	if vc.Counters["node1"] != 1 {
		t.Errorf("After first increment, counter = %d, want 1", vc.Counters["node1"])
	}

	// Second increment
	vc.Increment("node1")
	if vc.Counters["node1"] != 2 {
		t.Errorf("After second increment, counter = %d, want 2", vc.Counters["node1"])
	}

	// Increment another node
	vc.Increment("node2")
	if vc.Counters["node2"] != 1 {
		t.Errorf("New node counter = %d, want 1", vc.Counters["node2"])
	}
}

func TestVectorClockProtoConversion(t *testing.T) {
	// Create a vector clock
	vc := New()
	vc.Counters["node1"] = 3
	vc.Counters["node2"] = 5

	// Convert to proto
	proto := vc.ToProto()

	// Verify proto content
	if len(proto.Counters) != 2 {
		t.Errorf("Proto conversion resulted in %d keys, want 2", len(proto.Counters))
	}
	if proto.Counters["node1"] != 3 {
		t.Errorf("node1 counter = %d, want 3", proto.Counters["node1"])
	}
	if proto.Counters["node2"] != 5 {
		t.Errorf("node2 counter = %d, want 5", proto.Counters["node2"])
	}

	// Convert back to internal
	converted := FromProto(proto)

	// Verify round-trip conversion
	if len(converted.Counters) != 2 {
		t.Errorf("Round-trip conversion resulted in %d keys, want 2", len(converted.Counters))
	}
	if converted.Counters["node1"] != 3 {
		t.Errorf("node1 counter = %d, want 3", converted.Counters["node1"])
	}
	if converted.Counters["node2"] != 5 {
		t.Errorf("node2 counter = %d, want 5", converted.Counters["node2"])
	}
}

func TestFromProtoWithNil(t *testing.T) {
	// Test that FromProto handles nil input gracefully
	vc := FromProto(nil)
	if vc == nil {
		t.Errorf("FromProto(nil) returned nil, expected empty VectorClock")
	}
	if len(vc.Counters) != 0 {
		t.Errorf("FromProto(nil) returned non-empty counters, expected empty map")
	}
}
