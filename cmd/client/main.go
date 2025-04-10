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
	flag.Parse()

	client, err := client.NewClient(*serverAddr)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	client.Put(context.Background(), "foo", []byte("bar"))
	val, ts, err := client.Get(context.Background(), "foo")
	if err != nil {
		log.Fatalf("failed to get: %v", err)
	}

	fmt.Println(string(val))
	fmt.Println(ts.Format(time.RFC3339))
}
