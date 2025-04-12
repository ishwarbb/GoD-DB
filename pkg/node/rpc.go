package node

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

func (n *Node) Ping(ctx context.Context, in *rpc.PingRequest) (*rpc.PingResponse, error) {
	// log.Printf("Received: %v", in())
	// time.Sleep(1 * time.Second)
	return &rpc.PingResponse{Message: fmt.Sprintf("pong from port %d", n.Port)}, nil
}

func (n *Node) Get(ctx context.Context, in *rpc.GetRequest) (*rpc.GetResponse, error) {
	nodeID, err := n.ring.GetNode(in.Key)
	if err != nil {
		return nil, err
	}

	nodeMeta, success := n.ring.GetNodeMeta(nodeID)
	if !success { 
		return nil, fmt.Errorf("node not found in ring: %s", nodeID)
	}

	if nodeID != nodeMeta.NodeId {
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

	return &rpc.GetResponse{
		Value:       val,
		Timestamp:   timestamppb.New(ts),
		Status:      rpc.StatusCode_OK,
		Coordinator: &n.meta,
	}, nil
}

func (n *Node) Put(ctx context.Context, in *rpc.PutRequest) (*rpc.PutResponse, error) {
	nodeID, err := n.ring.GetNode(in.Key)
	if err != nil {
		return nil, err
	}
	fmt.Println("nodeID: ", nodeID)
	fmt.Println("n.meta.NodeId: ", n.meta.NodeId)
	fmt.Println("")

	nodeMeta, success := n.ring.GetNodeMeta(nodeID)
	if !success { 
		return nil, fmt.Errorf("node not found in ring: %s", nodeID)
	}

	if nodeID != nodeMeta.NodeId {
		return &rpc.PutResponse{
			Status:      rpc.StatusCode_WRONG_NODE,
			Coordinator: &n.meta,
			CorrectNode: &nodeMeta,
		}, nil
	}

	err = n.store.Put(ctx, in.Key, in.Value, in.Timestamp.AsTime())
	if err != nil {
		return nil, err
	}

	return &rpc.PutResponse{
		Status:         rpc.StatusCode_OK,
		Coordinator:    &n.meta,
		FinalTimestamp: in.Timestamp,
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

	return &rpc.GetPreferenceListResponse{
		Status:      rpc.StatusCode_OK,
		PreferenceList:   nodes,
		Coordinator: &n.meta,
	}, nil
}