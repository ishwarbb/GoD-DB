package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis ports
	redis1Port = 63079
	redis2Port = 63080
	redis3Port = 63081
	
	// HTTP API ports
	worker1HTTPPort = 9081
	worker2HTTPPort = 9082
	worker3HTTPPort = 9083

	// Hint prefix in Redis
	hintPrefix = "hint:"

	// ANSI color codes for output
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
)

type HintStats struct {
	NodeID         string
	TotalHints     int
	TargetNodeMap  map[string]int
	HintsByKey     map[string]string
	Redis          *redis.Client
}

func clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func connectRedis(port int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("localhost:%d", port),
	})
}

func getNodeStatus(port int) string {
	// Try pinging the node's HTTP API
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/ping", port))
	if err != nil || resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("%sOFFLINE%s", colorRed, colorReset)
	}
	
	defer resp.Body.Close()
	return fmt.Sprintf("%sONLINE%s", colorGreen, colorReset)
}

func checkHints(client *redis.Client, nodeID string) HintStats {
	ctx := context.Background()
	
	stats := HintStats{
		NodeID:        nodeID,
		TotalHints:    0,
		TargetNodeMap: make(map[string]int),
		HintsByKey:    make(map[string]string),
		Redis:         client,
	}
	
	// Get all hint keys
	keys, err := client.Keys(ctx, hintPrefix+"*").Result()
	if err != nil {
		fmt.Printf("Error getting hint keys: %v\n", err)
		return stats
	}
	
	stats.TotalHints = len(keys)
	
	// Process each hint
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) < 3 {
			continue
		}
		
		targetNode := parts[1]
		originalKey := strings.Join(parts[2:], ":")
		
		stats.TargetNodeMap[targetNode]++
		stats.HintsByKey[originalKey] = targetNode
	}
	
	return stats
}

func displayDashboard(node1Stats, node2Stats, node3Stats HintStats, historyLog []string) {
	clearScreen()
	
	// Get node status
	node1Status := getNodeStatus(worker1HTTPPort)
	node2Status := getNodeStatus(worker2HTTPPort)
	node3Status := getNodeStatus(worker3HTTPPort)
	
	fmt.Println("========== HINTED HANDOFF MONITORING DASHBOARD ==========")
	fmt.Println()
	
	// Node status section
	fmt.Printf("NODE STATUS\n")
	fmt.Printf("-------------------------------------\n")
	fmt.Printf("Worker 1 (HTTP port %d): %s\n", worker1HTTPPort, node1Status)
	fmt.Printf("Worker 2 (HTTP port %d): %s\n", worker2HTTPPort, node2Status)
	fmt.Printf("Worker 3 (HTTP port %d): %s\n", worker3HTTPPort, node3Status)
	fmt.Println()
	
	// Hint statistics
	fmt.Printf("HINT STATISTICS\n")
	fmt.Printf("-------------------------------------\n")
	fmt.Printf("Node 1 (Redis port %d): %s%d hints%s\n", redis1Port, colorYellow, node1Stats.TotalHints, colorReset)
	for target, count := range node1Stats.TargetNodeMap {
		fmt.Printf("  → %s%d hints%s for node %s\n", colorCyan, count, colorReset, target)
	}
	
	fmt.Printf("Node 2 (Redis port %d): %s%d hints%s\n", redis2Port, colorYellow, node2Stats.TotalHints, colorReset)
	for target, count := range node2Stats.TargetNodeMap {
		fmt.Printf("  → %s%d hints%s for node %s\n", colorCyan, count, colorReset, target)
	}
	
	fmt.Printf("Node 3 (Redis port %d): %s%d hints%s\n", redis3Port, colorYellow, node3Stats.TotalHints, colorReset)
	for target, count := range node3Stats.TargetNodeMap {
		fmt.Printf("  → %s%d hints%s for node %s\n", colorCyan, count, colorReset, target)
	}
	fmt.Println()
	
	// Activity log
	fmt.Printf("ACTIVITY LOG\n")
	fmt.Printf("-------------------------------------\n")
	
	maxEntries := 10
	startIdx := 0
	if len(historyLog) > maxEntries {
		startIdx = len(historyLog) - maxEntries
	}
	
	for i := startIdx; i < len(historyLog); i++ {
		fmt.Println(historyLog[i])
	}
	fmt.Println()
	
	fmt.Println("Press Ctrl+C to exit")
}

func checkHintDelivery(client *redis.Client, hints map[string]string) map[string]bool {
	ctx := context.Background()
	delivered := make(map[string]bool)
	
	for key := range hints {
		exists, err := client.Exists(ctx, key).Result()
		if err != nil {
			continue
		}
		
		delivered[key] = exists > 0
	}
	
	return delivered
}

func main() {
	// Connect to Redis instances
	redis1 := connectRedis(redis1Port)
	redis2 := connectRedis(redis2Port)
	redis3 := connectRedis(redis3Port)
	
	defer redis1.Close()
	defer redis2.Close()
	defer redis3.Close()
	
	// History log for activity
	historyLog := []string{
		fmt.Sprintf("%s[%s] Monitoring started%s", colorBlue, time.Now().Format("15:04:05"), colorReset),
	}
	
	// Track delivered hints
	prevHints := make(map[string]string)
	
	// Main monitoring loop
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Check hints on each node
			node1Stats := checkHints(redis1, "node1")
			node2Stats := checkHints(redis2, "node2")
			node3Stats := checkHints(redis3, "node3")
			
			// Track hint creation/delivery
			allCurrentHints := make(map[string]string)
			
			// Add all current hints to the map
			for k, v := range node1Stats.HintsByKey {
				allCurrentHints[k] = v
			}
			for k, v := range node2Stats.HintsByKey {
				allCurrentHints[k] = v
			}
			for k, v := range node3Stats.HintsByKey {
				allCurrentHints[k] = v
			}
			
			// Check for new hints
			for key, target := range allCurrentHints {
				if _, exists := prevHints[key]; !exists {
					// This is a new hint
					hostNode := "unknown"
					if _, ok := node1Stats.HintsByKey[key]; ok {
						hostNode = "node1"
					} else if _, ok := node2Stats.HintsByKey[key]; ok {
						hostNode = "node2"
					} else if _, ok := node3Stats.HintsByKey[key]; ok {
						hostNode = "node3"
					}
					
					logEntry := fmt.Sprintf("%s[%s] New hint created on %s for key '%s' targeting node %s%s", 
						colorYellow, time.Now().Format("15:04:05"), hostNode, key, target, colorReset)
					historyLog = append(historyLog, logEntry)
				}
			}
			
			// Check for delivered hints (hints that were in prevHints but not in allCurrentHints)
			for key, target := range prevHints {
				if _, exists := allCurrentHints[key]; !exists {
					// This hint is no longer present, it might have been delivered or expired
					
					// Check if the data exists on the target node
					var targetHTTPPort int
					switch target {
					case "localhost:8081":
						targetHTTPPort = worker1HTTPPort
					case "localhost:8082":
						targetHTTPPort = worker2HTTPPort
					case "localhost:8083":
						targetHTTPPort = worker3HTTPPort
					default:
						continue
					}
					
					// Try to get the key from the target node's HTTP API
					resp, err := http.Get(fmt.Sprintf("http://localhost:%d/get?key=%s", targetHTTPPort, key))
					if err == nil && resp.StatusCode == http.StatusOK {
						resp.Body.Close()
						logEntry := fmt.Sprintf("%s[%s] Hint for key '%s' successfully delivered to %s%s", 
							colorGreen, time.Now().Format("15:04:05"), key, target, colorReset)
						historyLog = append(historyLog, logEntry)
					}
				}
			}
			
			// Update previous hints
			prevHints = allCurrentHints
			
			// Display dashboard
			displayDashboard(node1Stats, node2Stats, node3Stats, historyLog)
		}
	}
} 