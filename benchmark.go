package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"no.cap/goddb/pkg/client"
)

type BenchmarkResults struct {
	Duration        time.Duration
	TotalOperations int64
	FailedOperations int64
	OpsPerSecond    float64
	FailureRate     float64
	AverageLatency  time.Duration
}

func main() {
	numClients := flag.Int("clients", 10, "number of concurrent clients")
	numRequests := flag.Int("requests", 1000, "number of requests per client")
	readPercentage := flag.Int("reads", 50, "percentage of read operations (0-100)")
	serverAddrs := flag.String("servers", "localhost:8081,localhost:8082,localhost:8083", "comma-separated list of server addresses")
	testDuration := flag.Duration("duration", 0, "test duration (if specified, ignores numRequests)")
	valueSize := flag.Int("valuesize", 100, "size of values in bytes")
	reportInterval := flag.Duration("report", 5*time.Second, "progress report interval")
	flag.Parse()

	// Parse server addresses
	servers := strings.Split(*serverAddrs, ",")
	
	fmt.Printf("=== GoD-DB Benchmark ===\n")
	fmt.Printf("Servers: %s\n", *serverAddrs)
	fmt.Printf("Clients: %d\n", *numClients)
	if *testDuration > 0 {
		fmt.Printf("Duration: %v\n", *testDuration)
	} else {
		fmt.Printf("Requests per client: %d\n", *numRequests)
	}
	fmt.Printf("Read percentage: %d%%\n", *readPercentage)
	fmt.Printf("Value size: %d bytes\n", *valueSize)
	fmt.Println("Starting benchmark...")
	
	results := runBenchmark(*numClients, *numRequests, *readPercentage, servers, *testDuration, *valueSize, *reportInterval)
	
	// Print results
	fmt.Printf("\n====== Benchmark Results ======\n")
	fmt.Printf("Duration: %v\n", results.Duration)
	fmt.Printf("Total Operations: %d\n", results.TotalOperations)
	fmt.Printf("Operations/second: %.2f\n", results.OpsPerSecond)
	fmt.Printf("Average Latency: %v\n", results.AverageLatency)
	fmt.Printf("Failed Operations: %d (%.2f%%)\n", results.FailedOperations, results.FailureRate)
}

func runBenchmark(numClients, requestsPerClient, readPercentage int, servers []string, duration time.Duration, valueSize int, reportInterval time.Duration) BenchmarkResults {
	var wg sync.WaitGroup
	startTime := time.Now()
	endTime := startTime.Add(duration)
	useFixedDuration := duration > 0
	
	// Metrics
	var totalOps int64
	var failures int64
	var latencySum int64
	var latencyCount int64
	
	// Generate random value of specified size
	randomValue := make([]byte, valueSize)
	for i := range randomValue {
		randomValue[i] = byte(rand.Intn(26) + 97) // Random lowercase letters
	}
	
	// Channel to stop clients when using duration-based testing
	done := make(chan struct{})
	
	// Progress reporting
	ticker := time.NewTicker(reportInterval)
	go func() {
		prevOps := int64(0)
		for {
			select {
			case <-ticker.C:
				currentOps := atomic.LoadInt64(&totalOps)
				elapsed := time.Since(startTime)
				opsPerSec := float64(currentOps-prevOps) / reportInterval.Seconds()
				fmt.Printf("[%v] Operations: %d, Rate: %.2f ops/sec\n", 
					elapsed.Round(time.Second), currentOps, opsPerSec)
				prevOps = currentOps
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	
	// Launch clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			
			// Connect to random server for initial load balancing
			serverAddr := servers[clientID%len(servers)]
			c, err := client.NewClient(serverAddr, servers, 3, 2, 2)
			if err != nil {
				log.Printf("Client %d failed to connect: %v", clientID, err)
				return
			}
			defer c.Close()
			
			// Run operations
			op := 0
			for {
				if !useFixedDuration && op >= requestsPerClient {
					break
				}
				if useFixedDuration && time.Now().After(endTime) {
					break
				}
				
				// Determine operation type (read or write)
				isRead := rand.Intn(100) < readPercentage
				key := fmt.Sprintf("bench-key-%d-%d", clientID, op)
				
				opStart := time.Now()
				var opErr error
				
				if isRead {
					// GET operation
					_, _, opErr = c.Get(context.Background(), key)
				} else {
					// PUT operation with random value
					_, opErr = c.Put(context.Background(), key, randomValue)
				}
				
				// Record metrics
				opLatency := time.Since(opStart)
				atomic.AddInt64(&latencySum, opLatency.Nanoseconds())
				atomic.AddInt64(&latencyCount, 1)
				atomic.AddInt64(&totalOps, 1)
				
				if opErr != nil {
					atomic.AddInt64(&failures, 1)
				}
				
				op++
				
				// Add small random delay to prevent perfect synchronization
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}(i)
	}
	
	// Wait for all clients to complete
	wg.Wait()
	close(done)
	
	// Calculate results
	duration := time.Since(startTime)
	ops := atomic.LoadInt64(&totalOps)
	failed := atomic.LoadInt64(&failures)
	opsPerSecond := float64(ops) / duration.Seconds()
	failureRate := float64(0)
	if ops > 0 {
		failureRate = float64(failed) / float64(ops) * 100
	}
	
	// Calculate average latency
	avgLatency := time.Duration(0)
	if atomic.LoadInt64(&latencyCount) > 0 {
		avgLatency = time.Duration(atomic.LoadInt64(&latencySum) / atomic.LoadInt64(&latencyCount))
	}
	
	return BenchmarkResults{
		Duration:        duration,
		TotalOperations: ops,
		FailedOperations: failed,
		OpsPerSecond:    opsPerSecond,
		FailureRate:     failureRate,
		AverageLatency:  avgLatency,
	}
} 