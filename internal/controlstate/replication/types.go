package replication

import (
	"errors"
	"time"
)

const (
	StreamDistanceThreshold = 10
	ElapsedLagThreshold     = 5 * time.Second
)

var ErrWriteNotWritable = errors.New("write fenced: node is not writable")

type ChangeEvent struct {
	Repository  string    `json:"repository"`
	Operation   string    `json:"operation"` // CREATE, UPDATE, DELETE, LOG, etc.
	TargetID    string    `json:"target_id,omitempty"`
	PayloadHash string    `json:"payload_hash,omitempty"`
	Payload     []byte    `json:"payload,omitempty"`
	StreamID    string    `json:"stream_id,omitempty"` // populated by producer/consumer
	Timestamp   time.Time `json:"timestamp"`
}
