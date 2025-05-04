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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	
	"no.cap/goddb/pkg/rpc"
)

// BenchmarkClient is a simplified client for performance testing
type BenchmarkClient struct {
	conn      *grpc.ClientConn
	rpcClient rpc.NodeServiceClient
	serverAddr string
}

// NewBenchmarkClient creates a new benchmark client
func NewBenchmarkClient(serverAddr string) (*BenchmarkClient, error) {
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", serverAddr, err)
	}
	
	return &BenchmarkClient{
		conn:      conn,
		rpcClient: rpc.NewNodeServiceClient(conn),
		serverAddr: serverAddr,
	}, nil
}

// Close the client connection
func (c *BenchmarkClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Put a key-value pair with retries
func (c *BenchmarkClient) Put(ctx context.Context, key string, value []byte) error {
	req := &rpc.PutRequest{
		Key:   key,
		Value: value,
	}
	
	// Try up to 3 times with backoff
	for retry := 0; retry < 3; retry++ {
		// Add timeout to context
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := c.rpcClient.Put(timeoutCtx, req)
		cancel()
		
		if err == nil {
			// Check response status
			if resp.Status == rpc.StatusCode_OK {
				return nil
			} else if resp.Status == rpc.StatusCode_WRONG_NODE && resp.CorrectNode != nil {
				// Could implement redirect logic here if needed
				return fmt.Errorf("wrong node, should contact: %s", resp.CorrectNode.NodeId)
			} else {
				return fmt.Errorf("operation failed: %v", resp.Status)
			}
		}
		
		// Check if we should retry based on the error
		if shouldRetry(err) && retry < 2 {
			backoff := time.Duration(50*(retry+1)) * time.Millisecond
			time.Sleep(backoff)
			continue
		}
		
		return err
	}
	
	return fmt.Errorf("failed after retries")
}

// Get a value by key with retries
func (c *BenchmarkClient) Get(ctx context.Context, key string) ([]byte, error) {
	req := &rpc.GetRequest{
		Key: key,
	}
	
	// Try up to 3 times with backoff
	for retry := 0; retry < 3; retry++ {
		// Add timeout to context
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := c.rpcClient.Get(timeoutCtx, req)
		cancel()
		
		if err == nil {
			if resp.Status == rpc.StatusCode_OK {
				return resp.Value, nil
			} else if resp.Status == rpc.StatusCode_NOT_FOUND {
				// For benchmark purposes, not found is expected for some gets
				// and shouldn't be treated as an error
				return nil, nil
			} else if resp.Status == rpc.StatusCode_WRONG_NODE && resp.CorrectNode != nil {
				// Could implement redirect logic here if needed
				return nil, fmt.Errorf("wrong node, should contact: %s", resp.CorrectNode.NodeId)
			} else {
				return nil, fmt.Errorf("operation failed: %v", resp.Status)
			}
		}
		
		// Check if we should retry based on the error
		if shouldRetry(err) && retry < 2 {
			backoff := time.Duration(50*(retry+1)) * time.Millisecond
			time.Sleep(backoff)
			continue
		}
		
		return nil, err
	}
	
	return nil, fmt.Errorf("failed after retries")
}

// Helper to determine if an error should trigger a retry
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	
	// Check if it's a gRPC status error
	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted:
			return true
		default:
			return false
		}
	}
	
	// For connection errors, timeouts, etc.
	return true
}

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
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	silent := flag.Bool("silent", false, "suppress all output (for pre-population)")
	flag.Parse()

	// Set log level
	if !*verbose {
		log.SetOutput(&nullWriter{})
	}

	// Parse server addresses
	servers := strings.Split(*serverAddrs, ",")
	
	if !*silent {
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
	}
	
	// Seed the random generator
	rand.Seed(time.Now().UnixNano())
	
	results := runBenchmark(*numClients, *numRequests, *readPercentage, servers, *testDuration, *valueSize, *reportInterval, *silent)
	
	// Print results
	if !*silent {
		fmt.Printf("\n====== Benchmark Results ======\n")
		fmt.Printf("Duration: %v\n", results.Duration)
		fmt.Printf("Total Operations: %d\n", results.TotalOperations)
		fmt.Printf("Operations/second: %.2f\n", results.OpsPerSecond)
		fmt.Printf("Average Latency: %v\n", results.AverageLatency)
		fmt.Printf("Failed Operations: %d (%.2f%%)\n", results.FailedOperations, results.FailureRate)
	}
}

// nullWriter for discarding log output
type nullWriter struct{}
func (nw *nullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func runBenchmark(numClients, requestsPerClient, readPercentage int, servers []string, duration time.Duration, valueSize int, reportInterval time.Duration, silent bool) BenchmarkResults {
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
				if !silent {
					fmt.Printf("[%v] Operations: %d, Rate: %.2f ops/sec, Failures: %d\n", 
						elapsed.Round(time.Second), currentOps, opsPerSec, atomic.LoadInt64(&failures))
				}
				prevOps = currentOps
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	
	// Initialize clients
	clientPool := make([]*BenchmarkClient, numClients)
	for i := 0; i < numClients; i++ {
		// Connect to different servers for load balancing
		serverAddr := servers[i%len(servers)]
		client, err := NewBenchmarkClient(serverAddr)
		if err != nil {
			log.Printf("Client %d failed to connect to %s: %v", i, serverAddr, err)
			clientPool[i] = nil  // Mark as unavailable
		} else {
			clientPool[i] = client
		}
	}
	
	// Make sure we have at least one connected client
	connectedClients := 0
	for _, client := range clientPool {
		if client != nil {
			connectedClients++
		}
	}
	
	if connectedClients == 0 {
		log.Fatalf("All clients failed to connect. Aborting benchmark.")
		return BenchmarkResults{}
	}
	
	// Prepare keys
	// First do a round of puts to ensure keys exist
	if readPercentage > 0 {
		log.Println("Performing initial PUT operations to populate keys")
		for i := 0; i < min(100, numClients*requestsPerClient/10); i++ {
			key := fmt.Sprintf("bench-key-%d", i)
			// Find a working client
			for _, client := range clientPool {
				if client != nil {
					client.Put(context.Background(), key, randomValue)
					break
				}
			}
		}
	}
	
	// Launch benchmark workers
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			
			client := clientPool[clientID]
			if client == nil {
				return  // Skip this worker if client init failed
			}
			defer client.Close()
			
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
				key := fmt.Sprintf("bench-key-%d", rand.Intn(1000))
				
				opStart := time.Now()
				var opErr error
				
				if isRead {
					// GET operation
					_, opErr = client.Get(context.Background(), key)
				} else {
					// PUT operation with random value
					opErr = client.Put(context.Background(), key, randomValue)
				}
				
				// Record metrics
				opLatency := time.Since(opStart)
				atomic.AddInt64(&latencySum, opLatency.Nanoseconds())
				atomic.AddInt64(&latencyCount, 1)
				atomic.AddInt64(&totalOps, 1)
				
				if opErr != nil {
					atomic.AddInt64(&failures, 1)
					log.Printf("Operation error: %v", opErr)
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
	elapsed := time.Since(startTime)
	ops := atomic.LoadInt64(&totalOps)
	failed := atomic.LoadInt64(&failures)
	opsPerSecond := float64(ops) / elapsed.Seconds()
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
		Duration:        elapsed,
		TotalOperations: ops,
		FailedOperations: failed,
		OpsPerSecond:    opsPerSecond,
		FailureRate:     failureRate,
		AverageLatency:  avgLatency,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
} 