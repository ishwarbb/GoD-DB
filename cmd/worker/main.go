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
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	n := node.NewNode(*port, *redisPort)
	rpc.RegisterNodeServiceServer(s, n)

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
