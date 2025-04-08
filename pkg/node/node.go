package node

import (
	"no.cap/goddb/pkg/rpc"
)

type Node struct {
	rpc.UnimplementedNodeServiceServer
	Port      int
	RedisPort int
}

func NewNode(port int, redisPort int) *Node {
	return &Node{
		Port:      port,
		RedisPort: redisPort,
	}
}
