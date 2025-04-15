package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
	"no.cap/goddb/pkg/vclock"
)

var (
	ErrNotFound          = errors.New("key not found")
	ErrReadQuorumFailed  = errors.New("read quorum not achieved")
	ErrWriteQuorumFailed = errors.New("write quorum not achieved")
	ErrConflict          = errors.New("conflict detected during write")
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
	vectorClocks       map[string]*vclock.VectorClock
	vcMutex            sync.RWMutex
}

func NewNodeCache(ttl time.Duration) *NodeCache {
	return &NodeCache{
		nodes:    make(map[string]*rpc.NodeMeta),
		keyNodes: make(map[string]string),
		ttl:      ttl,
	}
}

func NewClient(addr string, seedNodes []string, replicationFactorN, readQuorumR, writeQuorumW int) (*Client, error) {
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

	return &Client{conn: conn, rpc: c, nodeCache: node_cache, seedNodes: []string{addr}, replicationFactorN: replicationFactorN, readQuorumR: readQuorumR, writeQuorumW: writeQuorumW, connMgr: conn_mgr}, nil
}

func (c *Client) Close() {
	c.conn.Close()
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
		return nil, time.Time{}, fmt.Errorf("key not found")
	}

	// Perform read repair for nodes with stale or missing data
	c.performReadRepair(ctx, key, latestResponse, validResponses)

	// Return the latest value
	return latestResponse.Value, latestResponse.Timestamp.AsTime(), nil
}

// performReadRepair asynchronously updates nodes with stale or missing data
func (c *Client) performReadRepair(ctx context.Context, key string, latestResp *rpc.GetResponse, responses []*rpc.GetResponse) {
	// Create a new background context with a short timeout for repairs
	repairCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use a separate WaitGroup for repairs
	var repairWg sync.WaitGroup

	for _, resp := range responses {
		// Skip the node that already has the latest data
		if resp == latestResp {
			continue
		}

		// Repair if the node has no data or stale data
		if resp.Status == rpc.StatusCode_NOT_FOUND ||
			resp.Timestamp.AsTime().Before(latestResp.Timestamp.AsTime()) {

			repairWg.Add(1)

			go func(staleResp *rpc.GetResponse) {
				defer repairWg.Done()

				// Get the client for the node that needs repair
				nodeAddr := fmt.Sprintf("%s:%d", staleResp.Coordinator.Ip, staleResp.Coordinator.Port)
				client, err := c.getNodeClient(repairCtx, nodeAddr)
				if err != nil {
					log.Printf("Read repair: Failed to get client for node %s: %v",
						staleResp.Coordinator.NodeId, err)
					return
				}

				// Send a Put request with the latest value and timestamp
				_, err = client.Put(repairCtx, &rpc.PutRequest{
					Key:       key,
					Value:     latestResp.Value,
					Timestamp: latestResp.Timestamp,
				})

				if err != nil {
					log.Printf("Read repair: Failed to update node %s with latest value for key '%s': %v",
						staleResp.Coordinator.NodeId, key, err)
				} else {
					log.Printf("Read repair: Successfully updated node %s with latest value for key '%s'",
						staleResp.Coordinator.NodeId, key)
				}
			}(resp)
		}
	}

	// Don't wait for repairs - they're best effort and shouldn't block the main flow
	// But spawn a goroutine to wait and log when complete
	go func() {
		repairWg.Wait()
		log.Printf("Read repair: Completed all repair attempts for key '%s'", key)
	}()
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
	targetNode := preferenceList[0].NodeId
	triedNodes := make(map[string]bool)

	for retryCount > 0 {
		retryCount--
		triedNodes[targetNode] = true

		ts := timestamppb.Now()

		client, err := c.getNodeClient(ctx, targetNode)
		if err != nil {
			log.Printf("Failed to get client for node %s: %v", targetNode, err)
			return time.Time{}, fmt.Errorf("could not put: %v", err)
		}

		// Set a timeout for this read operation
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		r, err := client.Put(readCtx, &rpc.PutRequest{Key: key, Value: value, Timestamp: ts})
		if err != nil {
			return time.Time{}, fmt.Errorf("could not put: %v", err)
		}

		switch r.Status {
		case rpc.StatusCode_OK:
			return r.FinalTimestamp.AsTime(), nil
		case rpc.StatusCode_WRONG_NODE:
			log.Printf("WRONG_NODE: key %s should be on %v", key, r.CorrectNode)

			targetNode = r.CorrectNode.NodeId
			if targetNode == c.nodeCache.keyNodes[key] {
				log.Printf("Key %s is already in the cache with the correct node %v", key, r.CorrectNode)
				c.nodeCache.mu.Lock()
				c.nodeCache.keyNodes[key] = r.CorrectNode.NodeId
				c.nodeCache.nodes[r.Coordinator.NodeId] = r.CorrectNode
				c.nodeCache.mu.Unlock()
				return time.Time{}, fmt.Errorf("key already in cache with correct node")
			}
			log.Printf("c.nodeCache.keyNodes[key]: %v", c.nodeCache.keyNodes[key])
			log.Printf("targetNode: %v", targetNode)

		case rpc.StatusCode_QUORUM_FAILED:
			// The write was accepted by the coordinator but didn't achieve the required W quorum
			log.Printf("QUORUM_FAILED: Write to key %s was accepted by coordinator %s but didn't achieve W=%d quorum",
				key, r.Coordinator.NodeId, c.writeQuorumW)
			return time.Time{}, fmt.Errorf("write quorum failure: coordinator accepted write but couldn't replicate to enough nodes")
		default:
			log.Printf("Unknown error: %v", r.Status)
			// Log the error and pick the next node from the preference list

			preferenceList, _ = c.fetchPreferenceList(ctx, key) // Handle error appropriately TODO
			isUpdate := false
			for i := 0; i < len(preferenceList); i++ {
				if triedNodes[preferenceList[i].NodeId] {
					continue
				}

				targetNode = preferenceList[i].NodeId
				isUpdate = true
				break
			}

			if !isUpdate {
				log.Printf("No more nodes to try for key %s", key)
				return time.Time{}, fmt.Errorf("no more nodes to try")
			}
		}
	}

	// If we reach here, it means we exhausted all retries
	return time.Time{}, fmt.Errorf("failed to put key %s after retries", key)
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
	for _, seedNode := range c.seedNodes {
		client, err := c.getNodeClient(ctx, seedNode)
		if err != nil {
			lastErr = fmt.Errorf("failed to get node client: %v", err)
			continue
		}

		r, err := client.GetPreferenceList(ctx, &rpc.GetPreferenceListRequest{Key: key, N: int32(c.replicationFactorN)})
		if err != nil {
			lastErr = fmt.Errorf("failed to get preference list: %v", err)
			continue
		}

		if r.Status == rpc.StatusCode_OK {
			for _, nodeMeta := range r.PreferenceList {
				c.nodeCache.nodes[nodeMeta.NodeId] = nodeMeta
			}

			c.nodeCache.keyNodes[key] = r.PreferenceList[0].NodeId
			return r.PreferenceList, nil
		}
	}

	return nil, lastErr
}
