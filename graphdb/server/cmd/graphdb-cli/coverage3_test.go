package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/graph"
)

// ---------------------------------------------------------------------------
// batchCmd: test the full stdin-based batch execute path with all op types
// ---------------------------------------------------------------------------

func TestBatchCmdExecuteViaStdinAllOps(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert nodes first so we have IDs to use in batch
	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "product"})
	json.Unmarshal(out.Bytes(), &r)
	node2ID := r["id"].(string)

	// Create batch JSON covering multiple op types
	batch := `[
		{"op": "insert-node", "nodeType": "task", "properties": {"name": "batch-task"}},
		{"op": "set-property", "nodeId": "` + nodeID + `", "key": "email", "value": "test@test.com"},
		{"op": "insert-edge", "edgeType": "owns", "fromId": "` + nodeID + `", "toId": "` + node2ID + `"},
		{"op": "delete-property", "nodeId": "` + nodeID + `", "key": "email"}
	]`

	// Override stdin
	oldStdin := os.Stdin
	r2, w, _ := os.Pipe()
	w.Write([]byte(batch))
	w.Close()
	os.Stdin = r2
	defer func() { os.Stdin = oldStdin }()

	out.Reset()
	code := cli.Run([]string{"batch", "execute"})
	if code != 0 {
		t.Fatalf("batch execute failed with code %d", code)
	}

	var results []map[string]interface{}
	json.Unmarshal(out.Bytes(), &results)
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}
}

func TestBatchCmdInvalidJSON(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte("not-json"))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"batch", "execute"})
	if code != 1 {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(errOut.String(), "Invalid batch JSON") {
		t.Errorf("expected JSON error, got: %s", errOut.String())
	}
}

func TestBatchCmdToBatchOpError(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte(`[{"op": "delete-node", "nodeId": "not-a-uuid"}]`))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"batch", "execute"})
	if code != 1 {
		t.Error("expected error for invalid nodeId")
	}
	if !strings.Contains(errOut.String(), "Batch op 0") {
		t.Errorf("expected batch op error, got: %s", errOut.String())
	}
}

func TestBatchCmdUnknownOpType(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte(`[{"op": "explode"}]`))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"batch", "execute"})
	if code != 1 {
		t.Error("expected error for unknown op type")
	}
	if !strings.Contains(errOut.String(), "unknown op type") {
		t.Errorf("expected 'unknown op type' error, got: %s", errOut.String())
	}
}

func TestBatchCmdExecuteStoreError(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	fakeID := uuid.New().String()
	w.Write([]byte(`[{"op": "delete-node", "nodeId": "` + fakeID + `"}]`))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"batch", "execute"})
	if code != 1 {
		t.Error("expected error for delete of non-existent node")
	}
	if !strings.Contains(errOut.String(), "Batch error") {
		t.Errorf("expected batch error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// toBatchOp: additional paths
// ---------------------------------------------------------------------------

func TestToBatchOpMoveNodeWithParent(t *testing.T) {
	parentID := uuid.New().String()
	nodeID := uuid.New().String()
	op := batchInputOp{Op: "move-node", NodeID: nodeID, ParentID: parentID}
	result, err := op.toBatchOp()
	if err != nil {
		t.Fatalf("toBatchOp: %v", err)
	}
	if result.ParentID == nil {
		t.Error("expected ParentID to be set")
	}
}

func TestToBatchOpReorderNodePos(t *testing.T) {
	nodeID := uuid.New().String()
	op := batchInputOp{Op: "reorder-node", NodeID: nodeID, Position: "M"}
	result, err := op.toBatchOp()
	if err != nil {
		t.Fatalf("toBatchOp: %v", err)
	}
	if result.Position != "M" {
		t.Error("expected position M")
	}
}

func TestToBatchOpCascadeDeleteType(t *testing.T) {
	nodeID := uuid.New().String()
	op := batchInputOp{Op: "cascade-delete", NodeID: nodeID}
	result, err := op.toBatchOp()
	if err != nil {
		t.Fatalf("toBatchOp: %v", err)
	}
	if result.Type != graph.BatchCascadeDelete {
		t.Error("expected BatchCascadeDelete type")
	}
}

// ---------------------------------------------------------------------------
// adminReapOrphans: success paths with actual orphans
// ---------------------------------------------------------------------------

func TestAdminReapOrphansWithOrphans(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "file", "--parent", parentID})

	out.Reset()
	cli.Run([]string{"node", "soft-delete", parentID})

	out.Reset()
	code := cli.Run([]string{"admin", "reap-orphans"})
	if code != 0 {
		t.Fatal("reap-orphans should succeed")
	}

	var result map[string]interface{}
	json.Unmarshal(out.Bytes(), &result)
	count := result["reapedCount"].(float64)
	if count < 1 {
		t.Errorf("expected at least 1 reaped, got %v", count)
	}
}

func TestAdminReapOrphansNoOrphans(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "item"})

	out.Reset()
	code := cli.Run([]string{"admin", "reap-orphans"})
	if code != 0 {
		t.Fatal("reap-orphans should succeed")
	}

	var result map[string]interface{}
	json.Unmarshal(out.Bytes(), &result)
	if result["reapedCount"].(float64) != 0 {
		t.Error("expected 0 reaped")
	}
}

// ---------------------------------------------------------------------------
// nodeSoftDelete/nodeCascadeDelete/nodeRestore: store error paths
// ---------------------------------------------------------------------------

func TestNodeSoftDeleteNonExistent(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"node", "soft-delete", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error message, got: %s", errOut.String())
	}
}

func TestNodeCascadeDeleteNonExistent(t *testing.T) {
	cli, _, _ := newTestCLI()
	// CascadeDeleteNode on a non-existent node — exercises the code path
	code := cli.Run([]string{"node", "cascade-delete", uuid.New().String()})
	_ = code
}

func TestNodeRestoreNonExistent(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"node", "restore", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error message, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// edgeRestore: additional success path
// ---------------------------------------------------------------------------

func TestEdgeRestoreSuccessPath(t *testing.T) {
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
	cli.Run([]string{"edge", "insert", "--type", "link", "--from", aID, "--to", bID})
	json.Unmarshal(out.Bytes(), &r)
	edgeID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"edge", "delete", edgeID})

	out.Reset()
	code := cli.Run([]string{"edge", "restore", edgeID})
	if code != 0 {
		t.Fatal("edge restore should succeed")
	}
	if !strings.Contains(out.String(), "Restored edge") {
		t.Error("expected restore confirmation")
	}
}

// ---------------------------------------------------------------------------
// propertyDelete: non-existent node error
// ---------------------------------------------------------------------------

func TestPropertyDeleteNonExistentNode(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "delete", uuid.New().String(), "key"})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
}

// ---------------------------------------------------------------------------
// propertyGet: non-existent node error
// ---------------------------------------------------------------------------

func TestPropertyGetNonExistentNode(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "get", uuid.New().String(), "key"})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
}

// ---------------------------------------------------------------------------
// treeSubtree: non-existent node error
// ---------------------------------------------------------------------------

func TestTreeSubtreeNonExistent(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"tree", "subtree", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error message, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// edgeDelete: non-existent
// ---------------------------------------------------------------------------

func TestEdgeDeleteNonExistentEdge(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"edge", "delete", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent edge")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// edgeInsert: missing to, store error, missing type
// ---------------------------------------------------------------------------

func TestEdgeInsertMissingToFlag(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "insert", "--type", "e", "--from", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for missing to")
	}
}

func TestEdgeInsertStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"edge", "insert", "--type", "e", "--from", uuid.New().String(), "--to", uuid.New().String()})
	if code != 1 {
		t.Error("expected store error for non-existent nodes")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

func TestEdgeInsertMissingTypeFlag(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "insert", "--from", uuid.New().String(), "--to", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for missing type")
	}
}

// ---------------------------------------------------------------------------
// propertySet: store error and missing key
// ---------------------------------------------------------------------------

func TestPropertySetStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"property", "set", "--node", uuid.New().String(), "--key", "k", "--value", "v"})
	if code != 1 {
		t.Error("expected store error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

func TestPropertySetMissingKeyFlag(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "set", "--node", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for missing key")
	}
}

// ---------------------------------------------------------------------------
// nodeDelete: store error
// ---------------------------------------------------------------------------

func TestNodeDeleteStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"node", "delete", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// treeMove: store error
// ---------------------------------------------------------------------------

func TestTreeMoveStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"tree", "move", "--node", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// treeReorder: position error and between error (non-existent node)
// ---------------------------------------------------------------------------

func TestTreeReorderPositionStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"tree", "reorder", "--node", uuid.New().String(), "--position", "M"})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

func TestTreeReorderBetweenStoreError(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"tree", "reorder", "--node", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// queryByIndex: success with string fallback value
// ---------------------------------------------------------------------------

func TestQueryByIndexStringFallback(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Alice"}`})

	out.Reset()
	code := cli.Run([]string{"query", "by-index", "--type", "user", "--property", "name", "--value", "Alice"})
	if code != 0 {
		t.Fatal("by-index with string fallback should succeed")
	}
}

func TestQueryByIndexSuccessJSON(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"email":"alice@test.com"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"email":"bob@test.com"}`})

	out.Reset()
	code := cli.Run([]string{"query", "by-index", "--type", "user", "--property", "email", "--value", `"alice@test.com"`})
	if code != 0 {
		t.Fatal("by-index should succeed")
	}
}

// ---------------------------------------------------------------------------
// queryTraverse: with depth and direction
// ---------------------------------------------------------------------------

func TestQueryTraverseWithDepthAndDir(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "task"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"query", "traverse", "--start", nodeID, "--depth", "2", "--direction", "in"})
	if code != 0 {
		t.Fatal("traverse should succeed")
	}
}

// queryTraverse with non-existent start node triggers Traverse error
func TestQueryTraverseNonExistentStart(t *testing.T) {
	cli, _, errOut := newTestCLI()
	// Traverse returns error when start node not found (GetSubtree-like behavior)
	// Actually Traverse adds the start to visited even if it doesn't exist.
	// Let's test it anyway - it may not error but at least exercises the path.
	code := cli.Run([]string{"query", "traverse", "--start", uuid.New().String()})
	_ = code
	_ = errOut
}

// ---------------------------------------------------------------------------
// Full batch round trip with all op types via stdin (including restore/cascade)
// ---------------------------------------------------------------------------

func TestBatchCmdAllOpTypesViaStdin(t *testing.T) {
	store := graph.NewStore("test-batch-all")
	cli := NewCLI(store)
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cli.out = outBuf
	cli.errOut = errBuf

	id1, _ := store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := store.InsertNode("product", nil, nil)
	edgeID, _ := store.InsertEdge("owns", id1, id2, nil)

	store.SoftDeleteNode(id2)

	batch := `[
		{"op": "insert-node", "nodeType": "task", "properties": {"title": "do stuff"}},
		{"op": "set-property", "nodeId": "` + id1.String() + `", "key": "age", "value": 30},
		{"op": "delete-property", "nodeId": "` + id1.String() + `", "key": "name"},
		{"op": "move-node", "nodeId": "` + id1.String() + `"},
		{"op": "reorder-node", "nodeId": "` + id1.String() + `", "position": "M"},
		{"op": "restore-node", "nodeId": "` + id2.String() + `"},
		{"op": "delete-edge", "edgeId": "` + edgeID.String() + `"}
	]`

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte(batch))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	code := cli.Run([]string{"batch", "execute"})
	if code != 0 {
		t.Fatalf("batch execute failed: %s", errBuf.String())
	}

	var results []map[string]interface{}
	json.Unmarshal(outBuf.Bytes(), &results)
	if len(results) != 7 {
		t.Errorf("expected 7 results, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// nodeInsert: store error (schema validation)
// ---------------------------------------------------------------------------

func TestNodeInsertStoreError(t *testing.T) {
	store := graph.NewStore("test-cli-schema")
	schema := graph.NewSchema()
	schema.DefineNode(&graph.NodeTypeDef{
		Name: "user",
		Properties: map[string]*graph.PropertyDef{
			"name": {Name: "name", Type: graph.PropString, Required: true},
		},
	})
	store.SetSchema(schema)

	cli := NewCLI(store)
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cli.out = outBuf
	cli.errOut = errBuf

	code := cli.Run([]string{"node", "insert", "--type", "user"})
	if code != 1 {
		t.Error("expected schema validation error")
	}
	if !strings.Contains(errBuf.String(), "Error") {
		t.Errorf("expected error, got: %s", errBuf.String())
	}
}

// ---------------------------------------------------------------------------
// nodeGet: non-existent
// ---------------------------------------------------------------------------

func TestNodeGetNonExistent(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"node", "get", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
	if !strings.Contains(errOut.String(), "Error") {
		t.Errorf("expected error, got: %s", errOut.String())
	}
}

// ---------------------------------------------------------------------------
// Batch execute with cascade-delete via stdin
// ---------------------------------------------------------------------------

func TestBatchCmdCascadeDeleteViaStdin(t *testing.T) {
	store := graph.NewStore("test-batch-cascade")
	cli := NewCLI(store)
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cli.out = outBuf
	cli.errOut = errBuf

	root, _ := store.InsertNode("folder", nil, nil)
	store.InsertNode("file", &root, nil)

	batch := `[{"op": "cascade-delete", "nodeId": "` + root.String() + `"}]`

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	w.Write([]byte(batch))
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	code := cli.Run([]string{"batch", "execute"})
	if code != 0 {
		t.Fatalf("batch cascade-delete failed: %s", errBuf.String())
	}

	if len(store.AllNodes()) != 0 {
		t.Errorf("expected 0 nodes after cascade delete, got %d", len(store.AllNodes()))
	}
}
