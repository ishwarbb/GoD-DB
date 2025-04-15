package node

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
	"no.cap/goddb/pkg/vclock"
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

	// Get value from local storage
	value, vc, err := n.store.Get(ctx, in.Key)
	if err != nil {
		// Key not found
		return &rpc.GetResponse{
			Status:      rpc.StatusCode_NOT_FOUND,
			Coordinator: &n.meta,
		}, nil
	}

	// Convert vector clock to timestamp for backward compatibility
	vProto := vc.ToProto()
	// Create a timestamp using the largest counter value as Unix time
	maxCounter := int64(0)
	for _, counter := range vProto.Counters {
		if counter > maxCounter {
			maxCounter = counter
		}
	}
	ts := timestamppb.New(time.Unix(maxCounter, 0))

	return &rpc.GetResponse{
		Status:      rpc.StatusCode_OK,
		Value:       value,
		Timestamp:   ts,
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
	// Initialize vector clock from timestamp
	clientVC := vclock.New()
	if in.Timestamp != nil {
		ts := in.Timestamp.AsTime()
		// Use timestamp seconds as counter for a synthetic node ID
		clientVC.Counters["client"] = ts.Unix()
	}

	// Try to get existing value and its vector clock
	_, existingVC, err := n.store.Get(ctx, in.Key)
	if err == nil && existingVC != nil {
		// Key exists, compare vector clocks
		comparison := existingVC.Compare(clientVC)

		if comparison == 0 {
			// Concurrent updates detected - needs conflict resolution
			// Here we're using a simple strategy: increment our counter
			// A more complex system might track conflicting versions

			// Merge the two vector clocks
			mergedVC := existingVC.Merge(clientVC)
			// Increment our node's counter to create a new version
			mergedVC.Increment(n.meta.NodeId)

			// Update with merged vector clock
			err = n.store.Put(ctx, in.Key, in.Value, mergedVC)
			if err != nil {
				log.Printf("Node %s: Failed to store key '%s' locally: %v", n.meta.NodeId, in.Key, err)
				return &rpc.PutResponse{
					Status:      rpc.StatusCode_ERROR,
					Coordinator: &n.meta,
				}, nil
			}

			// Create a timestamp for the response
			maxCounter := int64(0)
			for _, counter := range mergedVC.Counters {
				if counter > maxCounter {
					maxCounter = counter
				}
			}
			finalTS := timestamppb.New(time.Unix(maxCounter, 0))

			return &rpc.PutResponse{
				Status:         rpc.StatusCode_OK, // Using OK instead of CONFLICT as it's not defined
				Coordinator:    &n.meta,
				FinalTimestamp: finalTS,
			}, nil
		} else if comparison < 0 {
			// Our version is older, client has newer version
			// Update our counter in the client's VC
			clientVC.Increment(n.meta.NodeId)
		} else {
			// Our version is newer, but client attempted write with old data
			// We'll accept it but update the VC to reflect reality
			updatedVC := existingVC.Clone()
			// Merge any new info from client VC
			updatedVC = updatedVC.Merge(clientVC)
			// Increment our counter
			updatedVC.Increment(n.meta.NodeId)
			clientVC = updatedVC
		}
	} else {
		// Key doesn't exist yet, initialize vector clock if needed
		if clientVC == nil || len(clientVC.Counters) == 0 {
			clientVC = vclock.New()
		}
		// Increment our node counter
		clientVC.Increment(n.meta.NodeId)
	}

	// Store locally first
	err = n.store.Put(ctx, in.Key, in.Value, clientVC)
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

		// Create timestamp for response
		maxCounter := int64(0)
		for _, counter := range clientVC.Counters {
			if counter > maxCounter {
				maxCounter = counter
			}
		}
		finalTS := timestamppb.New(time.Unix(maxCounter, 0))

		return &rpc.PutResponse{
			Status:         rpc.StatusCode_OK, // At least local storage succeeded
			Coordinator:    &n.meta,
			FinalTimestamp: finalTS,
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
		// Create timestamp for response
		maxCounter := int64(0)
		for _, counter := range clientVC.Counters {
			if counter > maxCounter {
				maxCounter = counter
			}
		}
		finalTS := timestamppb.New(time.Unix(maxCounter, 0))

		return &rpc.PutResponse{
			Status:         rpc.StatusCode_OK,
			Coordinator:    &n.meta,
			FinalTimestamp: finalTS,
		}, nil
	}

	// Create a channel for collecting results from replica nodes
	resultsChan := make(chan *rpc.ReplicatePutResponse, len(replicaNodes))

	// Use WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Create timestamp for replication request
	maxCounter := int64(0)
	for _, counter := range clientVC.Counters {
		if counter > maxCounter {
			maxCounter = counter
		}
	}
	replicaTS := timestamppb.New(time.Unix(maxCounter, 0))

	// Create the replication request once
	replicateReq := &rpc.ReplicatePutRequest{
		Key:       in.Key,
		Value:     in.Value,
		Timestamp: replicaTS,
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
				return
			}

			// Create client and make ReplicatePut RPC call
			client := rpc.NewNodeServiceClient(conn)

			// Use a timeout context for the RPC call
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			response, err := client.ReplicatePut(callCtx, replicateReq)
			if err != nil {
				log.Printf("Node %s: ReplicatePut RPC to %s failed: %v", n.meta.NodeId, replica.NodeId, err)
				resultsChan <- &rpc.ReplicatePutResponse{
					Status: rpc.StatusCode_ERROR,
					NodeId: replica.NodeId,
				}
				return
			}

			// Send the response to the channel
			resultsChan <- response
		}(replicaNode)
	}

	// Create goroutine to wait for all replications and close the channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect responses and check for quorum
	for response := range resultsChan {
		if response.Status == rpc.StatusCode_OK {
			successCount++

			// If we've reached the write quorum, we can return success
			if successCount >= n.WriteQuorumW {
				// We could break here, but let's wait for all responses for completeness
				break
			}
		}
	}

	// Create final timestamp for response
	finalTS := timestamppb.New(time.Unix(maxCounter, 0))

	// Check if we achieved quorum
	if successCount >= n.WriteQuorumW {
		return &rpc.PutResponse{
			Status:         rpc.StatusCode_OK,
			Coordinator:    &n.meta,
			FinalTimestamp: finalTS,
		}, nil
	} else {
		log.Printf("Node %s: Write quorum not achieved for key '%s'. Got %d successes, needed %d",
			n.meta.NodeId, in.Key, successCount, n.WriteQuorumW)
		return &rpc.PutResponse{
			Status:         rpc.StatusCode_QUORUM_FAILED,
			Coordinator:    &n.meta,
			FinalTimestamp: finalTS,
		}, nil
	}
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

	// Convert timestamp to vector clock
	vc := vclock.New()
	if req.Timestamp != nil {
		ts := req.Timestamp.AsTime()
		// Use timestamp seconds as counter for coordinator's node ID
		vc.Counters["coordinator"] = ts.Unix()
		// Also increment our counter to show we've seen it
		vc.Increment(n.meta.NodeId)
	} else {
		// If no timestamp, create a new vector clock with just our ID
		vc.Increment(n.meta.NodeId)
	}

	// Directly call the local store's Put method
	err := n.store.Put(ctx, req.Key, req.Value, vc)
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

// Additional RPC handlers will be added here
