package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"no.cap/goddb/pkg/node"
	"no.cap/goddb/pkg/rpc"

	"google.golang.org/grpc"
)

func main() {
	port := flag.Int("port", 50051, "the port to listen on")
	redisPort := flag.Int("redisport", 63079, "the redis port")
	replicationFactor := flag.Int("replication", 3, "replication factor (N)")
	writeQuorum := flag.Int("writequorum", 2, "write quorum (W)")
	readQuorum := flag.Int("readquorum", 2, "read quorum (R)")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Create node config with command line parameters
	config := node.NodeConfig{
		Port:               *port,
		RedisPort:          *redisPort,
		ReplicationFactorN: *replicationFactor,
		WriteQuorumW:       *writeQuorum,
		ReadQuorumR:        *readQuorum,
	}

	s := grpc.NewServer()
	n := node.NewNode(config)
	rpc.RegisterNodeServiceServer(s, n)

	log.Printf("server listening at %v", lis.Addr())
	log.Printf("replication factor: %d, write quorum: %d, read quorum: %d",
		n.ReplicationFactorN, n.WriteQuorumW, n.ReadQuorumR)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
