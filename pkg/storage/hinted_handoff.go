package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

const (
	// Prefix for hinted handoff keys in Redis
	hintPrefix = "hint:"
	// TTL for hints (default: 24 hours)
	defaultHintTTL = 24 * time.Hour
)

// StoreHint stores a hint for a temporarily unavailable node
func (s *Store) StoreHint(ctx context.Context, targetNodeID string, key string, value []byte, timestamp time.Time) error {
	// Create a unique hint key with the format hint:targetNodeID:key
	hintKey := fmt.Sprintf("%s%s:%s", hintPrefix, targetNodeID, key)
	
	// Create the hint data
	hint := rpc.HintedHandoffEntry{
		Key:           key,
		Value:         value,
		Timestamp:     timestamppb.New(timestamp),
		TargetNodeId:  targetNodeID,
		HintCreatedAt: timestamppb.New(time.Now()),
	}
	
	// Serialize the hint
	hintData, err := json.Marshal(hint)
	if err != nil {
		return fmt.Errorf("failed to marshal hint data: %w", err)
	}
	
	// Store the hint with TTL
	err = s.client.Set(ctx, hintKey, hintData, defaultHintTTL).Err()
	if err != nil {
		return fmt.Errorf("failed to store hint: %w", err)
	}
	
	log.Printf("Stored hint for node %s, key %s", targetNodeID, key)
	return nil
}

// GetHintsForNode retrieves all hints for a specific node
func (s *Store) GetHintsForNode(ctx context.Context, nodeID string) ([]*rpc.HintedHandoffEntry, error) {
	// Pattern to match all hints for this node
	pattern := fmt.Sprintf("%s%s:*", hintPrefix, nodeID)
	
	// Scan for matching keys
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to scan for hints: %w", err)
	}
	
	hints := make([]*rpc.HintedHandoffEntry, 0, len(keys))
	
	// Retrieve each hint
	for _, key := range keys {
		hintData, err := s.client.Get(ctx, key).Result()
		if err == redis.Nil {
			// Key might have expired
			continue
		} else if err != nil {
			log.Printf("Error retrieving hint %s: %v", key, err)
			continue
		}
		
		var hint rpc.HintedHandoffEntry
		if err := json.Unmarshal([]byte(hintData), &hint); err != nil {
			log.Printf("Error unmarshaling hint %s: %v", key, err)
			continue
		}
		
		hints = append(hints, &hint)
	}
	
	return hints, nil
}

// DeleteHint removes a hint after it has been successfully processed
func (s *Store) DeleteHint(ctx context.Context, targetNodeID string, key string) error {
	hintKey := fmt.Sprintf("%s%s:%s", hintPrefix, targetNodeID, key)
	err := s.client.Del(ctx, hintKey).Err()
	if err != nil {
		return fmt.Errorf("failed to delete hint: %w", err)
	}
	return nil
}

// CountHints counts the number of hints stored for all target nodes
func (s *Store) CountHints(ctx context.Context) (int, error) {
	// Use Redis KEYS command to get all hint keys
	keys, err := s.Client().Keys(ctx, hintPrefix+"*").Result()
	if err != nil {
		return 0, fmt.Errorf("failed to count hints: %w", err)
	}
	
	return len(keys), nil
} 