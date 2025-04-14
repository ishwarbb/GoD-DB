package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"no.cap/goddb/pkg/client"
)

func main() {
	serverAddr := flag.String("server", "localhost:50051", "the address to connect to")
	replicationFactor := flag.Int("replication", 1, "replication factor (N)")
	writeQuorum := flag.Int("writequorum", 1, "write quorum (W)")
	readQuorum := flag.Int("readquorum", 1, "read quorum (R)")
	flag.Parse()

	// Create client with specified replication parameters
	client, err := client.NewClient(*serverAddr, []string{*serverAddr}, *replicationFactor, *readQuorum, *writeQuorum)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	log.Printf("Connected to server at %s with replication factor N=%d, write quorum W=%d, read quorum R=%d",
		*serverAddr, *replicationFactor, *writeQuorum, *readQuorum)

	client.Put(context.Background(), "foo", []byte("bar"))
	client.Put(context.Background(), "foo", []byte("meow"))
	client.Put(context.Background(), "foo", []byte("rawr"))
	client.Put(context.Background(), "foo", []byte("llil"))
	client.Put(context.Background(), "meow", []byte("meow"))
	client.Put(context.Background(), "uwu", []byte("uwu"))
	client.Put(context.Background(), "lol", []byte("lol"))

	val, ts, err := client.Get(context.Background(), "foo")
	if err != nil {
		log.Fatalf("failed to get: %v", err)
	}

	fmt.Println(string(val))
	fmt.Println(ts.Format(time.RFC3339))

	val, ts, err = client.Get(context.Background(), "meow")
	if err != nil {
		log.Fatalf("failed to get: %v", err)
	}

	fmt.Println(string(val))
	fmt.Println(ts.Format(time.RFC3339))

	val, ts, err = client.Get(context.Background(), "uwu")
	if err != nil {
		log.Fatalf("failed to get: %v", err)
	}

	fmt.Println(string(val))
	fmt.Println(ts.Format(time.RFC3339))

	val, ts, err = client.Get(context.Background(), "lol")
	if err != nil {
		log.Fatalf("failed to get: %v", err)
	}

	fmt.Println(string(val))
	fmt.Println(ts.Format(time.RFC3339))
}
