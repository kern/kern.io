package crdt

import (
	"time"

	"github.com/google/uuid"
)

// OpType represents the type of operation in the event graph.
type OpType uint8

const (
	OpInsertNode OpType = iota
	OpDeleteNode
	OpSetProperty
	OpDeleteProperty
	OpInsertEdge
	OpDeleteEdge
	OpMoveNode    // re-parent in the hierarchy
	OpReorderNode // change sibling order via fractional index
	OpRestoreNode // un-delete a soft-deleted node
	OpRestoreEdge // un-delete a soft-deleted edge
)

// EventID uniquely identifies an event in the causal graph.
// It is a (ReplicaID, Seq) pair — a Lamport-style identifier.
type EventID struct {
	ReplicaID string `json:"replicaId"`
	Seq       uint64 `json:"seq"`
}

// Operation is a single atomic change to the graph.
type Operation struct {
	ID        EventID       `json:"id"`
	Parents   []EventID     `json:"parents"` // causal parents in the DAG
	Type      OpType        `json:"type"`
	TargetID  uuid.UUID     `json:"targetId"`            // node or edge being operated on
	Key       string        `json:"key,omitempty"`       // property key for set/delete property
	Value     interface{}   `json:"value,omitempty"`     // property value
	EdgeFrom  uuid.UUID     `json:"edgeFrom,omitempty"`  // for edge operations
	EdgeTo    uuid.UUID     `json:"edgeTo,omitempty"`    // for edge operations
	EdgeType  string        `json:"edgeType,omitempty"`  // edge type label
	NodeType  string        `json:"nodeType,omitempty"`  // node type label
	ParentRef *uuid.UUID    `json:"parentRef,omitempty"` // parent node for hierarchy
	Timestamp time.Time     `json:"timestamp"`
}

// Value types that can be stored as properties.
type Value struct {
	Type     ValueType   `json:"type"`
	Data     interface{} `json:"data"`
	// For register CRDT: track concurrent writes
	Versions []EventID  `json:"versions,omitempty"`
}

type ValueType uint8

const (
	ValueNull ValueType = iota
	ValueBool
	ValueInt
	ValueFloat
	ValueString
	ValueBytes
	ValueArray
	ValueMap
	ValueRef // reference to another node
)
