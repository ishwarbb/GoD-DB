package client

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
	"no.cap/goddb/pkg/emitter"
	"no.cap/goddb/pkg/events"
)

type NodeCache struct {
	mu       sync.Mutex
	nodes    map[string]*rpc.NodeMeta
	keyNodes map[string]string
	ttl      time.Duration
}

type ConnectionManager struct {
	mu          sync.Mutex
	connections map[string]*grpc.ClientConn
}

type Client struct {
	conn               *grpc.ClientConn
	rpc                rpc.NodeServiceClient
	nodeCache          *NodeCache
	seedNodes          []string
	replicationFactorN int
	readQuorumR        int
	writeQuorumW       int
	connMgr            *ConnectionManager
	emitter            emitter.EventEmitter
	clientId           string
}

func NewNodeCache(ttl time.Duration) *NodeCache {
	return &NodeCache{
		nodes:    make(map[string]*rpc.NodeMeta),
		keyNodes: make(map[string]string),
		ttl:      ttl,
	}
}

func NewClient(addr string, seedNodes []string, replicationFactorN, readQuorumR, writeQuorumW int, eventEmitter emitter.EventEmitter) (*Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("did not connect: %v", err)
	}

	c := rpc.NewNodeServiceClient(conn)
	node_cache := NewNodeCache(5 * time.Minute)
	conn_mgr := &ConnectionManager{
		connections: make(map[string]*grpc.ClientConn),
	}
	// Initialize the connection manager with the seed nodes
	for _, seedNode := range seedNodes {
		conn, err := grpc.Dial(seedNode, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to seed node %s: %v", seedNode, err)
		}
		conn_mgr.connections[seedNode] = conn
	}

	// Generate a simple client ID (replace with UUID later if needed)
	clientId := fmt.Sprintf("client-%s", strings.ReplaceAll(addr, ":", "-"))

	// Ensure emitter is not nil, default to NullEmitter if it is
	if eventEmitter == nil {
		log.Println("Warning: No event emitter provided to client, using NullEmitter.")
		eventEmitter = emitter.NewNullEmitter()
	}

	return &Client{
		conn:               conn,
		rpc:                c,
		nodeCache:          node_cache,
		seedNodes:          append([]string{addr}, seedNodes...),
		replicationFactorN: replicationFactorN,
		readQuorumR:        readQuorumR,
		writeQuorumW:       writeQuorumW,
		connMgr:            conn_mgr,
		emitter:            eventEmitter,
		clientId:           clientId,
	}, nil
}

func (c *Client) Close() {
	if c.emitter != nil {
		err := c.emitter.Close()
		if err != nil {
			log.Printf("Error closing event emitter: %v", err)
		}
	}
	// Close connections managed by ConnectionManager
	c.connMgr.mu.Lock()
	for nodeAddr, conn := range c.connMgr.connections {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection to %s: %v", nodeAddr, err)
		}
	}
	c.connMgr.connections = make(map[string]*grpc.ClientConn)
	c.connMgr.mu.Unlock()

	// Close the initial connection (if different from managed ones, though likely redundant now)
	if c.conn != nil {
		c.conn.Close()
	}
}

// *   Modify `Get(ctx context.Context, key string)`:
// *   Use `resolveCoordinator` to find the primary node for the key.
// *   Loop with retries:
// 	*   Get gRPC client for the target node (start with primary).
// 	*   Call the `Get` RPC.
// 	*   Handle response:
// 		*   `OK`, `NOT_FOUND`: Return result.
// 		*   `WRONG_NODE`: Update cache with `correct_node`. Set target node to `correct_node` for next retry.
// 		*   `ERROR`/Connection Error: Log error. Pick the *next* node from the preference list (if fetched) as the target for the next retry (simple failover for now).
// 	*   Decrement retry count.
// *   Return error if retries exhausted.

func (c *Client) Get(ctx context.Context, key string) ([]byte, time.Time, error) {
	// Get the preference list for this key
	preferenceList, err := c.fetchPreferenceList(ctx, key)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("could not get preference list: %v", err)
	}

	// Check if we have enough nodes for read quorum
	if len(preferenceList) < c.readQuorumR {
		return nil, time.Time{}, fmt.Errorf("insufficient nodes for read quorum (need %d, have %d)",
			c.readQuorumR, len(preferenceList))
	}

	// Create a channel for results
	resultsChan := make(chan *rpc.GetResponse, len(preferenceList))

	// Use a WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Query up to N nodes in parallel (preferenceList might contain fewer than N)
	targetNodes := preferenceList
	if len(preferenceList) > c.replicationFactorN {
		targetNodes = preferenceList[:c.replicationFactorN]
	}

	// Launch a goroutine for each target node
	for _, nodeMeta := range targetNodes {
		wg.Add(1)

		go func(node *rpc.NodeMeta) {
			defer wg.Done()

			// Get gRPC client for this node
			nodeAddr := fmt.Sprintf("%s:%d", node.Ip, node.Port)
			client, err := c.getNodeClient(ctx, nodeAddr)
			if err != nil {
				log.Printf("Failed to get client for node %s: %v", node.NodeId, err)
				resultsChan <- nil // Signal failure
				return
			}

			// Set a timeout for this read operation
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Call the Get RPC
			resp, err := client.Get(readCtx, &rpc.GetRequest{Key: key})
			if err != nil {
				log.Printf("Failed to get key '%s' from node %s: %v", key, node.NodeId, err)
				resultsChan <- nil // Signal failure
				return
			}

			// Send response to the channel
			resultsChan <- resp
		}(nodeMeta)
	}

	// Create a goroutine to wait for all reads and close the channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect valid responses
	validResponses := make([]*rpc.GetResponse, 0, len(targetNodes))

	for resp := range resultsChan {
		if resp != nil && (resp.Status == rpc.StatusCode_OK || resp.Status == rpc.StatusCode_NOT_FOUND) {
			validResponses = append(validResponses, resp)
		}
	}

	// Check if we have enough responses for read quorum
	if len(validResponses) < c.readQuorumR {
		return nil, time.Time{}, fmt.Errorf("read quorum not achieved (need %d, got %d valid responses)",
			c.readQuorumR, len(validResponses))
	}

	// Find the latest value based on timestamp
	var latestResponse *rpc.GetResponse

	for _, resp := range validResponses {
		if resp.Status == rpc.StatusCode_OK {
			if latestResponse == nil ||
				resp.Timestamp.AsTime().After(latestResponse.Timestamp.AsTime()) {
				latestResponse = resp
			}
		}
	}

	// If all responses were NOT_FOUND or we didn't find any OK responses
	if latestResponse == nil {
		// TODO: Emit QUORUM_RESULT here as well, indicating failure to find OK?
		return nil, time.Time{}, fmt.Errorf("key not found or no OK responses meeting quorum")
	}

	// Emit Quorum Result (Success case)
	c.emitQuorumResult(key, events.QuorumRead, true, validResponses)

	// Perform read repair for nodes with stale or missing data
	c.performReadRepair(ctx, key, latestResponse, validResponses)

	// Return the latest value
	return latestResponse.Value, latestResponse.Timestamp.AsTime(), nil
}

// Helper to map rpc.StatusCode to events.RpcStatus
func mapRpcStatus(s rpc.StatusCode) events.RpcStatus {
	switch s {
	case rpc.StatusCode_OK:
		return events.StatusOk
	case rpc.StatusCode_NOT_FOUND:
		return events.StatusNotFound
	case rpc.StatusCode_WRONG_NODE:
		return events.StatusWrongNode
	case rpc.StatusCode_QUORUM_FAILED:
		return events.StatusQuorumFailed
	default:
		return events.StatusError
	}
}

// Helper to emit RpcCallEnd event
func (c *Client) emitRpcCallEnd(targetNodeId, key string, operation events.RpcOperation, resp *rpc.GetResponse, err error) {
	payload := events.RpcCallEndPayload{
		ClientID:     c.clientId,
		TargetNodeID: targetNodeId,
		Operation:    operation,
		Key:          key,
	}

	if err != nil {
		if grpcStatus, ok := status.FromError(err); ok {
			if grpcStatus.Code() == codes.DeadlineExceeded {
				payload.Status = events.StatusTimeout
			} else {
				payload.Status = events.StatusError
			}
		} else if errors.Is(err, context.DeadlineExceeded) {
			payload.Status = events.StatusTimeout
		} else {
			payload.Status = events.StatusError
		}
	} else if resp != nil {
		payload.Status = mapRpcStatus(resp.Status)
		if payload.Status == events.StatusOk {
			valCopy := make([]byte, len(resp.Value))
			copy(valCopy, resp.Value)
			payload.Value = &valCopy
			timestamp := resp.Timestamp.AsTime()
			payload.FinalTimestamp = &timestamp
		} else if payload.Status == events.StatusWrongNode && resp.CorrectNode != nil {
			correctNodeId := resp.CorrectNode.NodeId
			payload.CorrectNodeID = &correctNodeId
		}
	} else {
		// Should not happen if err is nil, but handle defensively
		payload.Status = events.StatusError
	}

	c.emitter.Emit(events.NewEvent(events.RpcCallEnd, payload))
}

// Helper to emit QuorumResult event
func (c *Client) emitQuorumResult(key string, operation events.QuorumOperation, success bool, responses []events.QuorumResponseDetail) {
	payload := events.QuorumResultPayload{
		SourceNodeID:   c.clientId,
		Operation:      operation,
		Key:            key,
		Success:        success,
		AchievedCount:  len(responses), // This might need refinement based on actual success definition
		RequiredQuorum: c.readQuorumR, // TODO: Adapt for Write Quorum (W)
		Responses:      responses,
	}
	c.emitter.Emit(events.NewEvent(events.QuorumResult, payload))
}

// performReadRepair asynchronously updates nodes with stale or missing data
func (c *Client) performReadRepair(ctx context.Context, key string, latestResp *rpc.GetResponse, allResponses []events.QuorumResponseDetail) {
	// Create a new background context with a short timeout for repairs
	repairCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use a separate WaitGroup for repairs
	var repairWg sync.WaitGroup

	latestTimestamp := latestResp.Timestamp.AsTime()

	for _, respDetail := range allResponses {
		needsRepair := false
		if respDetail.Status == events.StatusNotFound {
			needsRepair = true
		} else if respDetail.Status == events.StatusOk && respDetail.Timestamp != nil && respDetail.Timestamp.Before(latestTimestamp) {
			needsRepair = true
		}

		// Skip the node that already has the latest data (or if repair not needed)
		if !needsRepair || respDetail.NodeID == latestResp.Coordinator.NodeId { // Coordinator in latestResp should be the node itself
			continue
		}

		repairWg.Add(1)
		go func(staleNodeId string, staleStatus events.RpcStatus, staleTimestamp *time.Time) {
			defer repairWg.Done()

			// Find the full NodeMeta for the stale node from the cache (needed for address)
			// This assumes fetchPreferenceList populated the cache sufficiently.
			c.nodeCache.mu.Lock()
			staleNodeMeta, exists := c.nodeCache.nodes[staleNodeId]
			c.nodeCache.mu.Unlock()
			if !exists {
				log.Printf("Read repair: NodeMeta for stale node %s not found in cache, cannot repair", staleNodeId)
				// Emit repair end event with failure
				c.emitReadRepairEnd(key, staleNodeId, false, "NodeMeta not found in cache")
				return
			}

			nodeAddr := fmt.Sprintf("%s:%d", staleNodeMeta.Ip, staleNodeMeta.Port)

			// Emit Read Repair Start
			startPayload := events.ReadRepairStartPayload{
				ClientID:        c.clientId,
				Key:             key,
				TargetNodeID:    staleNodeId,
				LatestValue:     latestResp.Value, // Assuming latestResp is the one with the highest timestamp
				LatestTimestamp: latestTimestamp,
			}
			c.emitter.Emit(events.NewEvent(events.ReadRepairStart, startPayload))


			client, err := c.getNodeClient(repairCtx, nodeAddr)
			if err != nil {
				log.Printf("Read repair: Failed to get client for node %s: %v",
					staleNodeId, err)
				// Emit repair end event with failure
				c.emitReadRepairEnd(key, staleNodeId, false, fmt.Sprintf("Failed to get client: %v", err))
				return
			}

			// Send a Put request with the latest value and timestamp
			putReq := &rpc.PutRequest{
				Key:       key,
				Value:     latestResp.Value,
				Timestamp: latestResp.Timestamp, // Send the original proto timestamp
			}
			_, err = client.Put(repairCtx, putReq)

			// Emit Read Repair End
			var errMsg string
			if err != nil {
				errMsg = err.Error()
				log.Printf("Read repair: Failed to update node %s with latest value for key '%s': %v",
					staleNodeId, key, err)
				c.emitReadRepairEnd(key, staleNodeId, false, errMsg)
			} else {
				log.Printf("Read repair: Successfully updated node %s with latest value for key '%s'",
					staleNodeId, key)
				c.emitReadRepairEnd(key, staleNodeId, true, "")
			}
		}(respDetail.NodeID, respDetail.Status, respDetail.Timestamp)

	}

	// Don't wait for repairs - they're best effort and shouldn't block the main flow
	// But spawn a goroutine to wait and log when complete
	go func() {
		repairWg.Wait()
		// Maybe emit an overall 'ReadRepairCycleComplete' event?
		log.Printf("Read repair: Completed all triggered repair attempts for key '%s'", key)
	}()
}

// Helper to emit ReadRepairEnd event
func (c *Client) emitReadRepairEnd(key, targetNodeId string, success bool, errorMsg string) {
	payload := events.ReadRepairEndPayload{
		ClientID:     c.clientId,
		Key:          key,
		TargetNodeID: targetNodeId,
		Success:      success,
	}
	if !success && errorMsg != "" {
		payload.Error = &errorMsg
	}
	c.emitter.Emit(events.NewEvent(events.ReadRepairEnd, payload))
}

// *   Modify `Put(ctx context.Context, key string, value []byte)`:
// *   Use `resolveCoordinator` to find the primary node.
// *   Loop with retries (similar to Get):
// 	*   Target a node (start with primary).
// 	*   Call `Put` RPC (generate timestamp).
// 	*   Handle response (`OK`, `WRONG_NODE`, `ERROR`/Connection Error) similarly to Get, updating target node for retries.
// *   Return error if retries exhausted.

func (c *Client) Put(ctx context.Context, key string, value []byte) (time.Time, error) {

	// Resolve the coordinator for the key

	// TODO
	// coordinator, err := c.resolveCoordinator(ctx, key)
	// if err != nil {
	// 	return nil, time.Time{}, fmt.Errorf("could not resolve coordinator: %v", err)
	// }

	retryCount := 3
	preferenceList, err := c.fetchPreferenceList(ctx, key) // Handle error appropriately TODO
	if err != nil {
		return time.Time{}, fmt.Errorf("could not get preference list: %v", err)
	}
	log.Printf("preferenceList: %v", preferenceList)
	// Coordinator is the first node in the preference list
	coordinatorNodeId := preferenceList[0].NodeId
	targetNodeId := coordinatorNodeId // Start by contacting the coordinator
	triedNodes := make(map[string]bool)

	for retryCount > 0 {
		retryCount--
		triedNodes[targetNodeId] = true

		ts := timestamppb.Now() // Generate timestamp just before the call

		// Get client for the target node
		// Use a timeout for the getNodeClient call itself?
		client, err := c.getNodeClient(ctx, targetNodeId) // Should get address from NodeMeta in cache ideally
		if err != nil {
			log.Printf("Put failed: Could not get client for node %s: %v", targetNodeId, err)
			// TODO: Maybe try next preference list node if getting client fails?
			// Emit an error event here? Maybe RPC_CALL_END with error status?
			// For now, fail fast if we can't get the client for the current target.
			return time.Time{}, fmt.Errorf("failed to get client for target node %s: %w", targetNodeId, err)
		}

		// Emit RPC Call Start
		putValueCopy := make([]byte, len(value))
		copy(putValueCopy, value)
		startPayload := events.RpcCallStartPayload{
			ClientID:     c.clientId,
			TargetNodeID: targetNodeId,
			Operation:    events.OpPut,
			Key:          key,
			Value:        &putValueCopy,
		}
		c.emitter.Emit(events.NewEvent(events.RpcCallStart, startPayload))

		// Set a timeout for this Put operation
		putCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		putReq := &rpc.PutRequest{Key: key, Value: value, Timestamp: ts}
		r, err := client.Put(putCtx, putReq)
		cancel() // Cancel context after call

		// Emit RPC Call End regardless of outcome
		c.emitRpcCallEnd(targetNodeId, key, events.OpPut, r, err)

		// Handle response
		if err != nil {
			log.Printf("Put failed on node %s: %v", targetNodeId, err)
			// If context deadline exceeded or other network error, potentially try next node?
			// Current logic fails fast on error.
			return time.Time{}, fmt.Errorf("put call failed on node %s: %w", targetNodeId, err)
		}

		switch r.Status {
		case rpc.StatusCode_OK:
			// Success!
			return r.FinalTimestamp.AsTime(), nil
		case rpc.StatusCode_WRONG_NODE:
			log.Printf("WRONG_NODE received from %s: key %s should be on %v", targetNodeId, key, r.CorrectNode)
			if r.CorrectNode == nil {
				log.Printf("Error: WRONG_NODE status received but CorrectNode is nil")
				return time.Time{}, fmt.Errorf("protocol error: WRONG_NODE received without CorrectNode info")
			}
			// Update cache with correct node info
			c.nodeCache.mu.Lock()
			c.nodeCache.keyNodes[key] = r.CorrectNode.NodeId
			c.nodeCache.nodes[r.CorrectNode.NodeId] = r.CorrectNode // Add/update NodeMeta for the correct node
			c.nodeCache.mu.Unlock()

			// Set target node for the next retry
			targetNodeId = r.CorrectNode.NodeId
			log.Printf("Retrying Put for key '%s' on correct node %s", key, targetNodeId)

			// Prevent infinite loops if the suggested node was already tried
			if triedNodes[targetNodeId] {
				log.Printf("Error: WRONG_NODE pointed back to an already tried node (%s). Aborting.", targetNodeId)
				return time.Time{}, fmt.Errorf("put failed: WRONG_NODE redirection loop detected")
			}
			// Continue to the next iteration of the loop
			continue

		case rpc.StatusCode_QUORUM_FAILED:
			// The write was accepted by the coordinator but didn't achieve the required W quorum
			log.Printf("QUORUM_FAILED received from coordinator %s: Write for key '%s' didn't achieve W=%d quorum",
				targetNodeId, key, c.writeQuorumW)
			// Event was emitted by emitRpcCallEnd. Return specific error.
			return time.Time{}, fmt.Errorf("write quorum failure (W=%d): coordinator %s accepted write but couldn't replicate to enough nodes",
				c.writeQuorumW, targetNodeId)

		default:
			// Handle other unexpected non-nil errors from the RPC call that weren't caught by err != nil check
			// (e.g., internal server errors mapped to specific status codes)
			log.Printf("Put attempt failed on node %s with unexpected status: %v", targetNodeId, r.Status)

			// Simple failover: Try the next node in the preference list that hasn't been tried.
			foundNext := false
			for _, prefNode := range preferenceList {
				if !triedNodes[prefNode.NodeId] {
					targetNodeId = prefNode.NodeId
					log.Printf("Trying next node in preference list for key '%s': %s", key, targetNodeId)
					foundNext = true
					break
				}
			}

			if !foundNext {
				log.Printf("No more nodes to try in preference list for key '%s' after failure on %s.", key, targetNodeId)
				return time.Time{}, fmt.Errorf("put failed: exhausted preference list after error on node %s (status %v)", targetNodeId, r.Status)
			}
			// Continue to the next iteration of the loop with the new target node
			continue
		}
	}

	// If we exit the loop, it means retries were exhausted
	log.Printf("Put failed for key '%s' after exhausting retries.", key)
	return time.Time{}, fmt.Errorf("failed to put key '%s' after retries", key)
}

// *   Implement `getNodeClient(ctx context.Context, nodeAddr string) (rpc.NodeServiceClient, error)` using the `connMgr`.

func (c *Client) getNodeClient(ctx context.Context, nodeAddr string) (rpc.NodeServiceClient, error) {
	c.connMgr.mu.Lock()
	defer c.connMgr.mu.Unlock()

	// Check if the connection already exists
	if conn, ok := c.connMgr.connections[nodeAddr]; ok {
		return rpc.NewNodeServiceClient(conn), nil
	}

	// Create a new connection
	conn, err := grpc.Dial(nodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to node %s: %v", nodeAddr, err)
	}

	c.connMgr.connections[nodeAddr] = conn
	return rpc.NewNodeServiceClient(conn), nil
}

func (c *Client) resolveCoordinator(ctx context.Context, key string) (*rpc.NodeMeta, error) {
	c.nodeCache.mu.Lock()
	defer c.nodeCache.mu.Unlock()

	// Check if the node is in the cache
	if nodeID, ok := c.nodeCache.keyNodes[key]; ok {
		if nodeMeta, ok := c.nodeCache.nodes[nodeID]; ok {
			return nodeMeta, nil
		}
	}

	// If not in cache, fetch the preference list
	nodeMetas, err := c.fetchPreferenceList(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch preference list: %v", err)
	}

	return nodeMetas[0], nil
}

func (c *Client) fetchPreferenceList(ctx context.Context, key string) ([]*rpc.NodeMeta, error) {
	var lastErr error
	// Use a copy of seedNodes to avoid potential concurrent modification issues if list is dynamic later
	seeds := make([]string, len(c.seedNodes))
	copy(seeds, c.seedNodes)

	for _, seedNodeAddr := range seeds {
		log.Printf("Attempting to fetch preference list for key '%s' from seed node %s", key, seedNodeAddr)
		// Create a context with a timeout for the GetPreferenceList call
		prefListCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Short timeout for this specific call

		client, err := c.getNodeClient(prefListCtx, seedNodeAddr)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("failed to get node client for seed %s: %v", seedNodeAddr, err)
			log.Println(lastErr)
			continue
		}

		// Emit RPC Call Start for GetPreferenceList (optional, but can be useful)
		// c.emitter.Emit(events.NewEvent(events.RpcCallStart, events.RpcCallStartPayload{ ... }))

		r, err := client.GetPreferenceList(prefListCtx, &rpc.GetPreferenceListRequest{Key: key, N: int32(c.replicationFactorN)})
		cancel() // Cancel context as soon as we have the result or error

		if err != nil {
			lastErr = fmt.Errorf("failed to call GetPreferenceList on seed %s: %v", seedNodeAddr, err)
			log.Println(lastErr)
			// Emit RPC Call End (Error) (optional)
			// c.emitter.Emit(events.NewEvent(events.RpcCallEnd, events.RpcCallEndPayload{ ... Status: events.StatusError ... }))
			continue
		}

		if r.Status == rpc.StatusCode_OK {
			log.Printf("Successfully fetched preference list for key '%s' from %s: %v", key, seedNodeAddr, r.PreferenceList)
			// Update cache
			c.nodeCache.mu.Lock()
			nodeIds := make([]string, len(r.PreferenceList))
			for i, nodeMeta := range r.PreferenceList {
				c.nodeCache.nodes[nodeMeta.NodeId] = nodeMeta
				nodeIds[i] = nodeMeta.NodeId
			}
			if len(r.PreferenceList) > 0 {
				c.nodeCache.keyNodes[key] = r.PreferenceList[0].NodeId // Cache coordinator

				// Emit Preference List Resolved event
				payload := events.PreferenceListResolvedPayload{
					ClientID:          c.clientId,
					Key:               key,
					CoordinatorNodeID: r.PreferenceList[0].NodeId,
					PreferenceList:    nodeIds,
				}
				c.emitter.Emit(events.NewEvent(events.PreferenceListResolved, payload))
			}
			c.nodeCache.mu.Unlock()

			// Emit RPC Call End (Success) (optional)
			// c.emitter.Emit(events.NewEvent(events.RpcCallEnd, events.RpcCallEndPayload{ ... Status: events.StatusOk ... }))

			return r.PreferenceList, nil
		} else {
			// Handle non-OK status from GetPreferenceList (e.g., node busy, internal error)
			lastErr = fmt.Errorf("GetPreferenceList on seed %s returned status %s", seedNodeAddr, r.Status)
			log.Println(lastErr)
			// Emit RPC Call End (Error/Specific Status) (optional)
			// c.emitter.Emit(events.NewEvent(events.RpcCallEnd, events.RpcCallEndPayload{ ... }))
		}
	}
	log.Printf("Failed to fetch preference list for key '%s' from all seed nodes. Last error: %v", key, lastErr)
	return nil, fmt.Errorf("failed to fetch preference list for key '%s' from any seed node: %w", key, lastErr)
}

// Helper Types for Get method
type getResultWrapper struct {
	node   *rpc.NodeMeta
	resp   *rpc.GetResponse
	err    error
}
