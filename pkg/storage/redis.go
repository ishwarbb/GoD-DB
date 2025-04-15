package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"no.cap/goddb/pkg/vclock"
)

// Store represents a Redis-backed storage engine
type Store struct {
	client *redis.Client
}

// ValueWithClock combines a byte value with its vector clock
type ValueWithClock struct {
	Value       []byte              `json:"value"`
	VectorClock *vclock.VectorClock `json:"vector_clock"`
	LastAccess  time.Time           `json:"last_access"` // For TTL/expiry if needed
}

// NewStore creates a new Redis store connection
func NewStore(addr string) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Test connection
	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Store{
		client: client,
	}, nil
}

// Put stores a value with its vector clock
func (s *Store) Put(ctx context.Context, key string, value []byte, vc *vclock.VectorClock) error {
	data := ValueWithClock{
		Value:       value,
		VectorClock: vc,
		LastAccess:  time.Now(),
	}

	// Serialize to JSON
	bytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Store in Redis
	if err := s.client.Set(ctx, key, bytes, 0).Err(); err != nil {
		return fmt.Errorf("failed to store value in Redis: %w", err)
	}

	return nil
}

// Get retrieves a value and its vector clock
func (s *Store) Get(ctx context.Context, key string) ([]byte, *vclock.VectorClock, error) {
	// Get from Redis
	bytes, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil, fmt.Errorf("key not found: %s", key)
		}
		return nil, nil, fmt.Errorf("failed to get value from Redis: %w", err)
	}

	// Deserialize from JSON
	var data ValueWithClock
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	// Update last access time
	data.LastAccess = time.Now()
	updatedBytes, _ := json.Marshal(data)
	// Ignore error for the TTL update since it's not critical
	s.client.Set(ctx, key, updatedBytes, 0)

	return data.Value, data.VectorClock, nil
}

// Close closes the Redis connection
func (s *Store) Close() error {
	return s.client.Close()
}
