package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"no.cap/goddb/pkg/client"
)

func main() {
	// time.Sleep(10 * time.Second)
	serverAddr := flag.String("server", "localhost:50051", "the address to connect to")
	replicationFactor := flag.Int("replication", 3, "replication factor (N)")
	writeQuorum := flag.Int("writequorum", 3, "write quorum (W)")
	readQuorum := flag.Int("readquorum", 3, "read quorum (R)")
	flag.Parse()

	// Create client with specified replication parameters
	client, err := client.NewClient(*serverAddr, []string{*serverAddr}, *replicationFactor, *readQuorum, *writeQuorum)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	log.Printf("Connected to server at %s with replication factor N=%d, write quorum W=%d, read quorum R=%d",
		*serverAddr, *replicationFactor, *writeQuorum, *readQuorum)

	// Keep track of recently accessed keys
	recentKeys := make(map[string]bool)

	// Interactive terminal loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		args := strings.Fields(input)

		if len(args) == 0 {
			continue
		}

		command := strings.ToLower(args[0])

		switch command {
		case "put", "p":
			if len(args) < 3 {
				fmt.Println("Usage: put <key> <value>")
				continue
			}
			key := args[1]
			value := strings.Join(args[2:], " ")
			// measure latency of put
			start := time.Now()
			ts, err := client.Put(context.Background(), key, []byte(value))
			end := time.Now()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				fmt.Printf("put_latency: %v\n", end.Sub(start))
			} else {
				fmt.Printf("Stored key '%s' with value '%s'\n", key, value)
				fmt.Printf("Timestamp: %v\n", ts)
				fmt.Printf("put_latency: %v\n", end.Sub(start))
				recentKeys[key] = true
			}

		case "get", "g":
			if len(args) != 2 {
				fmt.Println("Usage: get <key>")
				continue
			}
			key := args[1]

			start := time.Now()
			value, ts, err := client.Get(context.Background(), key)
			end := time.Now()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				fmt.Printf("get_latency: %v\n", end.Sub(start))
			} else {
				fmt.Printf("Key: %s\n", key)
				fmt.Printf("Value: %s\n", string(value))
				fmt.Printf("Timestamp: %v\n", ts)
				fmt.Printf("get_latency: %v\n", end.Sub(start))
				recentKeys[key] = true
			}

		case "exit", "quit", "q":
			fmt.Println("Goodbye!")
			return
		case "wait", "w":
			if len(args) != 2 {
				fmt.Println("Usage: wait <seconds>")
				continue
			}
			waittime, _ := strconv.Atoi(args[1])
			fmt.Println("Waiting for", waittime, "seconds...")
			for i := 0; i < waittime; i++ {
				time.Sleep(1 * time.Second)
				// create a big loop to approx sleep
				// for j := 0; j < 100000000; j++ {
				// 	time.Sleep(1 * time.Microsecond)
				// }
			}
		case "help", "h":
			fmt.Println("Available commands:")
			fmt.Println("  put <key> <value> (p) - Store a key-value pair")
			fmt.Println("  get <key> (g) - Retrieve the value of a key")
			fmt.Println("  wait <seconds> (w) - Wait for a specified number of seconds")
			fmt.Println("  exit/quit (q) - Exit the client")
			fmt.Println("  help (h) - Show this help message")
		case "recent", "r":
			fmt.Println("Recently accessed keys:")
			for key := range recentKeys {
				fmt.Println("  - ", key)
			}

		default:
			fmt.Printf("Unknown command: %s\n", command)
			fmt.Println("Type 'help' to see available commands")
		}
	}

}
