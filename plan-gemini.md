# DynamoMiniGo Reimplementation Plan

This document outlines the iterative plan for reimplementing the DynamoMini system (originally in Python) using Go.

## Iteration 1: Project Structure, Basic RPC, and Node/Client Stubs

*   **Goal:** Set up the Go project structure, define the initial RPC interface using gRPC, and create minimal runnable worker and client applications.
*   **Details:**
    1.  **Directory Structure:**
        ```
        DynamoMiniGo/
        ├── cmd/
        │   ├── worker/
        │   │   └── main.go      # Worker executable entry point
        │   └── client/
        │       └── main.go      # Client executable entry point
        ├── pkg/
        │   ├── node/
        │   │   ├── node.go      # Worker node logic
        │   │   └── rpc.go       # Worker RPC service implementation
        │   ├── client/
        │   │   └── client.go    # Client library logic
        │   ├── chash/
        │   │   └── ring.go      # Consistent Hashing implementation (stub)
        │   ├── rpc/
        │   │   ├── dynamo.proto # Protocol buffer definitions
        │   │   └── dynamo_grpc.pb.go # Generated gRPC code
        │   │   └── dynamo.pb.go # Generated protobuf code
        │   ├── config/
        │   │   └── config.go    # Configuration loading (stub)
        │   └── storage/
        │       └── redis.go     # Redis interaction (stub)
        ├── go.mod
        ├── go.sum
        └── plan-gemini.md
        ```
    2.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Define a basic `NodeService` with placeholder methods.
        *   Use `protoc` with the gRPC Go plugin to generate Go code (`*.pb.go`, `*_grpc.pb.go`).
        ```protobuf
        syntax = "proto3";

        package rpc;
        option go_package = "github.com/yourusername/DynamoMiniGo/pkg/rpc"; // Adjust path

        message NodeMeta {
          string node_id = 1; // e.g., hash of ip:port
          string ip = 2;
          int32 port = 3;
        }

        // Placeholder requests/responses
        message PingRequest {}
        message PingResponse {
          string message = 1;
        }

        service NodeService {
          rpc Ping(PingRequest) returns (PingResponse);
          // Add more RPCs in later iterations (Get, Put, Replicate, Gossip...)
        }
        ```
    3.  **Worker (`cmd/worker/main.go`, `pkg/node/node.go`, `pkg/node/rpc.go`):**
        *   `main.go`: Parses flags (e.g., `-port`, `-redisport`), creates a `node.Node` instance, starts a gRPC server exposing `NodeService`.
        *   `node.go`: Define `Node` struct (initially with just config like port, ID). Add `NewNode(...)` constructor.
        *   `rpc.go`: Implement the `NodeService` interface (initially just the `Ping` method returning "pong").
    4.  **Client (`cmd/client/main.go`, `pkg/client/client.go`):**
        *   `main.go`: Parses flags (e.g., `-server addr`), creates a `client.Client` instance, makes a test `Ping` call.
        *   `client.go`: Define `Client` struct. Add `NewClient(...)` constructor. Implement a `Ping(...)` method that connects to a worker via gRPC and calls its `Ping` RPC.
    5.  **Dependencies (`go.mod`):**
        *   Add `google.golang.org/grpc`, `google.golang.org/protobuf`, `github.com/go-redis/redis/v8` (or v9).
    6.  **Stubs (`pkg/chash/ring.go`, `pkg/storage/redis.go`, `pkg/config/config.go`):** Create empty files or structs as placeholders.

*   **Outcome:** A runnable worker that listens for RPC calls and a client that can ping it. Basic project layout is established.

## Iteration 2: Consistent Hashing and Basic GET/PUT

*   **Goal:** Implement consistent hashing logic, integrate Redis storage, and add basic (non-replicated) GET and PUT functionality.
*   **Details:**
    1.  **Consistent Hashing (`pkg/chash/ring.go`):**
        *   Define a `Ring` struct. This might hold a sorted list of node hashes and a map from hash to `rpc.NodeMeta`. Use `sync.RWMutex` for concurrent access.
        *   Implement `NewRing()` constructor.
        *   Implement `AddNode(node rpc.NodeMeta)`: Calculates the node's hash (e.g., using MD5 or SHA1 on `ip:port`), inserts the hash into the sorted list, and adds the node metadata to the map.
        *   Implement `RemoveNode(nodeID string)`: Removes the node hash and metadata.
        *   Implement `GetNode(key string) (string, error)`: Hashes the key, finds the next node hash clockwise on the ring using binary search (`sort.Search`), and returns the corresponding `nodeID` (the hash itself). Handle empty ring case.
    2.  **Redis Storage (`pkg/storage/redis.go`):**
        *   Define a `Store` struct holding a `*redis.Client`.
        *   Implement `NewStore(addr string)` constructor to connect to Redis.
        *   Implement `Put(ctx context.Context, key string, value []byte, timestamp time.Time) error`: Stores the value and timestamp. Consider storing value and timestamp together (e.g., in a Redis Hash or as a serialized struct). Use `HSET` for key->value and maybe a separate key or field for the timestamp.
        *   Implement `Get(ctx context.Context, key string) (value []byte, timestamp time.Time, err error)`: Retrieves the value and timestamp for a key. Handle `redis.Nil` error.
    3.  **Worker Node Enhancement (`pkg/node/node.go`):**
        *   Add `ring *chash.Ring` and `store *storage.Store` fields to the `Node` struct.
        *   Add node's own `meta rpc.NodeMeta`.
        *   Initialize these in `NewNode(...)`. Add the node itself to the ring.
        *   Modify `cmd/worker/main.go` to pass the Redis address and potentially node ID/IP/Port if not discoverable.
    4.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Add `GetRequest`, `GetResponse`, `PutRequest`, `PutResponse`.
        *   Include `key` (string), `value` (bytes), `timestamp` (e.g., `google.protobuf.Timestamp`), and context/metadata as needed.
        *   Define status codes (e.g., `OK`, `ERROR`, `NOT_FOUND`, `WRONG_NODE`).
        ```protobuf
        syntax = "proto3";

        package rpc;
        option go_package = "github.com/yourusername/DynamoMiniGo/pkg/rpc"; // Adjust path

        import "google/protobuf/timestamp.proto";

        message NodeMeta {
          string node_id = 1; // e.g., hash of ip:port
          string ip = 2;
          int32 port = 3;
          // Add version, load etc. later
        }

        enum StatusCode {
          OK = 0;
          ERROR = 1;
          NOT_FOUND = 2;
          WRONG_NODE = 3; // Indicate the request hit the wrong node
          // Add more status codes as needed (e.g., QUORUM_FAILED)
        }

        // Placeholder requests/responses
        message PingRequest {}
        message PingResponse {
          string message = 1;
        }

        message GetRequest {
          string key = 1;
          // Add context/options later (e.g., Read preference R)
        }

        message GetResponse {
          StatusCode status = 1;
          bytes value = 2;
          google.protobuf.Timestamp timestamp = 3;
          NodeMeta coordinator = 4; // Node that processed the request
        }

        message PutRequest {
          string key = 1;
          bytes value = 2;
          google.protobuf.Timestamp timestamp = 3; // Client sets this initially
          // Add context/options later (e.g., Write preference W)
        }

        message PutResponse {
          StatusCode status = 1;
          NodeMeta coordinator = 2; // Node that processed the request
          google.protobuf.Timestamp final_timestamp = 3; // Timestamp stored
        }

        service NodeService {
          rpc Ping(PingRequest) returns (PingResponse);
          rpc Get(GetRequest) returns (GetResponse);
          rpc Put(PutRequest) returns (PutResponse);
          // Add more RPCs in later iterations (ReplicatePut, Gossip...)
        }
        ```
        *   Regenerate Go code using `protoc`.
    5.  **Worker RPC Implementation (`pkg/node/rpc.go`):**
        *   Implement the `Get` RPC:
            *   Determine the expected responsible node ID using `node.ring.GetNode(req.Key)`.
            *   If the current node's ID (`node.meta.NodeId`) matches the expected ID:
                *   Call `node.store.Get(ctx, req.Key)`.
                *   Return `GetResponse` with `OK` status, value, timestamp, and self as coordinator. Handle `NOT_FOUND` from store.
            *   If IDs don't match:
                *   Return `GetResponse` with `WRONG_NODE` status and self as coordinator. (Client will handle retry/redirect later).
        *   Implement the `Put` RPC:
            *   Determine the expected responsible node ID using `node.ring.GetNode(req.Key)`.
            *   If the current node's ID matches the expected ID:
                *   Call `node.store.Put(ctx, req.Key, req.Value, req.Timestamp.AsTime())`.
                *   Return `PutResponse` with `OK` status, self as coordinator, and the stored timestamp. Handle errors from store.
            *   If IDs don't match:
                *   Return `PutResponse` with `WRONG_NODE` status and self as coordinator.
    6.  **Client Enhancement (`pkg/client/client.go`):**
        *   Implement `Get(ctx context.Context, key string)`:
            *   Connect to a *known* worker node (config).
            *   Call the worker's `Get` RPC.
            *   Handle the response: OK, NOT_FOUND, ERROR. If `WRONG_NODE`, log it for now (actual redirection in next iteration).
        *   Implement `Put(ctx context.Context, key string, value []byte)`:
            *   Connect to a known worker.
            *   Generate a timestamp (`timestamppb.Now()`).
            *   Call the worker's `Put` RPC.
            *   Handle the response: OK, ERROR. If `WRONG_NODE`, log it for now.

*   **Outcome:** A client can PUT/GET a key-value pair to a single worker, which handles it correctly based on consistent hashing (if it's the designated node). Storage is persistent in Redis. Still no replication or node discovery/routing.

## Iteration 3: Client-Side Routing & Preference Lists

*   **Goal:** Enable the client to discover the correct node for a key and handle `WRONG_NODE` responses. Implement the concept of a preference list on the worker.
*   **Details:**
    1.  **Consistent Hashing Enhancements (`pkg/chash/ring.go`):**
        *   Implement `GetN(key string, n int) ([]string, error)`: Hashes the key, finds the primary node using `GetNode`, and then walks the ring clockwise to find the next `n-1` unique node IDs. Returns the list of `n` node IDs (hashes). Handle cases where `n` is larger than the number of nodes in the ring.
        *   Implement `GetAllNodes() []rpc.NodeMeta`: Returns a slice of all node metadata currently in the ring. Useful for client cache population.
        *   Implement `GetNodeMeta(nodeID string) (rpc.NodeMeta, bool)`: Returns the metadata for a specific node ID.
    2.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Add RPC for fetching routing info: `GetPreferenceListRequest`, `GetPreferenceListResponse`.
        *   `GetPreferenceListRequest`: Contains the `key` and `n` (replication factor).
        *   `GetPreferenceListResponse`: Contains `status`, a list of `NodeMeta` (`repeated NodeMeta preference_list`), and the `coordinator` `NodeMeta`.
        *   Modify `GetResponse` and `PutResponse`: If status is `WRONG_NODE`, include the `NodeMeta` of the node the worker *thinks* is correct (`optional NodeMeta correct_node = 5;`).
        ```protobuf
        // ... (NodeMeta, StatusCode, Ping, existing Get/Put definitions)

        message GetPreferenceListRequest {
          string key = 1;
          int32 n = 2; // How many nodes needed (e.g., replication factor)
        }

        message GetPreferenceListResponse {
          StatusCode status = 1;
          repeated NodeMeta preference_list = 2; // List of N nodes responsible
          NodeMeta coordinator = 3; // Node that processed this request
        }

        // Modify GetResponse
        message GetResponse {
          StatusCode status = 1;
          bytes value = 2;
          google.protobuf.Timestamp timestamp = 3;
          NodeMeta coordinator = 4;
          optional NodeMeta correct_node = 5; // If status == WRONG_NODE
        }

        // Modify PutResponse
        message PutResponse {
          StatusCode status = 1;
          NodeMeta coordinator = 2;
          google.protobuf.Timestamp final_timestamp = 3;
          optional NodeMeta correct_node = 5; // If status == WRONG_NODE
        }

        service NodeService {
          // ... (Ping, Get, Put)
          rpc GetPreferenceList(GetPreferenceListRequest) returns (GetPreferenceListResponse);
          // Add ReplicatePut, Gossip later
        }
        ```
        *   Regenerate Go code.
    3.  **Worker RPC Implementation (`pkg/node/rpc.go`):**
        *   Implement `GetPreferenceList`:
            *   Use `node.ring.GetN(req.Key, int(req.N))` to get the list of `n` node IDs (hashes).
            *   Look up the `NodeMeta` for each ID from the ring's internal map (`node.ring.GetNodeMeta`).
            *   Return `GetPreferenceListResponse` with `OK` status, the list of found `NodeMeta`, and `node.meta` as coordinator. Handle errors from `GetN` (e.g., ring empty, not enough nodes).
        *   Modify `Get` and `Put` RPC handlers:
            *   When returning `WRONG_NODE`, use `node.ring.GetNode(req.Key)` to find the presumed correct node ID.
            *   Look up its `NodeMeta` using `node.ring.GetNodeMeta(correctNodeID)`.
            *   Include the found `NodeMeta` in the `correct_node` field of the response.
    4.  **Client Enhancements (`pkg/client/client.go`):**
        *   Add cache fields: `nodeCache *NodeCache` (needs its own struct with mutex, map `nodeID -> NodeMeta`, potentially TTL logic).
        *   Add config fields: `seedNodes []string`, `replicationFactorN int`, `readQuorumR int`, `writeQuorumW int`.
        *   Add a connection manager: `connMgr *ConnectionManager` (needs its own struct/interface for getting/managing gRPC `ClientConn`s, possibly with pooling).
        *   Implement `getNodeClient(ctx context.Context, nodeAddr string) (rpc.NodeServiceClient, error)` using the `connMgr`.
        *   Implement `resolveCoordinator(ctx context.Context, key string) (*rpc.NodeMeta, error)`:
            *   Check local cache (e.g., `key -> primaryNodeID -> NodeMeta`).
            *   If miss or stale, call `fetchPreferenceList` (see below) and update cache.
            *   Return the primary node's `NodeMeta`.
        *   Implement `fetchPreferenceList(ctx context.Context, key string) ([]*rpc.NodeMeta, error)`:
            *   Try contacting a random `seedNode` using `getNodeClient`.
            *   Call its `GetPreferenceList` RPC with `n = client.replicationFactorN`.
            *   If successful, update `nodeCache` with all nodes in the response and return the list.
            *   Implement retry logic for contacting seed nodes.
        *   Modify `Get(ctx context.Context, key string)`:
            *   Use `resolveCoordinator` to find the primary node for the key.
            *   Loop with retries:
                *   Get gRPC client for the target node (start with primary).
                *   Call the `Get` RPC.
                *   Handle response:
                    *   `OK`, `NOT_FOUND`: Return result.
                    *   `WRONG_NODE`: Update cache with `correct_node`. Set target node to `correct_node` for next retry.
                    *   `ERROR`/Connection Error: Log error. Pick the *next* node from the preference list (if fetched) as the target for the next retry (simple failover for now).
                *   Decrement retry count.
            *   Return error if retries exhausted.
        *   Modify `Put(ctx context.Context, key string, value []byte)`:
            *   Use `resolveCoordinator` to find the primary node.
            *   Loop with retries (similar to Get):
                *   Target a node (start with primary).
                *   Call `Put` RPC (generate timestamp).
                *   Handle response (`OK`, `WRONG_NODE`, `ERROR`/Connection Error) similarly to Get, updating target node for retries.
            *   Return error if retries exhausted.

*   **Outcome:** The client can route GET/PUT requests to the correct primary node using preference lists fetched from seed nodes. It handles basic redirection (`WRONG_NODE`) and connection errors by trying alternative nodes (though not yet the full preference list systematically for GET/PUT). PUT/GET still only interact with one node's storage at a time.

## Iteration 4: Write Replication (W Quorum)

*   **Goal:** Implement the write replication logic where the coordinator node forwards the PUT request to replicas and waits for a quorum (W) of successful responses.
*   **Details:**
    1.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Add a dedicated RPC for replication: `ReplicatePutRequest`, `ReplicatePutResponse`.
        *   `ReplicatePutRequest`: Contains `key`, `value`, `timestamp`.
        *   `ReplicatePutResponse`: Contains `status` (`OK` or `ERROR`) and `node_id` of the replica.
        *   Add new status code: `QUORUM_FAILED`.
        ```protobuf
        // ... (Existing NodeMeta, StatusCode [add QUORUM_FAILED=4], Ping, Get/Put Req/Resp, GetPreferenceList Req/Resp)

        enum StatusCode {
          OK = 0;
          ERROR = 1;
          NOT_FOUND = 2;
          WRONG_NODE = 3;
          QUORUM_FAILED = 4;
        }

        message ReplicatePutRequest {
          string key = 1;
          bytes value = 2;
          google.protobuf.Timestamp timestamp = 3;
          // optional NodeMeta original_coordinator = 4; // If needed later
        }

        message ReplicatePutResponse {
          StatusCode status = 1;
          string node_id = 2; // Replica node reporting status
        }

        service NodeService {
          // ... (Ping, Get, Put, GetPreferenceList)
          rpc ReplicatePut(ReplicatePutRequest) returns (ReplicatePutResponse);
          // Add Gossip later
        }
        ```
        *   Regenerate Go code.
    2.  **Worker RPC Implementation (`pkg/node/rpc.go`):**
        *   Implement `ReplicatePut`:
            *   This RPC is called *by* the coordinator onto a replica.
            *   The replica directly calls its local `node.store.Put(ctx, req.Key, req.Value, req.Timestamp.AsTime())`.
            *   Return `ReplicatePutResponse` with `OK` status and its own `node.meta.NodeId` on success, or `ERROR` on storage failure.
        *   Modify the `Put` RPC handler (Coordinator Logic):
            *   After successfully determining it's the correct coordinator:
                *   Call local `node.store.Put(...)`. If this fails, return `PutResponse` with `ERROR`. Initialize `successCount = 1`.
                *   Use `node.ring.GetN(req.Key, node.config.ReplicationFactorN)` to get the full preference list (`NodeMeta` objects).
                *   Identify the N-1 replica nodes (excluding self).
                *   Create a channel for results (`chan *rpc.ReplicatePutResponse`).
                *   Use a `sync.WaitGroup` to track goroutines.
                *   For each replica:
                    *   Increment WaitGroup.
                    *   Launch a goroutine:
                        *   Defer WaitGroup.Done().
                        *   Get gRPC client for the replica (use `node.connMgr` or similar).
                        *   Call `client.ReplicatePut(ctx, &rpc.ReplicatePutRequest{...})` with appropriate context (e.g., timeout).
                        *   Send the result (or an error indication) back on the results channel.
                *   Create a goroutine to wait for all replica goroutines using WaitGroup.Wait() and then close the results channel.
                *   Collect responses from the results channel:
                    *   Loop reading from the channel until it's closed.
                    *   If a response indicates success (`OK`), increment `successCount`.
                    *   If `successCount` reaches `node.config.WriteQuorumW`, break the loop early (quorum met).
                *   After the loop (channel closed or quorum met):
                    *   If `successCount >= node.config.WriteQuorumW`:
                        *   Return `PutResponse` with `OK` status.
                    *   Otherwise:
                        *   Return `PutResponse` with `QUORUM_FAILED` status. (Implement Hinted Handoff later for failures).
            *   If the current node is *not* the coordinator, return `WRONG_NODE` as before.
    3.  **Worker Node Enhancement (`pkg/node/node.go`):**
        *   Add configuration fields to `NodeConfig` struct or similar: `replicationFactorN int`, `writeQuorumW int`, `readQuorumR int`. Pass these during `NewNode`.
        *   Ensure the `Node` struct has access to a mechanism for making outgoing gRPC calls (e.g., a shared `ConnectionManager` instance or client factory passed during initialization).
    4.  **Client (`pkg/client/client.go`):**
        *   No major changes needed *for this iteration*. The client still sends the `Put` to the coordinator. The client only needs to handle the final `OK` or `QUORUM_FAILED` status from the coordinator's `PutResponse`.

*   **Outcome:** PUT requests are replicated across N nodes. The coordinator ensures at least W nodes acknowledge the write before confirming success to the client. Read (GET) operations are still basic.

## Iteration 5: Quorum Reads (R Quorum) and Basic Read Repair

*   **Goal:** Implement quorum reads where the client contacts R nodes from the preference list, waits for R responses, and performs basic read repair based on timestamps.
*   **Details:**
    1.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Add new status code: `READ_QUORUM_FAILED`.
        ```protobuf
        // ... (Existing NodeMeta, Ping, Get/Put Req/Resp, GetPreferenceList Req/Resp, ReplicatePut Req/Resp)
        enum StatusCode {
          OK = 0;
          ERROR = 1;        // General server-side error
          NOT_FOUND = 2;
          WRONG_NODE = 3;
          QUORUM_FAILED = 4; // Write quorum failed
          READ_QUORUM_FAILED = 5;
        }
        // ... (Rest of proto, Service definition)
        ```
        *   Regenerate Go code.
    2.  **Client Enhancements (`pkg/client/client.go`):**
        *   Modify `Get(ctx context.Context, key string) (value []byte, err error)`:
            *   Fetch the preference list using `fetchPreferenceList(ctx, key)`. Handle errors. If list has fewer than `client.readQuorumR` nodes, return error (e.g., `ErrInsufficientNodes`).
            *   Create a channel for results (`chan *rpc.GetResponse`).
            *   Use a `sync.WaitGroup`.
            *   Identify the first `client.replicationFactorN` nodes from the preference list (or fewer if the list is smaller).
            *   For each of these target N nodes:
                *   Increment WaitGroup.
                *   Launch a goroutine:
                    *   Defer WaitGroup.Done().
                    *   Get gRPC client for the node's address (`nodeMeta.Ip`, `nodeMeta.Port`).
                    *   Call `client.Get(ctx, &rpc.GetRequest{Key: key})` with appropriate timeout.
                    *   Send the `GetResponse` (or nil/error indication on failure) back on the results channel.
            *   Create a goroutine to `wg.Wait()` and then `close(resultsChan)`.
            *   Collect valid responses:
                *   Maintain `validResponses []*rpc.GetResponse`.
                *   Loop reading from `resultsChan` until closed.
                *   If a non-nil response `r` is received AND `r.Status == rpc.StatusCode_OK || r.Status == rpc.StatusCode_NOT_FOUND`:
                    *   Append `r` to `validResponses`.
            *   Check Quorum:
                *   If `len(validResponses) < client.readQuorumR`:
                    *   Return `nil, ErrReadQuorumFailed` (define this error).
            *   Find Latest Value:
                *   Find the response in `validResponses` with the maximum timestamp (`latestResponse`). Handle potential `NOT_FOUND` status among responses (latest version might be a deletion marker/tombstone).
                *   If `latestResponse == nil` (e.g., all R responses were NOT_FOUND or errors), return `nil, ErrNotFound`.
                *   If `latestResponse.Status == rpc.StatusCode_NOT_FOUND`, return `nil, ErrNotFound`.
            *   **Perform Read Repair:**
                *   Create a new context for repairs (short timeout).
                *   Use a separate `sync.WaitGroup` for repair operations.
                *   For each response `r` in `validResponses`:
                    *   If `r.Status == rpc.StatusCode_NOT_FOUND` or `r.Timestamp.AsTime().Before(latestResponse.Timestamp.AsTime())`:
                        *   Increment repair WaitGroup.
                        *   Launch repair goroutine:
                            *   Defer repair WaitGroup.Done().
                            *   Get gRPC client for `r.Coordinator`.
                            *   Call `client.Put(repairCtx, &rpc.PutRequest{Key: key, Value: latestResponse.Value, Timestamp: latestResponse.Timestamp})`. Log errors but don't block main path.
            *   Optional: Wait briefly for repairs using `repairWg.Wait()` if desired, but typically repairs are best effort/background.
            *   Return `latestResponse.Value, nil`.
    3.  **Worker Node (`pkg/node/node.go`, `pkg/node/rpc.go`):**
        *   No changes required in the worker logic for this iteration. It just needs to respond to `Get` requests from the client and potentially `Put` requests triggered by read repair.

*   **Outcome:** GET requests now achieve quorum by contacting R nodes. The client selects the latest value based on timestamps, improving read consistency. Basic read repair is implemented by asynchronously sending PUTs to nodes with stale data.

## Iteration 6: Gossip Protocol and Dynamic Membership

*   **Goal:** Implement a gossip protocol for nodes to exchange membership information, allowing the cluster view (`chash.Ring`) to converge dynamically.
*   **Details:**
    1.  **Vector Clocks/Versioning (`pkg/node/node.go`, `pkg/rpc/dynamo.proto`):**
        *   Add a `version` field (e.g., monotonic integer counter) to `rpc.NodeMeta`.
        *   Each `Node` in `pkg/node/node.go` needs a local `version` counter (atomic or mutex-protected) associated with its `meta`. This counter increments whenever the node updates its own ring state (adds/removes/updates another node's state based on gossip).
        ```protobuf
        // ... (Existing imports)
        message NodeMeta {
          string node_id = 1;
          string ip = 2;
          int32 port = 3;
          int64 version = 4; // Version counter for this node's state info in the ring
          // Optional: Add status enum (UP, DOWN, LEAVING) later for explicit state
        }
        // ... (Rest of proto)
        ```
        *   Regenerate Go code.
    2.  **RPC Definition (`pkg/rpc/dynamo.proto`):**
        *   Add `GossipRequest`, `GossipResponse`.
        *   `GossipRequest`: Contains the sender's `NodeMeta` and its view of the cluster: `map<string, NodeMeta> node_states` (NodeID -> Meta).
        *   `GossipResponse`: Contains nodes the receiver knows about that are *newer* than what the sender sent: `map<string, NodeMeta> updated_node_states`.
        ```protobuf
         // ... (Existing NodeMeta, StatusCodes, other Req/Resp)

        message GossipRequest {
          NodeMeta sender_node = 1; // Metadata of the node sending the gossip
          map<string, NodeMeta> node_states = 2; // Sender's view of the cluster (NodeID -> Meta)
        }

        message GossipResponse {
           map<string, NodeMeta> updated_node_states = 1; // Receiver sends back newer info it has
        }

        service NodeService {
          // ... (Ping, Get, Put, GetPreferenceList, ReplicatePut)
          rpc Gossip(GossipRequest) returns (GossipResponse);
        }
        ```
        *   Regenerate Go code.
    3.  **Worker Node - Background Gossip Task (`pkg/node/node.go`):**
        *   Add a `StartGossip(interval time.Duration, k int)` method to the `Node` struct, callable after node initialization.
        *   This method launches a background goroutine (`gossipLoop`).
        *   `gossipLoop()`:
            *   Use `time.NewTicker(interval)`.
            *   In a loop:
                *   Wait for ticker.
                *   Select `k` random *other* node IDs from `node.ring.GetAllNodes()` (handle case with < k other nodes).
                *   For each selected peer ID:
                    *   Get peer `NodeMeta` using `node.ring.GetNodeMeta(peerID)`.
                    *   Launch a goroutine `doGossipWithPeer(peerMeta)`.
    4.  **Worker Node - Initiate Gossip (`pkg/node/node.go`):**
        *   Implement `doGossipWithPeer(peerMeta rpc.NodeMeta)`:
            *   Prepare `GossipRequest`:
                *   Lock the ring for reading.
                *   Set `sender_node` to own `node.meta`.
                *   Copy current `node.ring` state into `request.node_states` (map NodeID -> NodeMeta).
                *   Unlock the ring.
            *   Get gRPC client for the peer's address.
            *   Call `client.Gossip(ctx, request)` with a timeout.
            *   If successful response received:
                *   Call `node.mergeGossipData(response.UpdatedNodeStates)`.
            *   Handle/log errors (e.g., connection failed - could trigger failure detection later).
    5.  **Worker Node - Process Incoming Gossip (`pkg/node/node.go`):**
        *   Implement `mergeGossipData(receivedStates map[string]*rpc.NodeMeta)`:
            *   Acquire lock for `node.ring` (write lock).
            *   `changed := false`
            *   Iterate through `receivedStates` (key=nodeID, value=receivedNodeMeta):
                *   Get existing meta for `nodeID` using `node.ring.GetNodeMeta(nodeID)`. (`existingNodeMeta`, `ok`) 
                *   If `!ok` (node unknown):
                    *   `node.ring.AddNode(*receivedNodeMeta)`
                    *   `changed = true`
                *   Else if `receivedNodeMeta.Version > existingNodeMeta.Version`:
                    *   Update node in ring (e.g., `node.ring.RemoveNode(nodeID)`, then `node.ring.AddNode(*receivedNodeMeta)` or have an `UpdateNode` method).
                    *   `changed = true`
            *   If `changed`:
                *   Increment own `node.meta.Version` (atomically or under lock).
            *   Release lock.
    6.  **Worker RPC Implementation (`pkg/node/rpc.go`):**
        *   Implement the `Gossip` RPC handler:
            *   Call `node.mergeGossipData(request.NodeStates)` to potentially update own state.
            *   Prepare `GossipResponse`:
                *   Lock ring for reading.
                *   Initialize `response.UpdatedNodeStates = make(map[string]*rpc.NodeMeta)`
                *   Iterate through own current `node.ring.GetAllNodes()` (`localNodeMeta`):
                    *   Check if sender knows this node: `senderMeta, senderKnows := request.NodeStates[localNodeMeta.NodeId]`
                    *   If `!senderKnows` OR `localNodeMeta.Version > senderMeta.Version`:
                        *   Add `localNodeMeta` to `response.UpdatedNodeStates`.
                *   Unlock ring.
            *   Return the `GossipResponse`.
    7.  **Client (`pkg/client/client.go`):**
        *   No direct changes needed. The client relies on `GetPreferenceList` which will become more accurate as nodes gossip and update their rings.

*   **Future Considerations (Beyond Iteration 6):**
    *   **Failure Detection:** Integrate active pinging or use gossip timeouts/failed connections to mark nodes as DOWN/UP, updating versions and propagating via gossip.
    *   **Hinted Handoff:** When a `ReplicatePut` fails, the coordinator stores the write locally and attempts delivery later when the replica recovers (detected via gossip).
    *   **Node Joining/Leaving:** Formalize the process. New nodes gossip to join using seed nodes. Leaving nodes could potentially gossip a "leaving" state. Need data transfer during joins/leaves (transferring key ranges).
    *   **Vector Clocks for Data:** Implement vector clocks per key-value pair to handle concurrent writes correctly, instead of just relying on timestamps.
    *   **Tombstones:** Implement deletion markers for proper propagation of deletes.
    *   **Configuration:** Robust config loading (`pkg/config`).
    *   **Observability:** Add logging, metrics, tracing.

*   **Outcome:** Nodes dynamically exchange and merge membership/state information using gossip. The consistent hash ring (`chash.Ring`) on each node converges towards the current cluster state, allowing the system to adapt to changes without manual intervention (after initial bootstrapping with seed nodes).

--- 