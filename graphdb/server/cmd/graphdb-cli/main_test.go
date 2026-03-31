package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/graph"
)

func newTestCLI() (*CLI, *bytes.Buffer, *bytes.Buffer) {
	store := graph.NewStore("test-replica")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cli := NewCLI(store)
	cli.out = out
	cli.errOut = errOut
	return cli, out, errOut
}

func TestNodeInsertAndGet(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert a node
	code := cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Alice","age":30}`})
	if code != 0 {
		t.Fatalf("insert failed: %s", out.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	nodeID := result["id"].(string)
	if _, err := uuid.Parse(nodeID); err != nil {
		t.Fatalf("invalid UUID: %v", err)
	}

	// Get the node
	out.Reset()
	code = cli.Run([]string{"node", "get", nodeID})
	if code != 0 {
		t.Fatalf("get failed with code %d", code)
	}

	var node map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	props := node["properties"].(map[string]interface{})
	if props["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", props["name"])
	}
}

func TestNodeDeleteAndRestore(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Bob"}`})
	var result map[string]interface{}
	json.Unmarshal(out.Bytes(), &result)
	nodeID := result["id"].(string)

	// Soft delete
	out.Reset()
	code := cli.Run([]string{"node", "soft-delete", nodeID})
	if code != 0 {
		t.Fatalf("soft-delete failed")
	}

	// Verify node is gone from regular get
	out.Reset()
	code = cli.Run([]string{"node", "get", nodeID})
	if code == 0 {
		t.Fatal("expected get to fail after soft delete")
	}

	// Check it shows up in deleted nodes
	out.Reset()
	code = cli.Run([]string{"admin", "deleted-nodes"})
	if code != 0 {
		t.Fatal("deleted-nodes failed")
	}
	if !strings.Contains(out.String(), nodeID) {
		t.Error("deleted node should appear in deleted-nodes list")
	}

	// Restore
	out.Reset()
	code = cli.Run([]string{"node", "restore", nodeID})
	if code != 0 {
		t.Fatalf("restore failed")
	}

	// Verify node is back
	out.Reset()
	code = cli.Run([]string{"node", "get", nodeID})
	if code != 0 {
		t.Fatal("expected get to succeed after restore")
	}
}

func TestCascadeDelete(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert parent
	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	// Insert children
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "file", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	child1ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "file", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	child2ID := r["id"].(string)

	// Cascade delete parent
	out.Reset()
	code := cli.Run([]string{"node", "cascade-delete", parentID})
	if code != 0 {
		t.Fatalf("cascade-delete failed")
	}

	// Verify all are deleted
	out.Reset()
	code = cli.Run([]string{"node", "get", parentID})
	if code == 0 {
		t.Error("parent should be deleted")
	}
	out.Reset()
	code = cli.Run([]string{"node", "get", child1ID})
	if code == 0 {
		t.Error("child1 should be deleted")
	}
	out.Reset()
	code = cli.Run([]string{"node", "get", child2ID})
	if code == 0 {
		t.Error("child2 should be deleted")
	}
}

func TestEdgeInsertAndGet(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert two nodes
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"A"}`})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"B"}`})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	// Insert edge
	out.Reset()
	code := cli.Run([]string{"edge", "insert", "--type", "follows", "--from", aID, "--to", bID, "--props", `{"since":"2024"}`})
	if code != 0 {
		t.Fatalf("edge insert failed")
	}
	json.Unmarshal(out.Bytes(), &r)
	edgeID := r["id"].(string)
	if _, err := uuid.Parse(edgeID); err != nil {
		t.Fatalf("invalid edge ID: %v", err)
	}

	// Get outgoing edges
	out.Reset()
	code = cli.Run([]string{"edge", "get", aID, "--direction", "out"})
	if code != 0 {
		t.Fatal("edge get failed")
	}
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	// Get incoming edges
	out.Reset()
	code = cli.Run([]string{"edge", "get", bID, "--direction", "in"})
	if code != 0 {
		t.Fatal("edge get failed")
	}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Fatalf("expected 1 incoming edge, got %d", len(edges))
	}
}

func TestPropertySetGetDelete(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert node
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Test"}`})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Set property
	out.Reset()
	code := cli.Run([]string{"property", "set", "--node", nodeID, "--key", "email", "--value", `"test@example.com"`})
	if code != 0 {
		t.Fatalf("property set failed")
	}

	// Get property
	out.Reset()
	code = cli.Run([]string{"property", "get", nodeID, "email"})
	if code != 0 {
		t.Fatal("property get failed")
	}
	if !strings.Contains(out.String(), "test@example.com") {
		t.Errorf("expected email value, got: %s", out.String())
	}

	// Delete property
	out.Reset()
	code = cli.Run([]string{"property", "delete", nodeID, "email"})
	if code != 0 {
		t.Fatal("property delete failed")
	}

	// Verify deleted
	out.Reset()
	code = cli.Run([]string{"property", "get", nodeID, "email"})
	if code == 0 {
		t.Error("expected property get to fail after delete")
	}
}

func TestTreeOperations(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create hierarchy: root -> child1, child2
	cli.Run([]string{"node", "insert", "--type", "folder", "--props", `{"name":"root"}`})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	rootID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "folder", "--parent", rootID, "--props", `{"name":"child1"}`})
	json.Unmarshal(out.Bytes(), &r)
	child1ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "folder", "--parent", rootID, "--props", `{"name":"child2"}`})
	json.Unmarshal(out.Bytes(), &r)

	// Test children
	out.Reset()
	cli.Run([]string{"tree", "children", rootID})
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 children, got %d", len(nodes))
	}

	// Test parent
	out.Reset()
	cli.Run([]string{"tree", "parent", child1ID})
	var parent map[string]interface{}
	json.Unmarshal(out.Bytes(), &parent)
	if parent["id"] != rootID {
		t.Errorf("expected parent to be root")
	}

	// Test roots
	out.Reset()
	cli.Run([]string{"tree", "roots"})
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 1 {
		t.Errorf("expected 1 root, got %d", len(nodes))
	}

	// Test subtree
	out.Reset()
	cli.Run([]string{"tree", "subtree", rootID})
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes in subtree, got %d", len(nodes))
	}

	// Test ancestors
	out.Reset()
	cli.Run([]string{"tree", "ancestors", child1ID})
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 1 {
		t.Errorf("expected 1 ancestor, got %d", len(nodes))
	}
}

func TestTreeMoveAndReorder(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create root and two children
	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	rootID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", rootID})
	json.Unmarshal(out.Bytes(), &r)
	item1ID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", rootID})
	json.Unmarshal(out.Bytes(), &r)
	item2ID := r["id"].(string)

	// Reorder: place item2 before item1
	out.Reset()
	code := cli.Run([]string{"tree", "reorder", "--node", item2ID, "--before", item1ID})
	if code != 0 {
		t.Fatal("reorder failed")
	}

	// Get ordered children
	out.Reset()
	cli.Run([]string{"tree", "ordered-children", rootID})
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 children, got %d", len(nodes))
	}
	// item2 should come first
	if nodes[0]["id"] != item2ID {
		t.Errorf("expected item2 first after reorder, got %v", nodes[0]["id"])
	}

	// Move item1 to root (no parent)
	out.Reset()
	code = cli.Run([]string{"tree", "move", "--node", item1ID, "--parent", "none"})
	if code != 0 {
		t.Fatal("move failed")
	}

	// Verify roots
	out.Reset()
	cli.Run([]string{"tree", "roots"})
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 roots after move, got %d", len(nodes))
	}
}

func TestQueryByType(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Alice"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Bob"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "product", "--props", `{"name":"Widget"}`})

	out.Reset()
	cli.Run([]string{"query", "by-type", "user"})
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 users, got %d", len(nodes))
	}
}

func TestQueryTraverse(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create A -> B -> C via edges
	cli.Run([]string{"node", "insert", "--type", "task", "--props", `{"name":"A"}`})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	aID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "task", "--props", `{"name":"B"}`})
	json.Unmarshal(out.Bytes(), &r)
	bID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "task", "--props", `{"name":"C"}`})
	json.Unmarshal(out.Bytes(), &r)
	cID := r["id"].(string)
	_ = cID

	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "depends_on", "--from", aID, "--to", bID})
	out.Reset()
	cli.Run([]string{"edge", "insert", "--type", "depends_on", "--from", bID, "--to", cID})

	// Traverse from A
	out.Reset()
	code := cli.Run([]string{"query", "traverse", "--start", aID, "--edge-type", "depends_on", "--direction", "out", "--depth", "5"})
	if code != 0 {
		t.Fatal("traverse failed")
	}
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes in traversal, got %d", len(nodes))
	}
}

func TestBatchExecute(t *testing.T) {
	cli, out, _ := newTestCLI()

	batch := `[
		{"op": "insert-node", "nodeType": "user", "properties": {"name": "Alice"}},
		{"op": "insert-node", "nodeType": "user", "properties": {"name": "Bob"}},
		{"op": "insert-node", "nodeType": "product", "properties": {"name": "Widget"}}
	]`

	// We need to override stdin for the batch command
	oldStdin := cli.out
	_ = oldStdin

	// Instead, test via store directly since CLI reads from os.Stdin
	store := graph.NewStore("test-batch")
	ops := []graph.BatchOp{
		{Type: graph.BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Alice"}},
		{Type: graph.BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Bob"}},
		{Type: graph.BatchInsertNode, NodeType: "product", Properties: map[string]interface{}{"name": "Widget"}},
	}

	result, err := store.ExecuteBatch(ops)
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	if len(result.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result.Results))
	}
	for i, r := range result.Results {
		if r.ResultID == uuid.Nil {
			t.Errorf("result %d has nil ID", i)
		}
	}

	// Verify nodes exist
	users := store.GetNodesByType("user")
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	_ = batch // used for reference
	_ = out
}

func TestOrphanReaping(t *testing.T) {
	store := graph.NewStore("test-reap")

	// Create hierarchy
	parentID, _ := store.InsertNode("folder", nil, map[string]interface{}{"name": "parent"})
	child1ID, _ := store.InsertNode("file", &parentID, map[string]interface{}{"name": "child1"})
	child2ID, _ := store.InsertNode("file", &parentID, map[string]interface{}{"name": "child2"})
	grandchildID, _ := store.InsertNode("file", &child1ID, map[string]interface{}{"name": "grandchild"})
	_ = child2ID
	_ = grandchildID

	// Soft delete the parent
	if err := store.SoftDeleteNode(parentID); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}

	// Reap orphans
	reaped, err := store.ReapOrphans()
	if err != nil {
		t.Fatalf("reap failed: %v", err)
	}

	// Should have reaped child1, child2, and grandchild (cascading)
	if len(reaped) < 2 {
		t.Errorf("expected at least 2 reaped nodes, got %d", len(reaped))
	}

	// All nodes should be deleted
	allNodes := store.AllNodes()
	if len(allNodes) != 0 {
		t.Errorf("expected 0 live nodes, got %d", len(allNodes))
	}
}

func TestFractionalIndexOrdering(t *testing.T) {
	store := graph.NewStore("test-ordering")

	parentID, _ := store.InsertNode("list", nil, map[string]interface{}{})

	id1, _ := store.InsertNode("item", &parentID, map[string]interface{}{"name": "first"})
	id2, _ := store.InsertNode("item", &parentID, map[string]interface{}{"name": "second"})
	id3, _ := store.InsertNode("item", &parentID, map[string]interface{}{"name": "third"})

	// Default order should be insertion order
	children := store.GetOrderedChildren(parentID)
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	if children[0].ID != id1 || children[1].ID != id2 || children[2].ID != id3 {
		t.Error("default order should be insertion order")
	}

	// Reorder: place third before first
	if err := store.ReorderBetween(id3, nil, &id1); err != nil {
		t.Fatalf("reorder failed: %v", err)
	}

	children = store.GetOrderedChildren(parentID)
	if children[0].ID != id3 {
		t.Error("id3 should be first after reorder")
	}
}

func TestAdminStats(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Alice"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"name":"Bob"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "product", "--props", `{"name":"Widget"}`})

	out.Reset()
	code := cli.Run([]string{"admin", "stats"})
	if code != 0 {
		t.Fatal("stats failed")
	}

	var stats map[string]interface{}
	json.Unmarshal(out.Bytes(), &stats)
	if stats["totalNodes"].(float64) != 3 {
		t.Errorf("expected 3 nodes, got %v", stats["totalNodes"])
	}
}

func TestNodeList(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user"})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user"})
	out.Reset()

	code := cli.Run([]string{"node", "list"})
	if code != 0 {
		t.Fatal("list failed")
	}
	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestEdgeList(t *testing.T) {
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
	code := cli.Run([]string{"edge", "list"})
	if code != 0 {
		t.Fatal("edge list failed")
	}
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestHelpCommand(t *testing.T) {
	cli, out, _ := newTestCLI()
	code := cli.Run([]string{"help"})
	if code != 0 {
		t.Fatal("help should return 0")
	}
	if !strings.Contains(out.String(), "graphdb-cli") {
		t.Error("help output should mention graphdb-cli")
	}
}

func TestUnknownCommand(t *testing.T) {
	cli, _, errOut := newTestCLI()
	code := cli.Run([]string{"nonexistent"})
	if code != 1 {
		t.Error("unknown command should return 1")
	}
	if !strings.Contains(errOut.String(), "Unknown command") {
		t.Error("should print unknown command error")
	}
}

func TestBatchOperationsViaStore(t *testing.T) {
	store := graph.NewStore("batch-test")

	// Batch: insert 2 nodes, then set a property, then insert edge
	ops := []graph.BatchOp{
		{Type: graph.BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Alice"}},
		{Type: graph.BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Bob"}},
	}

	result, err := store.ExecuteBatch(ops)
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}

	aliceID := result.Results[0].ResultID
	bobID := result.Results[1].ResultID

	// Second batch: set property and insert edge
	ops2 := []graph.BatchOp{
		{Type: graph.BatchSetProperty, NodeID: aliceID, Key: "email", Value: "alice@test.com"},
		{Type: graph.BatchInsertEdge, EdgeType: "knows", FromID: aliceID, ToID: bobID},
	}

	result2, err := store.ExecuteBatch(ops2)
	if err != nil {
		t.Fatalf("batch 2 failed: %v", err)
	}

	edgeID := result2.Results[1].ResultID
	if edgeID == uuid.Nil {
		t.Error("expected edge ID")
	}

	// Verify
	node, _ := store.GetNode(aliceID)
	if node.Properties["email"] != "alice@test.com" {
		t.Error("expected email property")
	}

	edges := store.GetOutEdges(aliceID)
	if len(edges) != 1 {
		t.Error("expected 1 edge")
	}
}
