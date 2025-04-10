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

	if nodeID != n.meta.NodeId {
		// TODO: need to forward request
		// return nil, fmt.Errorf("key not found on this node")
		return &rpc.GetResponse{
			Status:      rpc.StatusCode_WRONG_NODE,
			Coordinator: &n.meta,
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

	if nodeID != n.meta.NodeId {
		// TODO: need to forward request
		// return nil, fmt.Errorf("key not found on this node")
		return &rpc.PutResponse{
			Status:      rpc.StatusCode_WRONG_NODE,
			Coordinator: &n.meta,
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
