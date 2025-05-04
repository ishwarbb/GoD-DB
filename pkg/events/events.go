package events

import "time"

// --- Enums ---

type EventType string

const (
	NodeStatus             EventType = "NODE_STATUS"
	RpcCallStart           EventType = "RPC_CALL_START"
	RpcCallReceived        EventType = "RPC_CALL_RECEIVED" // Server-side
	RpcCallEnd             EventType = "RPC_CALL_END"
	PreferenceListResolved EventType = "PREFERENCE_LIST_RESOLVED"
	QuorumAttempt          EventType = "QUORUM_ATTEMPT"
	QuorumResult           EventType = "QUORUM_RESULT"
	ReadRepairStart        EventType = "READ_REPAIR_START"
	ReadRepairEnd          EventType = "READ_REPAIR_END"
	NodeStateUpdate        EventType = "NODE_STATE_UPDATE" // Server-side
)

type NodeOperationalStatus string

const (
	StatusUp   NodeOperationalStatus = "UP"
	StatusDown NodeOperationalStatus = "DOWN"
)

type RpcOperation string

const (
	OpGet             RpcOperation = "GET"
	OpPut             RpcOperation = "PUT"
	OpGetPreference   RpcOperation = "GET_PREFERENCE_LIST" // Server-side receive
	OpReadRepair      RpcOperation = "READ_REPAIR"         // Server-side receive
	OpInternalForward RpcOperation = "INTERNAL_FORWARD"    // Potentially server-side
)

type RpcStatus string

const (
	StatusOk          RpcStatus = "OK"
	StatusNotFound    RpcStatus = "NOT_FOUND"
	StatusWrongNode   RpcStatus = "WRONG_NODE"
	StatusQuorumFailed RpcStatus = "QUORUM_FAILED"
	StatusError       RpcStatus = "ERROR"
	StatusTimeout     RpcStatus = "TIMEOUT"
)

type QuorumOperation string

const (
	QuorumRead  QuorumOperation = "READ_QUORUM"
	QuorumWrite QuorumOperation = "WRITE_QUORUM"
)


// --- Base Event Structure ---

// BaseEvent contains common fields for all events.
type BaseEvent struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
}

// --- Specific Event Payloads ---
// Note: Using pointers for optional fields to distinguish between zero value and not present

type NodeStatusPayload struct {
	NodeID  string                `json:"nodeId"`
	Address string                `json:"address"`
	Status  NodeOperationalStatus `json:"status"`
}

type RpcCallStartPayload struct {
	ClientID     string       `json:"clientId"` // ID of the client instance making the call
	TargetNodeID string       `json:"targetNodeId"`
	Operation    RpcOperation `json:"operation"`
	Key          string       `json:"key"`
	Value        *[]byte      `json:"value,omitempty"` // Only for PUT
}

type RpcCallReceivedPayload struct { // Server-side event
	NodeID    string       `json:"nodeId"` // Node receiving the call
	SourceID  string       `json:"sourceId"` // Client ID or originating Node ID
	Operation RpcOperation `json:"operation"`
	Key       string       `json:"key"`
}

type RpcCallEndPayload struct {
	ClientID       string       `json:"clientId"`
	TargetNodeID   string       `json:"targetNodeId"`
	Operation      RpcOperation `json:"operation"`
	Key            string       `json:"key"`
	Status         RpcStatus    `json:"status"`
	CorrectNodeID  *string      `json:"correctNodeId,omitempty"` // If status is WRONG_NODE
	Value          *[]byte      `json:"value,omitempty"`          // If status is OK for GET
	FinalTimestamp *time.Time   `json:"finalTimestamp,omitempty"` // For successful PUT/GET
}

type PreferenceListResolvedPayload struct {
	ClientID          string   `json:"clientId"`
	Key               string   `json:"key"`
	CoordinatorNodeID string   `json:"coordinatorNodeId"`
	PreferenceList    []string `json:"preferenceList"` // List of node IDs
}

type QuorumAttemptPayload struct {
	SourceNodeID   string          `json:"sourceNodeId"` // Client ID or Coordinator Node ID
	Operation      QuorumOperation `json:"operation"`
	Key            string          `json:"key"`
	TargetNodes    []string        `json:"targetNodes"` // List of node IDs being contacted
	RequiredQuorum int             `json:"requiredQuorum"` // R or W
}

type QuorumResponseDetail struct {
	NodeID    string     `json:"nodeId"`
	Status    RpcStatus  `json:"status"`
	Value     *[]byte    `json:"value,omitempty"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
}


type QuorumResultPayload struct {
	SourceNodeID   string                 `json:"sourceNodeId"`
	Operation      QuorumOperation        `json:"operation"`
	Key            string                 `json:"key"`
	Success        bool                   `json:"success"`
	AchievedCount  int                    `json:"achievedCount"`
	RequiredQuorum int                    `json:"requiredQuorum"`
	Responses      []QuorumResponseDetail `json:"responses"` // Details from each node contacted
}

type ReadRepairStartPayload struct {
	ClientID        string    `json:"clientId"`
	Key             string    `json:"key"`
	TargetNodeID    string    `json:"targetNodeId"` // Node being repaired
	LatestValue     []byte    `json:"latestValue"`
	LatestTimestamp time.Time `json:"latestTimestamp"`
}

type ReadRepairEndPayload struct {
	ClientID     string  `json:"clientId"`
	Key          string  `json:"key"`
	TargetNodeID string  `json:"targetNodeId"`
	Success      bool    `json:"success"`
	Error        *string `json:"error,omitempty"`
}

type KeyValueEntry struct {
	Value     []byte    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

type NodeStateUpdatePayload struct { // Server-side event
	NodeID  string                   `json:"nodeId"`
	KvStore map[string]KeyValueEntry `json:"kvStore"` // Snapshot or update of the node's KVs
}

// --- Event Wrapper ---

// Event is the structure sent to the visualizer.
type Event struct {
	BaseEvent
	Payload interface{} `json:"payload"`
}

// Helper function to create a new event
func NewEvent(eventType EventType, payload interface{}) Event {
	return Event{
		BaseEvent: BaseEvent{
			Type:      eventType,
			Timestamp: time.Now().UTC(),
		},
		Payload: payload,
	}
} 