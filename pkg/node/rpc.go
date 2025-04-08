package node

import (
	"context"
	"fmt"

	"no.cap/goddb/pkg/rpc"
)

func (n *Node) Ping(ctx context.Context, in *rpc.PingRequest) (*rpc.PingResponse, error) {
	// log.Printf("Received: %v", in())
	// time.Sleep(1 * time.Second)
	return &rpc.PingResponse{Message: fmt.Sprintf("pong from port %d", n.Port)}, nil
}
