package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

type Client struct {
	conn *grpc.ClientConn
	rpc  rpc.NodeServiceClient
}

func NewClient(addr string) (*Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("did not connect: %v", err)
	}

	c := rpc.NewNodeServiceClient(conn)
	return &Client{conn: conn, rpc: c}, nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) Get(ctx context.Context, key string) ([]byte, time.Time, error) {
	r, err := c.rpc.Get(ctx, &rpc.GetRequest{Key: key})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("could not get: %v", err)
	}

	switch r.Status {
	case rpc.StatusCode_OK:
		return r.Value, r.Timestamp.AsTime(), nil
	case rpc.StatusCode_NOT_FOUND:
		return nil, time.Time{}, fmt.Errorf("key not found")
	case rpc.StatusCode_WRONG_NODE:
		log.Printf("WRONG_NODE: key %s should be on %v", key, r.CorrectNode)
		return nil, time.Time{}, fmt.Errorf("wrong node")
	default:
		return nil, time.Time{}, fmt.Errorf("unknown error")
	}
}

func (c *Client) Put(ctx context.Context, key string, value []byte) (time.Time, error) {
	ts := timestamppb.Now()
	r, err := c.rpc.Put(ctx, &rpc.PutRequest{Key: key, Value: value, Timestamp: ts})
	if err != nil {
		return time.Time{}, fmt.Errorf("could not put: %v", err)
	}

	switch r.Status {
	case rpc.StatusCode_OK:
		return r.FinalTimestamp.AsTime(), nil
	case rpc.StatusCode_WRONG_NODE:
		log.Printf("WRONG_NODE: key %s should be on %v", key, r.CorrectNode)
		return time.Time{}, fmt.Errorf("wrong node")
	default:
		return time.Time{}, fmt.Errorf("unknown error")
	}
}
