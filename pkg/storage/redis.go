package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	client *redis.Client
}

type Data struct {
	Value     []byte    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

func NewStore(addr string) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Store{client: client}, nil
}

func (s *Store) Put(ctx context.Context, key string, value []byte, timestamp time.Time) error {
	data := Data{
		Value:     value,
		Timestamp: timestamp,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	err = s.client.HSet(ctx, key, "data", jsonData).Err()
	if err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (value []byte, timestamp time.Time, err error) {
	jsonData, err := s.client.HGet(ctx, key, "data").Result()
	if err == redis.Nil {
		// Key not found - return empty value instead of error for more graceful handling
		return []byte{}, time.Time{}, nil
	} else if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to get key: %w", err)
	}

	var data Data
	err = json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return data.Value, data.Timestamp, nil
}

// Client returns the underlying Redis client
func (s *Store) Client() *redis.Client {
	return s.client
}
