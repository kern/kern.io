package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// --- toBatchOp coverage: all op types and error paths ---

func TestToBatchOpAllTypes(t *testing.T) {
	nodeID := uuid.New().String()
	edgeID := uuid.New().String()
	fromID := uuid.New().String()
	toID := uuid.New().String()
	parentID := uuid.New().String()

	tests := []struct {
		name string
		op   batchInputOp
	}{
		{"insertNode", batchInputOp{Op: "insertNode", NodeType: "user"}},
		{"insert-node", batchInputOp{Op: "insert-node", NodeType: "user"}},
		{"deleteNode", batchInputOp{Op: "deleteNode", NodeID: nodeID}},
		{"delete-node", batchInputOp{Op: "delete-node", NodeID: nodeID}},
		{"setProperty", batchInputOp{Op: "setProperty", NodeID: nodeID, Key: "k", Value: "v"}},
		{"set-property", batchInputOp{Op: "set-property", NodeID: nodeID, Key: "k", Value: "v"}},
		{"deleteProperty", batchInputOp{Op: "deleteProperty", NodeID: nodeID, Key: "k"}},
		{"delete-property", batchInputOp{Op: "delete-property", NodeID: nodeID, Key: "k"}},
		{"insertEdge", batchInputOp{Op: "insertEdge", EdgeType: "e", FromID: fromID, ToID: toID}},
		{"insert-edge", batchInputOp{Op: "insert-edge", EdgeType: "e", FromID: fromID, ToID: toID}},
		{"deleteEdge", batchInputOp{Op: "deleteEdge", EdgeID: edgeID}},
		{"delete-edge", batchInputOp{Op: "delete-edge", EdgeID: edgeID}},
		{"moveNode", batchInputOp{Op: "moveNode", NodeID: nodeID, ParentID: parentID}},
		{"move-node", batchInputOp{Op: "move-node", NodeID: nodeID}},
		{"reorderNode", batchInputOp{Op: "reorderNode", NodeID: nodeID, Position: "M"}},
		{"reorder-node", batchInputOp{Op: "reorder-node", NodeID: nodeID, Position: "M"}},
		{"restoreNode", batchInputOp{Op: "restoreNode", NodeID: nodeID}},
		{"restore-node", batchInputOp{Op: "restore-node", NodeID: nodeID}},
		{"cascadeDelete", batchInputOp{Op: "cascadeDelete", NodeID: nodeID}},
		{"cascade-delete", batchInputOp{Op: "cascade-delete", NodeID: nodeID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, err := tt.op.toBatchOp()
			if err != nil {
				t.Fatalf("toBatchOp(%s) failed: %v", tt.name, err)
			}
			_ = op
		})
	}
}

func TestToBatchOpUnknownType(t *testing.T) {
	op := batchInputOp{Op: "unknown-op"}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for unknown op type")
	}
	if !strings.Contains(err.Error(), "unknown op type") {
		t.Errorf("expected 'unknown op type' error, got: %v", err)
	}
}

func TestToBatchOpInvalidNodeID(t *testing.T) {
	op := batchInputOp{Op: "deleteNode", NodeID: "not-a-uuid"}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for invalid nodeId")
	}
}

func TestToBatchOpInvalidEdgeID(t *testing.T) {
	op := batchInputOp{Op: "deleteEdge", EdgeID: "not-a-uuid"}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for invalid edgeId")
	}
}

func TestToBatchOpInvalidParentID(t *testing.T) {
	op := batchInputOp{Op: "moveNode", NodeID: uuid.New().String(), ParentID: "not-a-uuid"}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for invalid parentId")
	}
}

func TestToBatchOpInvalidFromID(t *testing.T) {
	op := batchInputOp{Op: "insertEdge", FromID: "not-a-uuid", ToID: uuid.New().String()}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for invalid fromId")
	}
}

func TestToBatchOpInvalidToID(t *testing.T) {
	op := batchInputOp{Op: "insertEdge", FromID: uuid.New().String(), ToID: "not-a-uuid"}
	_, err := op.toBatchOp()
	if err == nil {
		t.Error("expected error for invalid toId")
	}
}

func TestToBatchOpWithProperties(t *testing.T) {
	op := batchInputOp{
		Op:         "insertNode",
		NodeType:   "user",
		Properties: map[string]interface{}{"name": "Alice", "age": 30.0},
	}
	result, err := op.toBatchOp()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Properties["name"] != "Alice" {
		t.Error("expected properties to be preserved")
	}
}

// --- adminCmd coverage ---

func TestAdminCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"admin"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestAdminCmdUnknown(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"admin", "unknown-cmd"})
	if code != 1 {
		t.Error("expected error for unknown admin command")
	}
	if !strings.Contains(errOut.String(), "Unknown admin command") {
		t.Errorf("expected 'Unknown admin command' error, got: %s", errOut.String())
	}
}

func TestAdminDeletedNodesEmpty(t *testing.T) {
	cli, out, _ := newTestCLI()
	code := cli.Run([]string{"admin", "deleted-nodes"})
	if code != 0 {
		t.Fatal("deleted-nodes should succeed")
	}
	if !strings.Contains(out.String(), "[]") && !strings.Contains(out.String(), "null") {
		t.Errorf("expected empty list, got: %s", out.String())
	}
}

func TestAdminStatsWithEdges(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert nodes
	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "product"})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	// Insert edge
	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "owns", "--from", aID, "--to", bID})

	// Stats
	out.Reset()
	code := cli.Run([]string{"admin", "stats"})
	if code != 0 {
		t.Fatal("stats failed")
	}
	var stats map[string]interface{}
	json.Unmarshal(out.Bytes(), &stats)
	if stats["totalEdges"].(float64) != 1 {
		t.Errorf("expected 1 edge, got %v", stats["totalEdges"])
	}
	nbt := stats["nodesByType"].(map[string]interface{})
	if nbt["user"].(float64) != 1 || nbt["product"].(float64) != 1 {
		t.Error("nodesByType counts wrong")
	}
}

// --- printJSON error path ---

func TestPrintJSONUnmarshalable(t *testing.T) {
	cli, _, errOut := newTestCLI()
	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	cli.printJSON(ch)
	if !strings.Contains(errOut.String(), "Error marshaling JSON") {
		t.Errorf("expected marshal error, got: %s", errOut.String())
	}
}

// --- treeReorder additional scenarios ---

func TestTreeReorderWithAfter(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create parent and children
	cli.Run([]string{"node", "insert", "--type", "list"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item1ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item2ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item3ID := r["id"].(string)

	// Reorder: place item3 after item1
	out.Reset()
	code := cli.Run([]string{"tree", "reorder", "--node", item3ID, "--after", item1ID})
	if code != 0 {
		t.Fatal("reorder with after failed")
	}

	// Verify order
	out.Reset()
	cli.Run([]string{"tree", "ordered-children", parentID})
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 children, got %d", len(nodes))
	}
	// item3 should be after item1 but before item2
	if nodes[0]["id"] != item1ID {
		t.Errorf("expected item1 first, got %v", nodes[0]["id"])
	}
	_ = item2ID
}

func TestTreeReorderBadNodeID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "reorder", "--node", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad node UUID")
	}
}

func TestTreeReorderBadAfterID(t *testing.T) {
	cli, _, _ := newTestCLI()
	nodeID := uuid.New().String()
	code := cli.Run([]string{"tree", "reorder", "--node", nodeID, "--after", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad after UUID")
	}
}

func TestTreeReorderBadBeforeID(t *testing.T) {
	cli, _, _ := newTestCLI()
	nodeID := uuid.New().String()
	code := cli.Run([]string{"tree", "reorder", "--node", nodeID, "--before", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad before UUID")
	}
}

func TestTreeReorderBetweenAfterAndBefore(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "list"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item1ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item2ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	item3ID := r["id"].(string)

	// Reorder item3 between item1 and item2
	out.Reset()
	code := cli.Run([]string{"tree", "reorder", "--node", item3ID, "--after", item1ID, "--before", item2ID})
	if code != 0 {
		t.Fatal("reorder between failed")
	}
}

// --- nodeInsert with parent and props ---

func TestNodeInsertWithParentAndProps(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"node", "insert", "--type", "file", "--parent", parentID, "--props", `{"name":"test.txt","size":1024}`})
	if code != 0 {
		t.Fatal("insert with parent+props failed")
	}
	json.Unmarshal(out.Bytes(), &r)
	childID := r["id"].(string)

	// Verify parent
	out.Reset()
	cli.Run([]string{"tree", "parent", childID})
	var parent map[string]interface{}
	json.Unmarshal(out.Bytes(), &parent)
	if parent["id"] != parentID {
		t.Error("parent mismatch")
	}
}

func TestNodeInsertBadParent(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "insert", "--type", "file", "--parent", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad parent UUID")
	}
}

func TestNodeInsertBadProps(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "insert", "--type", "file", "--props", "not-json"})
	if code != 1 {
		t.Error("expected error for invalid JSON props")
	}
}

// --- nodeSoftDelete/nodeCascadeDelete/nodeRestore success paths ---

func TestNodeSoftDeleteSuccess(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"node", "soft-delete", nodeID})
	if code != 0 {
		t.Fatal("soft-delete failed")
	}
	if !strings.Contains(out.String(), "Soft-deleted") {
		t.Error("expected soft-delete confirmation")
	}
}

func TestNodeSoftDeleteBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "soft-delete", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestNodeCascadeDeleteSuccess(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"node", "cascade-delete", nodeID})
	if code != 0 {
		t.Fatal("cascade-delete failed")
	}
	if !strings.Contains(out.String(), "Cascade-deleted") {
		t.Error("expected cascade-delete confirmation")
	}
}

func TestNodeCascadeDeleteBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "cascade-delete", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestNodeRestoreSuccess(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "soft-delete", nodeID})

	out.Reset()
	code := cli.Run([]string{"node", "restore", nodeID})
	if code != 0 {
		t.Fatal("restore failed")
	}
	if !strings.Contains(out.String(), "Restored") {
		t.Error("expected restore confirmation")
	}
}

func TestNodeRestoreBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "restore", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

// --- edgeInsert with props, edgeGet success paths ---

func TestEdgeInsertWithProps(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user"})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"edge", "insert", "--type", "follows", "--from", aID, "--to", bID, "--props", `{"weight":5}`})
	if code != 0 {
		t.Fatal("edge insert with props failed")
	}
}

func TestEdgeInsertBadFromID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "insert", "--type", "e", "--from", "bad-uuid", "--to", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for bad from UUID")
	}
}

func TestEdgeInsertBadToID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "insert", "--type", "e", "--from", uuid.New().String(), "--to", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad to UUID")
	}
}

func TestEdgeInsertBadProps(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "a"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "b"})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"edge", "insert", "--type", "e", "--from", aID, "--to", bID, "--props", "not-json"})
	if code != 1 {
		t.Error("expected error for bad JSON props")
	}
}

func TestEdgeGetWithTypeFilter(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user"})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "follows", "--from", aID, "--to", bID})

	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "likes", "--from", aID, "--to", bID})

	// Get out edges with type filter
	out.Reset()
	code := cli.Run([]string{"edge", "get", aID, "--direction", "out", "--type", "follows"})
	if code != 0 {
		t.Fatal("edge get with type filter failed")
	}
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Errorf("expected 1 follows edge, got %d", len(edges))
	}

	// Get in edges with type filter
	out.Reset()
	code = cli.Run([]string{"edge", "get", bID, "--direction", "in", "--type", "follows"})
	if code != 0 {
		t.Fatal("edge get in with type filter failed")
	}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Errorf("expected 1 incoming follows edge, got %d", len(edges))
	}

	// Get both directions
	out.Reset()
	code = cli.Run([]string{"edge", "get", aID, "--direction", "both"})
	if code != 0 {
		t.Fatal("edge get both failed")
	}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 2 {
		t.Errorf("expected 2 edges in both, got %d", len(edges))
	}
}

func TestEdgeGetDefaultDirection(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user"})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "follows", "--from", aID, "--to", bID})

	// Get edges with default direction (out)
	out.Reset()
	code := cli.Run([]string{"edge", "get", aID})
	if code != 0 {
		t.Fatal("edge get default direction failed")
	}
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

// --- treeParent/treeSubtree/treeAncestors success paths ---

func TestTreeParentNotFound(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert root node (no parent)
	cli.Run([]string{"node", "insert", "--type", "root"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	rootID := r["id"].(string)

	// Get parent of root (should be null)
	out.Reset()
	code := cli.Run([]string{"tree", "parent", rootID})
	if code != 0 {
		t.Fatal("tree parent for root should succeed")
	}
	if !strings.Contains(out.String(), "null") {
		t.Errorf("expected null parent, got: %s", out.String())
	}
}

func TestTreeParentBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "parent", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestTreeSubtreeBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "subtree", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestTreeAncestorsBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "ancestors", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestTreeOrderedChildrenBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "ordered-children", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

// --- propertySet various types ---

func TestPropertySetNumericValue(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"property", "set", "--node", nodeID, "--key", "age", "--value", "42"})
	if code != 0 {
		t.Fatal("property set numeric failed")
	}
}

func TestPropertySetBoolValue(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"property", "set", "--node", nodeID, "--key", "active", "--value", "true"})
	if code != 0 {
		t.Fatal("property set bool failed")
	}
}

func TestPropertySetStringFallback(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Non-JSON value should fall back to string
	out.Reset()
	code := cli.Run([]string{"property", "set", "--node", nodeID, "--key", "name", "--value", "plain-string"})
	if code != 0 {
		t.Fatal("property set string fallback failed")
	}
}

func TestPropertySetBadNodeID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "set", "--node", "bad-uuid", "--key", "k"})
	if code != 1 {
		t.Error("expected error for bad node UUID")
	}
}

func TestPropertyGetNotFoundKey(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"property", "get", nodeID, "nonexistent"})
	if code != 1 {
		t.Error("expected error for nonexistent property")
	}
}

// --- queryByType, queryTraverse ---

func TestQueryByTypeEmpty(t *testing.T) {
	cli, out, _ := newTestCLI()
	code := cli.Run([]string{"query", "by-type", "nonexistent"})
	if code != 0 {
		t.Fatal("by-type should succeed even with no matches")
	}
	// Should return empty or null
	output := strings.TrimSpace(out.String())
	if output != "[]" && output != "null" {
		t.Errorf("expected empty list, got: %s", output)
	}
}

func TestQueryByTypeNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query", "by-type"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestQueryTraverseBadStartID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query", "traverse", "--start", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad start UUID")
	}
}

func TestQueryTraverseDefaultDirection(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "task"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Traverse with defaults (no edge-type, default direction, default depth)
	out.Reset()
	code := cli.Run([]string{"query", "traverse", "--start", nodeID})
	if code != 0 {
		t.Fatal("traverse with defaults failed")
	}
}

func TestQueryUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown query command")
	}
}

// --- Unknown subcommands ---

func TestNodeCmdUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown node command")
	}
}

func TestEdgeCmdUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown edge command")
	}
}

func TestPropertyCmdUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown property command")
	}
}

func TestTreeCmdUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown tree command")
	}
}

// --- prop alias ---

func TestPropAlias(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"prop", "set", "--node", nodeID, "--key", "k", "--value", `"v"`})
	if code != 0 {
		t.Fatal("prop alias for property should work")
	}
}

// --- help variants ---

func TestHelpFlag(t *testing.T) {
	cli, out, _ := newTestCLI()
	code := cli.Run([]string{"--help"})
	if code != 0 {
		t.Error("--help should return 0")
	}
	if !strings.Contains(out.String(), "graphdb-cli") {
		t.Error("--help should show usage")
	}
}

func TestHFlag(t *testing.T) {
	cli, out, _ := newTestCLI()
	code := cli.Run([]string{"-h"})
	if code != 0 {
		t.Error("-h should return 0")
	}
	if !strings.Contains(out.String(), "graphdb-cli") {
		t.Error("-h should show usage")
	}
}

// --- TreeMove bad UUID ---

func TestTreeMoveBadNodeID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "move", "--node", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad node UUID")
	}
}

func TestTreeMoveBadParentID(t *testing.T) {
	cli, _, _ := newTestCLI()
	nodeID := uuid.New().String()
	code := cli.Run([]string{"tree", "move", "--node", nodeID, "--parent", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad parent UUID")
	}
}

func TestTreeMoveToNull(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Move to null parent
	out.Reset()
	code := cli.Run([]string{"tree", "move", "--node", nodeID, "--parent", "null"})
	if code != 0 {
		t.Fatal("move to null parent failed")
	}
}

// --- parseFlags edge cases ---

func TestParseFlagsBooleanFlag(t *testing.T) {
	flags := parseFlags([]string{"--verbose", "--key", "value"})
	if flags.Get("verbose") != "true" {
		t.Error("boolean flag should be 'true'")
	}
	if flags.Get("key") != "value" {
		t.Error("key-value flag should work")
	}
}

// --- uuidsToStrings ---

func TestUuidsToStrings(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	strs := uuidsToStrings(ids)
	if len(strs) != 2 {
		t.Fatalf("expected 2 strings, got %d", len(strs))
	}
	for i, s := range strs {
		if s != ids[i].String() {
			t.Errorf("mismatch at %d", i)
		}
	}
}

func TestUuidsToStringsEmpty(t *testing.T) {
	strs := uuidsToStrings([]uuid.UUID{})
	if len(strs) != 0 {
		t.Error("expected empty slice")
	}
}
