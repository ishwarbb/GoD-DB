package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"no.cap/goddb/pkg/client"
)

func main() {
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

	// Print welcome message and help
	printHelp()

	// Interactive terminal loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nGoD-DB> ")
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

			ts, err := client.Put(context.Background(), key, []byte(value))
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Stored key '%s' with value '%s'\n", key, value)
				fmt.Printf("Timestamp: %v\n", ts)
				recentKeys[key] = true
			}

		case "get", "g":
			if len(args) != 2 {
				fmt.Println("Usage: get <key>")
				continue
			}
			key := args[1]

			value, ts, err := client.Get(context.Background(), key)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Key: %s\n", key)
				fmt.Printf("Value: %s\n", string(value))
				fmt.Printf("Timestamp: %v\n", ts)
				recentKeys[key] = true
			}

		case "list", "ls", "l":
			if len(recentKeys) == 0 {
				fmt.Println("No keys have been accessed yet")
			} else {
				fmt.Println("Recently accessed keys:")
				for key := range recentKeys {
					fmt.Printf("  - %s\n", key)
				}
			}

		case "help", "h", "?":
			printHelp()

		case "exit", "quit", "q":
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Printf("Unknown command: %s\n", command)
			fmt.Println("Type 'help' to see available commands")
		}
	}
}

func printHelp() {
	fmt.Println("\n=== GoD-DB Client Help ===")
	fmt.Println("Available commands:")
	fmt.Println("  put <key> <value>    Store a key-value pair")
	fmt.Println("  get <key>            Retrieve a value by key")
	fmt.Println("  list                 Show recently accessed keys")
	fmt.Println("  help                 Show this help message")
	fmt.Println("  exit                 Exit the client")
	fmt.Println("Short command aliases: p (put), g (get), l (list), h (help), q (quit)")
}
