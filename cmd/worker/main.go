package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"no.cap/goddb/pkg/node"
	"no.cap/goddb/pkg/rpc"

	"google.golang.org/grpc"
)

func main() {
	port := flag.Int("port", 50051, "the port to listen on")
	redisPort := flag.Int("redisport", 63079, "the redis port")
	replicationFactor := flag.Int("replication", 3, "replication factor (N)")
	writeQuorum := flag.Int("writequorum", 3, "write quorum (W)")
	readQuorum := flag.Int("readquorum", 3, "read quorum (R)")
	gossipInterval := flag.Duration("gossip", 10*time.Second, "gossip interval")
	gossipPeers := flag.Int("peers", 2, "number of peers to gossip with each round")
	enableGossip := flag.Bool("enablegossip", true, "enable gossip protocol")
	discoveryRedis := flag.String("discovery", "localhost:63179", "Redis address for service discovery")
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
		DiscoveryRedisAddr: *discoveryRedis,
	}

	s := grpc.NewServer()
	n := node.NewNode(config)
	rpc.RegisterNodeServiceServer(s, n)

	// Start gossip protocol if enabled
	if *enableGossip {
		log.Printf("Starting gossip protocol (interval: %v, peers per round: %d)", *gossipInterval, *gossipPeers)
		n.StartGossip(*gossipInterval, *gossipPeers)
	}

	log.Printf("server listening at %v", lis.Addr())
	log.Printf("replication factor: %d, write quorum: %d, read quorum: %d",
		n.ReplicationFactorN, n.WriteQuorumW, n.ReadQuorumR)
	log.Printf("discovery Redis: %s", config.DiscoveryRedisAddr)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
