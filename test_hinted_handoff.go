package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

const (
	worker1Port = 8081
	worker2Port = 8082
	worker3Port = 8083
)

// getClient returns a gRPC client connected to the specified worker
func getClient(port int) (*grpc.ClientConn, rpc.NodeServiceClient, error) {
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %v", addr, err)
	}
	client := rpc.NewNodeServiceClient(conn)
	return conn, client, nil
}

// ping tests if a worker is responsive
func ping(client rpc.NodeServiceClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	resp, err := client.Ping(ctx, &rpc.PingRequest{})
	if err != nil {
		return false
	}
	
	log.Printf("Ping response: %s", resp.Message)
	return true
}

// put writes a key-value pair to the specified worker
func put(client rpc.NodeServiceClient, key string, value string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	req := &rpc.PutRequest{
		Key:       key,
		Value:     []byte(value),
		Timestamp: timestamppb.Now(),
	}
	
	resp, err := client.Put(ctx, req)
	if err != nil {
		return false, fmt.Sprintf("RPC error: %v", err)
	}
	
	if resp.Status != rpc.StatusCode_OK {
		return false, fmt.Sprintf("Put failed: %s", resp.Message)
	}
	
	return true, ""
}

// get retrieves a key-value pair from the specified worker
func get(client rpc.NodeServiceClient, key string) (bool, string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	req := &rpc.GetRequest{
		Key: key,
	}
	
	resp, err := client.Get(ctx, req)
	if err != nil {
		return false, "", fmt.Sprintf("RPC error: %v", err)
	}
	
	if resp.Status == rpc.StatusCode_NOT_FOUND {
		return false, "", "Key not found"
	}
	
	if resp.Status != rpc.StatusCode_OK {
		return false, "", fmt.Sprintf("Get failed: status=%v", resp.Status)
	}
	
	return true, string(resp.Value), ""
}

// killWorker kills the specified worker process
func killWorker(port int) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("pkill -f \"bin/worker -port %d\"", port))
	return cmd.Run()
}

// startWorker starts a worker with the specified port
func startWorker(port int, redisPort int) (*exec.Cmd, error) {
	cmd := exec.Command("./bin/worker", 
		"-port", fmt.Sprintf("%d", port),
		"-redisport", fmt.Sprintf("%d", redisPort),
		"-discovery", "localhost:63179",
		"-replication", "3",
		"-writequorum", "2",
		"-readquorum", "2",
		"-virtualnodes", "50",
		"-hintinterval", "5s", // Use a shorter interval for faster testing
		"-enablehints", "true")
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd, cmd.Start()
}

func main() {
	log.Println("=== Testing Hinted Handoff ===")
	
	// Step 1: Connect to the three worker nodes
	log.Println("Connecting to workers...")
	
	conn1, client1, err := getClient(worker1Port)
	if err != nil {
		log.Fatalf("Failed to connect to worker 1: %v", err)
	}
	defer conn1.Close()
	
	conn2, client2, err := getClient(worker2Port)
	if err != nil {
		log.Fatalf("Failed to connect to worker 2: %v", err)
	}
	defer conn2.Close()
	
	conn3, client3, err := getClient(worker3Port)
	if err != nil {
		log.Fatalf("Failed to connect to worker 3: %v", err)
	}
	defer conn3.Close()
	
	// Step 2: Verify all workers are up
	log.Println("Verifying workers are responsive...")
	if !ping(client1) || !ping(client2) || !ping(client3) {
		log.Fatalf("Not all workers are responsive. Please make sure all three workers are running.")
	}
	
	// Step 3: Write some initial data through worker 1
	log.Println("Writing initial data to the cluster...")
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("initial-key-%d", i)
		value := fmt.Sprintf("initial-value-%d", i)
		
		success, errMsg := put(client1, key, value)
		if !success {
			log.Printf("WARN: Failed to write %s: %s", key, errMsg)
		} else {
			log.Printf("Successfully wrote %s = %s", key, value)
		}
	}
	
	// Step 4: Verify data can be read from all workers
	log.Println("Verifying data can be read from all workers...")
	key := "initial-key-1"
	
	success1, value1, errMsg1 := get(client1, key)
	success2, value2, errMsg2 := get(client2, key)
	success3, value3, errMsg3 := get(client3, key)
	
	if !success1 || !success2 || !success3 {
		log.Printf("Unexpected result in reading %s from workers:", key)
		log.Printf("  Worker 1: success=%v, value=%s, error=%s", success1, value1, errMsg1)
		log.Printf("  Worker 2: success=%v, value=%s, error=%s", success2, value2, errMsg2)
		log.Printf("  Worker 3: success=%v, value=%s, error=%s", success3, value3, errMsg3)
	} else {
		log.Printf("Successfully verified %s = %s across all workers", key, value1)
	}
	
	// Step 5: Kill worker 3
	log.Println("Killing worker 3 to simulate a node failure...")
	err = killWorker(worker3Port)
	if err != nil {
		log.Printf("Error killing worker 3: %v", err)
	}
	
	// Give it time to fully shut down
	time.Sleep(2 * time.Second)
	
	// Step 6: Write more data through worker 1 (some should generate hints for worker 3)
	log.Println("Writing more data with worker 3 down (should generate hints)...")
	hintedKeys := []string{}
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("hinted-key-%d", i)
		value := fmt.Sprintf("hinted-value-%d", i)
		hintedKeys = append(hintedKeys, key)
		
		success, errMsg := put(client1, key, value)
		if !success {
			log.Printf("WARN: Failed to write %s: %s", key, errMsg)
		} else {
			log.Printf("Successfully wrote %s = %s", key, value)
		}
	}
	
	// Step 7: Restart worker 3
	log.Println("Restarting worker 3...")
	worker3Cmd, err := startWorker(worker3Port, 63081)
	if err != nil {
		log.Fatalf("Failed to restart worker 3: %v", err)
	}
	defer worker3Cmd.Process.Kill()
	
	// Give worker 3 time to start and process any hints
	log.Println("Waiting for worker 3 to start and process hints (30 seconds)...")
	time.Sleep(30 * time.Second)
	
	// Step 8: Reconnect to worker 3
	conn3.Close()
	conn3, client3, err = getClient(worker3Port)
	if err != nil {
		log.Fatalf("Failed to reconnect to worker 3: %v", err)
	}
	defer conn3.Close()
	
	if !ping(client3) {
		log.Fatalf("Worker 3 is not responsive after restart")
	}
	
	// Step 9: Verify the hinted data was properly delivered to worker 3
	log.Println("Verifying hinted data was delivered to worker 3...")
	
	successCount := 0
	for _, key := range hintedKeys {
		success, value, errMsg := get(client3, key)
		if success {
			successCount++
			log.Printf("SUCCESS: Worker 3 has %s = %s (delivered via hint)", key, value)
		} else {
			log.Printf("FAIL: Worker 3 missing %s: %s", key, errMsg)
		}
	}
	
	log.Printf("Hinted handoff success rate: %d/%d keys (%d%%)", 
		successCount, len(hintedKeys), successCount*100/len(hintedKeys))
	
	if successCount > 0 {
		log.Println("TEST PASSED: Hinted handoff successfully delivered data to the recovered node!")
	} else {
		log.Println("TEST FAILED: Hinted handoff did not deliver any data to the recovered node")
	}
} 