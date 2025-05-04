# Hinted Handoff Feature

## Overview

Hinted Handoff is a technique used in distributed databases to handle temporary node failures. When a node that should receive a replica is unavailable, the system stores a "hint" for that data on another node. Once the failed node comes back online, the hints are "handed off" to the recovered node, ensuring data consistency despite temporary failures.

## Implementation Details

Our implementation of hinted handoff includes the following components:

1. **Hint Storage**: When a write operation can't reach a replica node, the coordinator stores a hint in its local Redis instance with the following information:
   - The original key
   - The value
   - The timestamp of the original write
   - The target node ID that should eventually receive this data
   - A TTL (time-to-live) for the hint (default: 24 hours)

2. **Hint Processing**: Each node runs a background process that periodically checks for hints that should be delivered. When a previously unavailable node becomes available again, hints are delivered to it.

3. **Hint Replay**: When a hint is delivered, the target node validates that the data belongs to it and stores it normally.

4. **Hint Cleanup**: Successfully delivered hints are removed from the hint store. Hints that expire (based on their TTL) are automatically cleaned up by Redis.

## Protocol Buffer Messages

The implementation uses the following protocol buffer messages:

```protobuf
message HintedHandoffEntry {
    string key = 1;
    bytes value = 2;
    google.protobuf.Timestamp timestamp = 3;
    string target_node_id = 4;
    google.protobuf.Timestamp hint_created_at = 5;
}

message ReplayHintRequest {
    HintedHandoffEntry hint = 1;
}

message ReplayHintResponse {
    StatusCode status = 1;
    string message = 2;
}
```

## Configuration

The hinted handoff feature can be configured with the following parameters:

- `enablehints`: Enable or disable hinted handoff functionality (boolean)
- `hintinterval`: The interval at which to check for and process hints (duration, e.g., "30s")
- `hintttl`: The time-to-live for stored hints (duration, e.g., "24h")

Example:
```
./bin/worker -port 8081 -enablehints true -hintinterval 30s
```

## Testing

You can test the hinted handoff functionality using the dedicated test script:

```
./run_handoff_test.sh
```

This script:
1. Starts three worker nodes
2. Writes initial data to the cluster
3. Simulates a node failure by killing one worker
4. Writes more data (which should generate hints for the failed node)
5. Restarts the failed node
6. Verifies that the hinted data was properly delivered

## Benefits

The hinted handoff feature provides the following benefits:

1. **Improved Availability**: Write operations can succeed even when some replica nodes are down
2. **Eventual Consistency**: Ensures data is eventually consistent across all nodes
3. **Self-Healing**: The system automatically repairs itself when nodes recover
4. **Reduced Manual Intervention**: Reduces the need for manual data repair after node failures

## Limitations

- Hints have a finite TTL, so if a node remains down for longer than the TTL, data might be lost
- Hint storage consumes additional space on the nodes that store the hints
- There is a small performance overhead for processing and delivering hints 