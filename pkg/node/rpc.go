package node

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

func (n *Node) Ping(ctx context.Context, in *rpc.PingRequest) (*rpc.PingResponse, error) {
	// log.Printf("Received: %v", in())
	// time.Sleep(1 * time.Second)
	return &rpc.PingResponse{Message: fmt.Sprintf("pong from port %d", n.Port)}, nil
}

func (n *Node) Get(ctx context.Context, in *rpc.GetRequest) (*rpc.GetResponse, error) {

	potentialNodes, err := n.ring.GetN(in.Key, n.ReplicationFactorN)
	if err != nil {
		return nil, err
	}

	// find if n.meta.NodeId is in the potentialNodes
	nodeID := ""
	for _, node := range potentialNodes {
		if node == n.meta.NodeId {
			nodeID = node
			break
		}
	}

	if nodeID == "" {
		nodeMeta, success := n.ring.GetNodeMeta(potentialNodes[0])
		if !success {
			return nil, fmt.Errorf("node not found in ring: %s", nodeID)
		}

		return &rpc.GetResponse{
			Status:      rpc.StatusCode_WRONG_NODE,
			Coordinator: &n.meta,
			CorrectNode: &nodeMeta,
		}, nil
	}

	val, ts, err := n.store.Get(ctx, in.Key)
	if err != nil {
		return nil, err
	}
	
	// If we got an empty value, it means the key was not found
	if len(val) == 0 {
		return &rpc.GetResponse{
			Status:      rpc.StatusCode_NOT_FOUND,
			Coordinator: &n.meta,
		}, nil
	}

	return &rpc.GetResponse{
		Value:       val,
		Timestamp:   timestamppb.New(ts),
		Status:      rpc.StatusCode_OK,
		Coordinator: &n.meta,
	}, nil
}

func (n *Node) Put(ctx context.Context, in *rpc.PutRequest) (*rpc.PutResponse, error) {
	potentialNodes, err := n.ring.GetN(in.Key, n.ReplicationFactorN)
	if err != nil {
		return nil, err
	}

	// find if n.meta.NodeId is in the potentialNodes
	nodeID := ""
	for _, node := range potentialNodes {
		if node == n.meta.NodeId {
			nodeID = node
			break
		}
	}

	if nodeID == "" {
		nodeMeta, success := n.ring.GetNodeMeta(potentialNodes[0])
		if !success {
			return nil, fmt.Errorf("node not found in ring: %s", nodeID)
		}
		return &rpc.PutResponse{
			Status:      rpc.StatusCode_WRONG_NODE,
			Coordinator: &n.meta,
			CorrectNode: &nodeMeta,
		}, nil
	}

	// This node is the coordinator for this key
	// First, store the value locally
	err = n.store.Put(ctx, in.Key, in.Value, in.Timestamp.AsTime())
	if err != nil {
		log.Printf("Node %s: Failed to store key '%s' locally: %v", n.meta.NodeId, in.Key, err)
		return &rpc.PutResponse{
			Status:      rpc.StatusCode_ERROR,
			Coordinator: &n.meta,
		}, nil
	}

	// Initialize success count to 1 (local store succeeded)
	successCount := 1

	// Get the full preference list of nodes responsible for this key
	nodeIDs, err := n.ring.GetN(in.Key, n.ReplicationFactorN)
	if err != nil {
		log.Printf("Node %s: Failed to get preference list for key '%s': %v", n.meta.NodeId, in.Key, err)
		
		// If write quorum is 1, we can still succeed with just local storage
		if n.WriteQuorumW <= 1 {
			return &rpc.PutResponse{
				Status:         rpc.StatusCode_OK, 
				Coordinator:    &n.meta,
				FinalTimestamp: in.Timestamp,
			}, nil
		}
		
		// Otherwise, return an error for quorum failure
		return &rpc.PutResponse{
			Status:      rpc.StatusCode_ERROR,
			Coordinator: &n.meta,
			Message:     "Failed to achieve write quorum",
		}, nil
	}

	// Identify the replica nodes (excluding self)
	replicaNodes := make([]*rpc.NodeMeta, 0, len(nodeIDs)-1)
	for _, id := range nodeIDs {
		if id != n.meta.NodeId {
			nodeMeta, ok := n.ring.GetNodeMeta(id)
			if ok {
				replicaNodes = append(replicaNodes, &nodeMeta)
			}
		}
	}

	// If there are no other nodes, return success
	if len(replicaNodes) == 0 {
		return &rpc.PutResponse{
			Status:         rpc.StatusCode_OK,
			Coordinator:    &n.meta,
			FinalTimestamp: in.Timestamp,
		}, nil
	}

	// Create a channel for collecting results from replica nodes
	resultsChan := make(chan *rpc.ReplicatePutResponse, len(replicaNodes))
	
	// Create a channel for collecting failed nodes for hinted handoff
	failedNodes := make(chan *rpc.NodeMeta, len(replicaNodes))

	// Use WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Create the replication request once
	replicateReq := &rpc.ReplicatePutRequest{
		Key:       in.Key,
		Value:     in.Value,
		Timestamp: in.Timestamp,
	}

	// Launch goroutines to replicate to each node
	for _, replicaNode := range replicaNodes {
		wg.Add(1)

		go func(replica *rpc.NodeMeta) {
			defer wg.Done()

			// Get the connection to the replica node using the connection manager
			replicaAddr := fmt.Sprintf("%s:%d", replica.Ip, replica.Port)

			// Get a connection from the connection manager
			conn, err := n.connMgr.GetConnection(replicaAddr)
			if err != nil {
				log.Printf("Node %s: Failed to connect to replica %s: %v", n.meta.NodeId, replica.NodeId, err)
				resultsChan <- &rpc.ReplicatePutResponse{
					Status: rpc.StatusCode_ERROR,
					NodeId: replica.NodeId,
				}
				// Add this node to the failed nodes for hinted handoff
				failedNodes <- replica
				return
			}

			// Create client and make ReplicatePut RPC call
			client := rpc.NewNodeServiceClient(conn)

			// Use a timeout context for the RPC call
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			response, err := client.ReplicatePut(callCtx, replicateReq)
			if err != nil {
				log.Printf("Node %s: ReplicatePut RPC to %s failed: %v", n.meta.NodeId, replica.NodeId, err)
				resultsChan <- &rpc.ReplicatePutResponse{
					Status: rpc.StatusCode_ERROR,
					NodeId: replica.NodeId,
				}
				// Add this node to the failed nodes for hinted handoff
				failedNodes <- replica
				return
			}

			// Send the response to the channel
			resultsChan <- response
		}(replicaNode)
	}

	// Create goroutine to wait for all replications and close the channels
	go func() {
		wg.Wait()
		close(resultsChan)
		close(failedNodes)
	}()

	// Collect responses and check for quorum
	for response := range resultsChan {
		if response.Status == rpc.StatusCode_OK {
			successCount++
			
			// If we've achieved write quorum, we can return success immediately
			if successCount >= n.WriteQuorumW {
				// Still process any hints in the background
				go func() {
					// Process any failed nodes for hinted handoff
					for failedNode := range failedNodes {
						err := n.store.StoreHint(context.Background(), failedNode.NodeId, in.Key, in.Value, in.Timestamp.AsTime())
						if err != nil {
							log.Printf("Node %s: Failed to store hint for node %s: %v", 
								n.meta.NodeId, failedNode.NodeId, err)
						}
					}
				}()
				
				return &rpc.PutResponse{
					Status:         rpc.StatusCode_OK,
					Coordinator:    &n.meta,
					FinalTimestamp: in.Timestamp,
				}, nil
			}
		}
	}

	// Process any failed nodes for hinted handoff
	var failedNodeList []*rpc.NodeMeta
	for failedNode := range failedNodes {
		failedNodeList = append(failedNodeList, failedNode)
		err := n.store.StoreHint(context.Background(), failedNode.NodeId, in.Key, in.Value, in.Timestamp.AsTime())
		if err != nil {
			log.Printf("Node %s: Failed to store hint for node %s: %v", 
				n.meta.NodeId, failedNode.NodeId, err)
		} else {
			log.Printf("Node %s: Stored hint for node %s, key %s", 
				n.meta.NodeId, failedNode.NodeId, in.Key)
		}
	}

	// After collecting all responses, check if we achieved write quorum
	if successCount >= n.WriteQuorumW {
		return &rpc.PutResponse{
			Status:         rpc.StatusCode_OK,
			Coordinator:    &n.meta,
			FinalTimestamp: in.Timestamp,
		}, nil
	}

	// We failed to achieve write quorum
	failedMsg := fmt.Sprintf("Failed to achieve write quorum: required %d, got %d", n.WriteQuorumW, successCount)
	if len(failedNodeList) > 0 {
		failedMsg += fmt.Sprintf(" (stored %d hints for unavailable nodes)", len(failedNodeList))
	}
	
	log.Printf("Node %s: %s", n.meta.NodeId, failedMsg)
	
	return &rpc.PutResponse{
		Status:      rpc.StatusCode_ERROR,
		Coordinator: &n.meta,
		Message:     failedMsg,
	}, nil
}

func (n *Node) GetPreferenceList(ctx context.Context, in *rpc.GetPreferenceListRequest) (*rpc.GetPreferenceListResponse, error) {
	nodeIDs, err := n.ring.GetN(in.Key, int(in.N))
	if err != nil {
		return nil, err
	}

	nodes := make([]*rpc.NodeMeta, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeMeta, ok := n.ring.GetNodeMeta(id)
		if !ok {
			return nil, fmt.Errorf("node not found in ring: %s", id)
		}
		nodes = append(nodes, &nodeMeta)
	}

	if len(nodes) < int(in.N) {
		return nil, fmt.Errorf("not enough nodes in ring")
	}

	return &rpc.GetPreferenceListResponse{
		Status:         rpc.StatusCode_OK,
		PreferenceList: nodes,
		Coordinator:    &n.meta,
	}, nil
}

// ReplicatePut is called by a coordinator node onto a replica to store a key-value pair.
func (n *Node) ReplicatePut(ctx context.Context, req *rpc.ReplicatePutRequest) (*rpc.ReplicatePutResponse, error) {
	log.Printf("Node %s: Received ReplicatePut request for key '%s'", n.meta.NodeId, req.Key)

	// Convert protobuf timestamp to time.Time
	ts := req.Timestamp.AsTime()

	// Directly call the local store's Put method
	err := n.store.Put(ctx, req.Key, req.Value, ts)
	if err != nil {
		log.Printf("Node %s: Failed to store replicated key '%s' in local store: %v", n.meta.NodeId, req.Key, err)
		return &rpc.ReplicatePutResponse{
			Status: rpc.StatusCode_ERROR,
			NodeId: n.meta.NodeId,
		}, nil // Return nil error, status code indicates failure
	}

	log.Printf("Node %s: Successfully stored replicated key '%s'", n.meta.NodeId, req.Key)
	return &rpc.ReplicatePutResponse{
		Status: rpc.StatusCode_OK,
		NodeId: n.meta.NodeId,
	}, nil
}

// Add the implementation of the Gossip RPC method
func (n *Node) Gossip(ctx context.Context, req *rpc.GossipRequest) (*rpc.GossipResponse, error) {
	// Log the gossip request
	log.Printf("Node %s: Received gossip from node %s", n.meta.NodeId, req.SenderNode.NodeId)

	// Process the received node states by merging them into our ring
	n.mergeGossipData(req.NodeStates)

	// Prepare gossip response
	response := &rpc.GossipResponse{
		UpdatedNodeStates: make(map[string]*rpc.NodeMeta),
	}

	// Get all nodes from our ring
	localNodes := n.ring.GetAllNodes()

	// Compare each local node with what sender knows
	for _, localNode := range localNodes {
		// Check if sender knows about this node
		senderNode, senderKnows := req.NodeStates[localNode.NodeId]

		// If sender doesn't know this node OR our version is newer, send our info
		if !senderKnows || localNode.Version > senderNode.Version {
			// Make a copy of the NodeMeta to avoid modification
			nodeCopy := localNode
			response.UpdatedNodeStates[localNode.NodeId] = &nodeCopy
		}
	}

	log.Printf("Node %s: Sending %d updated node states to %s",
		n.meta.NodeId, len(response.UpdatedNodeStates), req.SenderNode.NodeId)

	return response, nil
}

// ReplayHint handles a request to replay a hinted handoff
func (n *Node) ReplayHint(ctx context.Context, req *rpc.ReplayHintRequest) (*rpc.ReplayHintResponse, error) {
	hint := req.Hint
	log.Printf("Node %s: Processing hint replay for key %s (target node: %s, created at: %s)",
		n.meta.NodeId, hint.Key, hint.TargetNodeId, hint.HintCreatedAt.AsTime().Format(time.RFC3339))

	// Verify this node is the intended recipient of the hint
	if hint.TargetNodeId != n.meta.NodeId {
		log.Printf("Node %s: Rejecting hint for key %s - intended for node %s", 
			n.meta.NodeId, hint.Key, hint.TargetNodeId)
		return &rpc.ReplayHintResponse{
			Status:  rpc.StatusCode_ERROR,
			Message: "Hint intended for different node",
		}, nil
	}

	// Verify key belongs on this node
	potentialNodes, err := n.ring.GetN(hint.Key, n.ReplicationFactorN)
	if err != nil {
		return nil, fmt.Errorf("failed to get preference list for key: %w", err)
	}

	// Check if this node is in the preference list
	nodeInPreference := false
	for _, nodeID := range potentialNodes {
		if nodeID == n.meta.NodeId {
			nodeInPreference = true
			break
		}
	}

	if !nodeInPreference {
		log.Printf("Node %s: Rejecting hint for key %s - no longer responsible for this key", 
			n.meta.NodeId, hint.Key)
		return &rpc.ReplayHintResponse{
			Status:  rpc.StatusCode_WRONG_NODE,
			Message: "Key no longer belongs on this node",
		}, nil
	}

	// Store the value in the local store
	err = n.store.Put(ctx, hint.Key, hint.Value, hint.Timestamp.AsTime())
	if err != nil {
		log.Printf("Node %s: Failed to store hinted value for key %s: %v", 
			n.meta.NodeId, hint.Key, err)
		return &rpc.ReplayHintResponse{
			Status:  rpc.StatusCode_ERROR,
			Message: fmt.Sprintf("Failed to store value: %v", err),
		}, nil
	}

	log.Printf("Node %s: Successfully processed hint for key %s", n.meta.NodeId, hint.Key)
	return &rpc.ReplayHintResponse{
		Status:  rpc.StatusCode_OK,
		Message: "Hint processed successfully",
	}, nil
}

// Additional RPC handlers will be added here
