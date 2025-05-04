package node

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"no.cap/goddb/pkg/rpc"
)

// StartHTTPServer starts an HTTP server that provides a simple API for the node
func (n *Node) StartHTTPServer(port int) error {
	mux := http.NewServeMux()

	// GET /ping - health check
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	})

	// GET /put?key=<key>&value=<value> - put operation
	mux.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		value := r.URL.Query().Get("value")

		if key == "" || value == "" {
			http.Error(w, "Missing key or value parameter", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := &rpc.PutRequest{
			Key:       key,
			Value:     []byte(value),
			Timestamp: timestamppb.Now(),
		}

		resp, err := n.Put(ctx, req)
		if err != nil {
			http.Error(w, fmt.Sprintf("RPC error: %v", err), http.StatusInternalServerError)
			return
		}

		if resp.Status != rpc.StatusCode_OK {
			http.Error(w, fmt.Sprintf("Put failed: %s", resp.Message), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "OK: %s = %s", key, value)
	})

	// GET /get?key=<key> - get operation
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "Missing key parameter", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := &rpc.GetRequest{
			Key: key,
		}

		resp, err := n.Get(ctx, req)
		if err != nil {
			http.Error(w, fmt.Sprintf("RPC error: %v", err), http.StatusInternalServerError)
			return
		}

		if resp.Status == rpc.StatusCode_NOT_FOUND {
			http.Error(w, fmt.Sprintf("Key not found: %s", key), http.StatusNotFound)
			return
		}

		if resp.Status != rpc.StatusCode_OK {
			http.Error(w, fmt.Sprintf("Get failed: status=%v", resp.Status), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, string(resp.Value))
	})

	// GET /hintcount - get the number of hints this node is storing
	mux.HandleFunc("/hintcount", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		count, err := n.store.CountHints(context.Background())
		if err != nil {
			http.Error(w, fmt.Sprintf("Error counting hints: %v", err), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "%d", count)
	})

	// GET /info - get basic node info
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		fmt.Fprintf(w, "Node ID: %s\nIP: %s\nPort: %d\n", 
			n.meta.NodeId, 
			n.meta.Ip, 
			n.meta.Port)
	})

	// Start the HTTP server in a goroutine
	go func() {
		addr := fmt.Sprintf(":%d", port)
		err := http.ListenAndServe(addr, mux)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Printf("HTTP API server started on port %d", port)
	return nil
} 