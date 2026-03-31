package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNodeDelete(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Insert node
	cli.Run([]string{"node", "insert", "--type", "user"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Delete node
	out.Reset()
	code := cli.Run([]string{"node", "delete", nodeID})
	if code != 0 {
		t.Fatalf("delete failed")
	}

	// Verify gone
	out.Reset()
	code = cli.Run([]string{"node", "get", nodeID})
	if code == 0 {
		t.Error("expected get to fail after delete")
	}
}

func TestNodeDeleteNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "delete"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestNodeDeleteBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "delete", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestNodeSoftDeleteNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "soft-delete"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestNodeCascadeDeleteNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "cascade-delete"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestNodeRestoreNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "restore"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestEdgeDelete(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create nodes and edge
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

	// Delete edge
	out.Reset()
	code := cli.Run([]string{"edge", "delete", edgeID})
	if code != 0 {
		t.Fatalf("edge delete failed")
	}

	// Verify gone
	out.Reset()
	code = cli.Run([]string{"edge", "list"})
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 0 {
		t.Errorf("expected 0 edges after delete, got %d", len(edges))
	}
}

func TestEdgeDeleteNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "delete"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestEdgeDeleteBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "delete", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestEdgeRestoreNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "restore"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestEdgeRestoreBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "restore", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestEdgeRestore(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create nodes and edge
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

	// Delete edge
	out.Reset()
	cli.Run([]string{"edge", "delete", edgeID})

	// Restore edge
	out.Reset()
	code := cli.Run([]string{"edge", "restore", edgeID})
	if code != 0 {
		t.Fatalf("edge restore failed")
	}

	// Verify restored
	out.Reset()
	cli.Run([]string{"edge", "list"})
	var edges []map[string]interface{}
	json.Unmarshal(out.Bytes(), &edges)
	if len(edges) != 1 {
		t.Errorf("expected 1 edge after restore, got %d", len(edges))
	}
}

func TestQueryByIndex(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"email":"alice@test.com"}`})
	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "user", "--props", `{"email":"bob@test.com"}`})

	out.Reset()
	code := cli.Run([]string{"query", "by-index", "--type", "user", "--property", "email", "--value", `"alice@test.com"`})
	if code != 0 {
		t.Fatal("by-index failed")
	}

	var nodes []map[string]interface{}
	json.Unmarshal(out.Bytes(), &nodes)
	if len(nodes) != 1 {
		t.Errorf("expected 1 result, got %d", len(nodes))
	}
}

func TestQueryByIndexNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query", "by-index"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestAdminReapOrphans(t *testing.T) {
	cli, out, _ := newTestCLI()

	// Create parent with children
	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "file", "--parent", parentID})

	// Soft delete parent
	out.Reset()
	cli.Run([]string{"node", "soft-delete", parentID})

	// Reap orphans
	out.Reset()
	code := cli.Run([]string{"admin", "reap-orphans"})
	if code != 0 {
		t.Fatal("reap-orphans failed")
	}
	if !strings.Contains(out.String(), "reaped") {
		t.Error("should report reaped count")
	}
}

func TestNodeInsertNoType(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "insert"})
	if code != 1 {
		t.Error("expected error for no type")
	}
}

func TestNodeGetNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "get"})
	if code != 1 {
		t.Error("expected error for no args")
	}
}

func TestNodeGetBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "get", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestEdgeInsertMissingArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "insert"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestEdgeGetNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "get"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestPropertySetMissingArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "set"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestPropertyGetMissingArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "get"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestPropertyDeleteMissingArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "delete"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeChildrenNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "children"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeOrderedChildrenNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "ordered-children"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeParentNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "parent"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeSubtreeNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "subtree"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeAncestorsNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "ancestors"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeMoveNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "move"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestTreeReorderNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "reorder"})
	if code != 1 {
		t.Error("expected error for missing args")
	}
}

func TestQueryTraverseNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query", "traverse"})
	if code != 1 {
		t.Error("expected error for missing start")
	}
}

func TestQueryCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"query"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestPropertyCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestEdgeCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestTreeCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestNodeCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestBatchCmdNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"batch"})
	if code != 1 {
		t.Error("expected error for missing subcommand")
	}
}

func TestBatchCmdUnknown(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"batch", "unknown"})
	if code != 1 {
		t.Error("expected error for unknown subcommand")
	}
}

func TestBatchExecuteWithStdin(t *testing.T) {
	cli, out, _ := newTestCLI()

	batchJSON := `[{"op":"insert-node","nodeType":"item","properties":{"name":"batch1"}},{"op":"insert-node","nodeType":"item","properties":{"name":"batch2"}}]`

	// Temporarily replace os.Stdin with a pipe containing our batch JSON
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		w.Write([]byte(batchJSON))
		w.Close()
	}()
	defer func() { os.Stdin = oldStdin }()

	out.Reset()
	code := cli.Run([]string{"batch", "execute"})
	if code != 0 {
		t.Fatalf("batch execute failed")
	}
}

func TestNodeGetNotFound(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "get", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
}

func TestNodeDeleteNotFound(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"node", "delete", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent node")
	}
}

func TestEdgeDeleteNotFound(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "delete", uuid.New().String()})
	if code != 1 {
		t.Error("expected error for non-existent edge")
	}
}

func TestTreeChildrenBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"tree", "children", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestTreeReorderWithPosition(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parentID := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parentID})
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	out.Reset()
	code := cli.Run([]string{"tree", "reorder", "--node", nodeID, "--position", "M"})
	if code != 0 {
		t.Fatal("reorder with position failed")
	}
}

func TestTreeMoveToParent(t *testing.T) {
	cli, out, _ := newTestCLI()

	cli.Run([]string{"node", "insert", "--type", "folder"})
	var r map[string]interface{}
	json.Unmarshal(out.Bytes(), &r)
	parent1 := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "folder"})
	json.Unmarshal(out.Bytes(), &r)
	parent2 := r["id"].(string)

	out.Reset()
	cli.Run([]string{"node", "insert", "--type", "item", "--parent", parent1})
	json.Unmarshal(out.Bytes(), &r)
	nodeID := r["id"].(string)

	// Move to parent2
	out.Reset()
	code := cli.Run([]string{"tree", "move", "--node", nodeID, "--parent", parent2})
	if code != 0 {
		t.Fatal("move to new parent failed")
	}
}

func TestEdgeGetBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"edge", "get", "bad-uuid"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestPropertyGetBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "get", "bad-uuid", "key"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestPropertyDeleteBadUUID(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{"property", "delete", "bad-uuid", "key"})
	if code != 1 {
		t.Error("expected error for bad UUID")
	}
}

func TestNoArgs(t *testing.T) {
	cli, _, _ := newTestCLI()
	code := cli.Run([]string{})
	if code != 1 {
		t.Error("expected error for no args")
	}
}
