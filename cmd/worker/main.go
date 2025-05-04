package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	virtualNodes := flag.Int("virtualnodes", 10, "number of virtual nodes per physical node")
	gossipInterval := flag.Duration("gossip", 10*time.Second, "gossip interval")
	gossipPeers := flag.Int("peers", 2, "number of peers to gossip with")
	metricsPort := flag.Int("metrics", 2112, "port for Prometheus metrics")
	discoveryRedisAddress := flag.String("discovery", "localhost:63179", "redis address for discovery")
	enableHints := flag.Bool("enablehints", false, "enable hinted handoff")
	hintInterval := flag.Duration("hintinterval", 30*time.Second, "interval to process hints")
	httpPort := flag.Int("httpport", 0, "port for HTTP API (defaults to gRPC port + 1000)")

	flag.Parse()

	// If HTTP port not specified, default to gRPC port + 1000
	if *httpPort == 0 {
		*httpPort = *port + 1000
	}

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
		VirtualNodesCount:  *virtualNodes,
		DiscoveryRedisAddr: *discoveryRedisAddress,
	}

	s := grpc.NewServer()
	n := node.NewNode(config)
	rpc.RegisterNodeServiceServer(s, n)

	// Start metrics server
	go startMetricsServer(*metricsPort)

	// Start gossip protocol if enabled
	if *gossipPeers > 0 {
		log.Printf("Starting gossip protocol (interval: %v, peers per round: %d)", *gossipInterval, *gossipPeers)
		n.StartGossip(*gossipInterval, *gossipPeers)
	}

	// Start hinted handoff processor if enabled
	if *enableHints {
		log.Printf("Starting hinted handoff processor (interval: %v)", *hintInterval)
		n.StartHintProcessor(*hintInterval)
	}

	// Start HTTP server
	err = n.StartHTTPServer(*httpPort)
	if err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}

	log.Printf("Server listening at %v", lis.Addr())
	log.Printf("Metrics available at http://localhost:%d/metrics", *metricsPort)
	log.Printf("Replication factor: %d, write quorum: %d, read quorum: %d",
		n.ReplicationFactorN, n.WriteQuorumW, n.ReadQuorumR)
	log.Printf("Virtual nodes per physical node: %d", config.VirtualNodesCount)
	log.Printf("Discovery Redis: %s", config.DiscoveryRedisAddr)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// Start Prometheus metrics HTTP server
func startMetricsServer(port int) {
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Starting metrics server on :%d", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Printf("Metrics server error: %v", err)
	}
}
