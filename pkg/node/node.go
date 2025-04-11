package node

import (
	"fmt"

	"no.cap/goddb/pkg/chash"
	"no.cap/goddb/pkg/rpc"
	"no.cap/goddb/pkg/storage"
)

type Node struct {
	rpc.UnimplementedNodeServiceServer
	Port      int
	RedisPort int
	ring      *chash.Ring
	store     *storage.Store
	meta      rpc.NodeMeta
}

func NewNode(port int, redisPort int) *Node {
	store, err := storage.NewStore(
		fmt.Sprintf("localhost:%d", redisPort),
	)

	ring := chash.NewRing()
	for i := 0; i < 5; i++ {
		ring.AddNode(rpc.NodeMeta{
			Ip:   "localhost",
			Port: int32(port),
			NodeId: fmt.Sprintf("localhost:%d", port),
		})
	}

	if err != nil {
		panic(err) // TODO: deal with this better
	}

	return &Node{
		Port:      port,
		RedisPort: redisPort,
		ring:      ring,
		store:     store,
		meta: rpc.NodeMeta{
			NodeId: fmt.Sprintf("localhost:%d", port),
			Ip:     "localhost",
			Port:   int32(port),
		},
	}
}
