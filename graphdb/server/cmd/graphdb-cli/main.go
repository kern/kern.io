// graphdb-cli is a feature-complete command-line interface for the GraphDB graph database.
//
// Usage:
//
//	graphdb-cli <command> [flags]
//
// Commands:
//
//	node insert, get, delete, restore, list, soft-delete, cascade-delete
//	edge insert, get, delete, restore, list
//	property set, get, delete
//	tree children, parent, roots, subtree, ancestors, move, reorder
//	query by-type, by-index, traverse
//	batch execute (reads JSON from stdin)
//	admin reap-orphans, deleted-nodes, stats
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
)

// CLI wraps a graph.Store and exposes command-line operations.
type CLI struct {
	store  *graph.Store
	out    io.Writer
	errOut io.Writer
}

// NewCLI creates a new CLI with the given store.
func NewCLI(store *graph.Store) *CLI {
	return &CLI{
		store:  store,
		out:    os.Stdout,
		errOut: os.Stderr,
	}
}

func (c *CLI) printf(format string, args ...interface{}) {
	fmt.Fprintf(c.out, format, args...)
}

func (c *CLI) errorf(format string, args ...interface{}) {
	fmt.Fprintf(c.errOut, format, args...)
}

func (c *CLI) printJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		c.errorf("Error marshaling JSON: %v\n", err)
		return
	}
	c.printf("%s\n", string(b))
}

// Run executes a CLI command.
func (c *CLI) Run(args []string) int {
	if len(args) < 1 {
		c.printUsage()
		return 1
	}

	switch args[0] {
	case "node":
		return c.nodeCmd(args[1:])
	case "edge":
		return c.edgeCmd(args[1:])
	case "property", "prop":
		return c.propertyCmd(args[1:])
	case "tree":
		return c.treeCmd(args[1:])
	case "query":
		return c.queryCmd(args[1:])
	case "batch":
		return c.batchCmd(args[1:])
	case "admin":
		return c.adminCmd(args[1:])
	case "help", "--help", "-h":
		c.printUsage()
		return 0
	default:
		c.errorf("Unknown command: %s\n", args[0])
		c.printUsage()
		return 1
	}
}

func (c *CLI) printUsage() {
	c.printf(`graphdb-cli - GraphDB command-line interface

Usage: graphdb-cli <command> <subcommand> [flags]

Commands:
  node      insert, get, delete, soft-delete, cascade-delete, restore, list
  edge      insert, get, delete, restore, list
  property  set, get, delete
  tree      children, ordered-children, parent, roots, subtree, ancestors, move, reorder
  query     by-type, by-index, traverse
  batch     execute (reads JSON from stdin)
  admin     reap-orphans, deleted-nodes, stats
  help      Show this help
`)
}

// --- Node commands ---

func (c *CLI) nodeCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node <insert|get|delete|soft-delete|cascade-delete|restore|list>\n")
		return 1
	}

	switch args[0] {
	case "insert":
		return c.nodeInsert(args[1:])
	case "get":
		return c.nodeGet(args[1:])
	case "delete":
		return c.nodeDelete(args[1:])
	case "soft-delete":
		return c.nodeSoftDelete(args[1:])
	case "cascade-delete":
		return c.nodeCascadeDelete(args[1:])
	case "restore":
		return c.nodeRestore(args[1:])
	case "list":
		return c.nodeList(args[1:])
	default:
		c.errorf("Unknown node command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) nodeInsert(args []string) int {
	flags := parseFlags(args)
	nodeType := flags.Get("type")
	if nodeType == "" {
		c.errorf("Usage: graphdb-cli node insert --type <type> [--parent <id>] [--props '{...}']\n")
		return 1
	}

	var parentID *uuid.UUID
	if p := flags.Get("parent"); p != "" {
		id, err := uuid.Parse(p)
		if err != nil {
			c.errorf("Invalid parent ID: %v\n", err)
			return 1
		}
		parentID = &id
	}

	props := make(map[string]interface{})
	if p := flags.Get("props"); p != "" {
		if err := json.Unmarshal([]byte(p), &props); err != nil {
			c.errorf("Invalid JSON properties: %v\n", err)
			return 1
		}
	}

	id, err := c.store.InsertNode(nodeType, parentID, props)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(map[string]interface{}{"id": id.String()})
	return 0
}

func (c *CLI) nodeGet(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node get <id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	node, err := c.store.GetNode(id)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(node)
	return 0
}

func (c *CLI) nodeDelete(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node delete <id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.DeleteNode(id); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Deleted node %s\n", id)
	return 0
}

func (c *CLI) nodeSoftDelete(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node soft-delete <id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.SoftDeleteNode(id); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Soft-deleted node %s\n", id)
	return 0
}

func (c *CLI) nodeCascadeDelete(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node cascade-delete <id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.CascadeDeleteNode(id); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Cascade-deleted node %s and descendants\n", id)
	return 0
}

func (c *CLI) nodeRestore(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli node restore <id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.RestoreNode(id); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Restored node %s\n", id)
	return 0
}

func (c *CLI) nodeList(args []string) int {
	nodes := c.store.AllNodes()
	c.printJSON(nodes)
	return 0
}

// --- Edge commands ---

func (c *CLI) edgeCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli edge <insert|get|delete|restore|list>\n")
		return 1
	}

	switch args[0] {
	case "insert":
		return c.edgeInsert(args[1:])
	case "get":
		return c.edgeGet(args[1:])
	case "delete":
		return c.edgeDelete(args[1:])
	case "restore":
		return c.edgeRestore(args[1:])
	case "list":
		return c.edgeList(args[1:])
	default:
		c.errorf("Unknown edge command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) edgeInsert(args []string) int {
	flags := parseFlags(args)
	edgeType := flags.Get("type")
	from := flags.Get("from")
	to := flags.Get("to")
	if edgeType == "" || from == "" || to == "" {
		c.errorf("Usage: graphdb-cli edge insert --type <type> --from <id> --to <id> [--props '{...}']\n")
		return 1
	}

	fromID, err := uuid.Parse(from)
	if err != nil {
		c.errorf("Invalid from ID: %v\n", err)
		return 1
	}
	toID, err := uuid.Parse(to)
	if err != nil {
		c.errorf("Invalid to ID: %v\n", err)
		return 1
	}

	props := make(map[string]interface{})
	if p := flags.Get("props"); p != "" {
		if err := json.Unmarshal([]byte(p), &props); err != nil {
			c.errorf("Invalid JSON properties: %v\n", err)
			return 1
		}
	}

	id, err := c.store.InsertEdge(edgeType, fromID, toID, props)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(map[string]interface{}{"id": id.String()})
	return 0
}

func (c *CLI) edgeGet(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli edge get <node-id> [--direction out|in|both] [--type <edge-type>]\n")
		return 1
	}
	nodeID, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	flags := parseFlags(args[1:])
	dir := flags.Get("direction")
	if dir == "" {
		dir = "out"
	}
	edgeType := flags.Get("type")

	var edges []*crdt.MaterializedEdge
	switch dir {
	case "out":
		if edgeType != "" {
			edges = c.store.GetOutEdgesByType(nodeID, edgeType)
		} else {
			edges = c.store.GetOutEdges(nodeID)
		}
	case "in":
		if edgeType != "" {
			edges = c.store.GetInEdgesByType(nodeID, edgeType)
		} else {
			edges = c.store.GetInEdges(nodeID)
		}
	case "both":
		out := c.store.GetOutEdges(nodeID)
		in := c.store.GetInEdges(nodeID)
		edges = append(out, in...)
	}

	c.printJSON(edges)
	return 0
}

func (c *CLI) edgeDelete(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli edge delete <edge-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.DeleteEdge(id); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Deleted edge %s\n", id)
	return 0
}

func (c *CLI) edgeRestore(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli edge restore <edge-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	// Access walker directly for edge restore
	_, err = c.store.Walker().RestoreEdge(id)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Restored edge %s\n", id)
	return 0
}

func (c *CLI) edgeList(args []string) int {
	edges := c.store.AllEdges()
	c.printJSON(edges)
	return 0
}

// --- Property commands ---

func (c *CLI) propertyCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli property <set|get|delete>\n")
		return 1
	}

	switch args[0] {
	case "set":
		return c.propertySet(args[1:])
	case "get":
		return c.propertyGet(args[1:])
	case "delete":
		return c.propertyDelete(args[1:])
	default:
		c.errorf("Unknown property command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) propertySet(args []string) int {
	flags := parseFlags(args)
	nodeStr := flags.Get("node")
	key := flags.Get("key")
	val := flags.Get("value")
	if nodeStr == "" || key == "" {
		c.errorf("Usage: graphdb-cli property set --node <id> --key <key> --value <json-value>\n")
		return 1
	}
	nodeID, err := uuid.Parse(nodeStr)
	if err != nil {
		c.errorf("Invalid node ID: %v\n", err)
		return 1
	}

	// Try to parse value as JSON, fall back to string
	var value interface{}
	if err := json.Unmarshal([]byte(val), &value); err != nil {
		value = val
	}

	if err := c.store.SetProperty(nodeID, key, value); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Set %s = %v on node %s\n", key, value, nodeID)
	return 0
}

func (c *CLI) propertyGet(args []string) int {
	if len(args) < 2 {
		c.errorf("Usage: graphdb-cli property get <node-id> <key>\n")
		return 1
	}
	nodeID, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	node, err := c.store.GetNode(nodeID)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	val, ok := node.Properties[args[1]]
	if !ok {
		c.errorf("Property '%s' not found\n", args[1])
		return 1
	}
	c.printJSON(val)
	return 0
}

func (c *CLI) propertyDelete(args []string) int {
	if len(args) < 2 {
		c.errorf("Usage: graphdb-cli property delete <node-id> <key>\n")
		return 1
	}
	nodeID, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	if err := c.store.DeleteProperty(nodeID, args[1]); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Deleted property '%s' from node %s\n", args[1], nodeID)
	return 0
}

// --- Tree commands ---

func (c *CLI) treeCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree <children|ordered-children|parent|roots|subtree|ancestors|move|reorder>\n")
		return 1
	}

	switch args[0] {
	case "children":
		return c.treeChildren(args[1:])
	case "ordered-children":
		return c.treeOrderedChildren(args[1:])
	case "parent":
		return c.treeParent(args[1:])
	case "roots":
		return c.treeRoots(args[1:])
	case "subtree":
		return c.treeSubtree(args[1:])
	case "ancestors":
		return c.treeAncestors(args[1:])
	case "move":
		return c.treeMove(args[1:])
	case "reorder":
		return c.treeReorder(args[1:])
	default:
		c.errorf("Unknown tree command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) treeChildren(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree children <parent-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	children := c.store.GetChildren(id)
	c.printJSON(children)
	return 0
}

func (c *CLI) treeOrderedChildren(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree ordered-children <parent-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	children := c.store.GetOrderedChildren(id)
	c.printJSON(children)
	return 0
}

func (c *CLI) treeParent(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree parent <node-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	parent, ok := c.store.GetParent(id)
	if !ok {
		c.printf("null\n")
		return 0
	}
	c.printJSON(parent)
	return 0
}

func (c *CLI) treeRoots(args []string) int {
	roots := c.store.GetRoots()
	c.printJSON(roots)
	return 0
}

func (c *CLI) treeSubtree(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree subtree <node-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	subtree, err := c.store.GetSubtree(id)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(subtree)
	return 0
}

func (c *CLI) treeAncestors(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli tree ancestors <node-id>\n")
		return 1
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		c.errorf("Invalid ID: %v\n", err)
		return 1
	}
	ancestors := c.store.GetAncestors(id)
	c.printJSON(ancestors)
	return 0
}

func (c *CLI) treeMove(args []string) int {
	flags := parseFlags(args)
	nodeStr := flags.Get("node")
	parentStr := flags.Get("parent")
	if nodeStr == "" {
		c.errorf("Usage: graphdb-cli tree move --node <id> --parent <id|none>\n")
		return 1
	}
	nodeID, err := uuid.Parse(nodeStr)
	if err != nil {
		c.errorf("Invalid node ID: %v\n", err)
		return 1
	}
	var parentID *uuid.UUID
	if parentStr != "" && parentStr != "none" && parentStr != "null" {
		id, err := uuid.Parse(parentStr)
		if err != nil {
			c.errorf("Invalid parent ID: %v\n", err)
			return 1
		}
		parentID = &id
	}
	if err := c.store.MoveNode(nodeID, parentID); err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printf("Moved node %s\n", nodeID)
	return 0
}

func (c *CLI) treeReorder(args []string) int {
	flags := parseFlags(args)
	nodeStr := flags.Get("node")
	afterStr := flags.Get("after")
	beforeStr := flags.Get("before")
	posStr := flags.Get("position")

	if nodeStr == "" {
		c.errorf("Usage: graphdb-cli tree reorder --node <id> [--after <id>] [--before <id>] [--position <pos>]\n")
		return 1
	}
	nodeID, err := uuid.Parse(nodeStr)
	if err != nil {
		c.errorf("Invalid node ID: %v\n", err)
		return 1
	}

	if posStr != "" {
		if err := c.store.ReorderNode(nodeID, posStr); err != nil {
			c.errorf("Error: %v\n", err)
			return 1
		}
	} else {
		var afterID, beforeID *uuid.UUID
		if afterStr != "" {
			id, err := uuid.Parse(afterStr)
			if err != nil {
				c.errorf("Invalid after ID: %v\n", err)
				return 1
			}
			afterID = &id
		}
		if beforeStr != "" {
			id, err := uuid.Parse(beforeStr)
			if err != nil {
				c.errorf("Invalid before ID: %v\n", err)
				return 1
			}
			beforeID = &id
		}
		if err := c.store.ReorderBetween(nodeID, afterID, beforeID); err != nil {
			c.errorf("Error: %v\n", err)
			return 1
		}
	}
	c.printf("Reordered node %s\n", nodeID)
	return 0
}

// --- Query commands ---

func (c *CLI) queryCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli query <by-type|by-index|traverse>\n")
		return 1
	}

	switch args[0] {
	case "by-type":
		return c.queryByType(args[1:])
	case "by-index":
		return c.queryByIndex(args[1:])
	case "traverse":
		return c.queryTraverse(args[1:])
	default:
		c.errorf("Unknown query command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) queryByType(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli query by-type <node-type>\n")
		return 1
	}
	nodes := c.store.GetNodesByType(args[0])
	c.printJSON(nodes)
	return 0
}

func (c *CLI) queryByIndex(args []string) int {
	flags := parseFlags(args)
	nodeType := flags.Get("type")
	prop := flags.Get("property")
	val := flags.Get("value")
	if nodeType == "" || prop == "" {
		c.errorf("Usage: graphdb-cli query by-index --type <node-type> --property <key> --value <value>\n")
		return 1
	}
	var value interface{}
	if err := json.Unmarshal([]byte(val), &value); err != nil {
		value = val
	}
	nodes := c.store.FindByIndex(nodeType, prop, value)
	c.printJSON(nodes)
	return 0
}

func (c *CLI) queryTraverse(args []string) int {
	flags := parseFlags(args)
	startStr := flags.Get("start")
	edgeType := flags.Get("edge-type")
	dir := flags.Get("direction")
	depthStr := flags.Get("depth")
	if startStr == "" {
		c.errorf("Usage: graphdb-cli query traverse --start <id> [--edge-type <type>] [--direction out|in|both] [--depth <n>]\n")
		return 1
	}
	startID, err := uuid.Parse(startStr)
	if err != nil {
		c.errorf("Invalid start ID: %v\n", err)
		return 1
	}
	if dir == "" {
		dir = "out"
	}
	depth := 10
	if depthStr != "" {
		depth, _ = strconv.Atoi(depthStr)
	}
	nodes, err := c.store.Traverse(startID, edgeType, dir, depth)
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(nodes)
	return 0
}

// --- Batch commands ---

func (c *CLI) batchCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli batch execute < batch.json\n")
		return 1
	}
	if args[0] != "execute" {
		c.errorf("Unknown batch command: %s\n", args[0])
		return 1
	}

	// Read batch from stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		c.errorf("Error reading stdin: %v\n", err)
		return 1
	}

	var batchInput []batchInputOp
	if err := json.Unmarshal(data, &batchInput); err != nil {
		c.errorf("Invalid batch JSON: %v\n", err)
		return 1
	}

	ops := make([]graph.BatchOp, len(batchInput))
	for i, in := range batchInput {
		op, err := in.toBatchOp()
		if err != nil {
			c.errorf("Batch op %d: %v\n", i, err)
			return 1
		}
		ops[i] = op
	}

	result, err := c.store.ExecuteBatch(ops)
	if err != nil {
		c.errorf("Batch error: %v\n", err)
		return 1
	}

	// Output result IDs
	var output []map[string]interface{}
	for i, r := range result.Results {
		entry := map[string]interface{}{
			"index": i,
			"type":  batchInput[i].Op,
		}
		if r.ResultID != uuid.Nil {
			entry["resultId"] = r.ResultID.String()
		}
		output = append(output, entry)
	}
	c.printJSON(output)
	return 0
}

type batchInputOp struct {
	Op         string                 `json:"op"`
	NodeType   string                 `json:"nodeType,omitempty"`
	ParentID   string                 `json:"parentId,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	NodeID     string                 `json:"nodeId,omitempty"`
	EdgeID     string                 `json:"edgeId,omitempty"`
	Key        string                 `json:"key,omitempty"`
	Value      interface{}            `json:"value,omitempty"`
	EdgeType   string                 `json:"edgeType,omitempty"`
	FromID     string                 `json:"fromId,omitempty"`
	ToID       string                 `json:"toId,omitempty"`
	Position   string                 `json:"position,omitempty"`
}

func (b *batchInputOp) toBatchOp() (graph.BatchOp, error) {
	op := graph.BatchOp{
		Properties: b.Properties,
		NodeType:   b.NodeType,
		Key:        b.Key,
		Value:      b.Value,
		EdgeType:   b.EdgeType,
		Position:   b.Position,
	}

	if b.NodeID != "" {
		id, err := uuid.Parse(b.NodeID)
		if err != nil {
			return op, fmt.Errorf("invalid nodeId: %v", err)
		}
		op.NodeID = id
	}
	if b.EdgeID != "" {
		id, err := uuid.Parse(b.EdgeID)
		if err != nil {
			return op, fmt.Errorf("invalid edgeId: %v", err)
		}
		op.EdgeID = id
	}
	if b.ParentID != "" {
		id, err := uuid.Parse(b.ParentID)
		if err != nil {
			return op, fmt.Errorf("invalid parentId: %v", err)
		}
		op.ParentID = &id
	}
	if b.FromID != "" {
		id, err := uuid.Parse(b.FromID)
		if err != nil {
			return op, fmt.Errorf("invalid fromId: %v", err)
		}
		op.FromID = id
	}
	if b.ToID != "" {
		id, err := uuid.Parse(b.ToID)
		if err != nil {
			return op, fmt.Errorf("invalid toId: %v", err)
		}
		op.ToID = id
	}

	switch strings.ToLower(b.Op) {
	case "insertnode", "insert-node":
		op.Type = graph.BatchInsertNode
	case "deletenode", "delete-node":
		op.Type = graph.BatchDeleteNode
	case "setproperty", "set-property":
		op.Type = graph.BatchSetProperty
	case "deleteproperty", "delete-property":
		op.Type = graph.BatchDeleteProperty
	case "insertedge", "insert-edge":
		op.Type = graph.BatchInsertEdge
	case "deleteedge", "delete-edge":
		op.Type = graph.BatchDeleteEdge
	case "movenode", "move-node":
		op.Type = graph.BatchMoveNode
	case "reordernode", "reorder-node":
		op.Type = graph.BatchReorderNode
	case "restorenode", "restore-node":
		op.Type = graph.BatchRestoreNode
	case "cascadedelete", "cascade-delete":
		op.Type = graph.BatchCascadeDelete
	default:
		return op, fmt.Errorf("unknown op type: %s", b.Op)
	}

	return op, nil
}

// --- Admin commands ---

func (c *CLI) adminCmd(args []string) int {
	if len(args) < 1 {
		c.errorf("Usage: graphdb-cli admin <reap-orphans|deleted-nodes|stats>\n")
		return 1
	}

	switch args[0] {
	case "reap-orphans":
		return c.adminReapOrphans(args[1:])
	case "deleted-nodes":
		return c.adminDeletedNodes(args[1:])
	case "stats":
		return c.adminStats(args[1:])
	default:
		c.errorf("Unknown admin command: %s\n", args[0])
		return 1
	}
}

func (c *CLI) adminReapOrphans(args []string) int {
	reaped, err := c.store.ReapOrphans()
	if err != nil {
		c.errorf("Error: %v\n", err)
		return 1
	}
	c.printJSON(map[string]interface{}{
		"reapedCount": len(reaped),
		"reapedIds":   uuidsToStrings(reaped),
	})
	return 0
}

func (c *CLI) adminDeletedNodes(args []string) int {
	nodes := c.store.GetDeletedNodes()
	c.printJSON(nodes)
	return 0
}

func (c *CLI) adminStats(args []string) int {
	nodes := c.store.AllNodes()
	edges := c.store.AllEdges()
	deleted := c.store.GetDeletedNodes()
	roots := c.store.GetRoots()

	typeCounts := make(map[string]int)
	for _, n := range nodes {
		typeCounts[n.Type]++
	}

	c.printJSON(map[string]interface{}{
		"totalNodes":    len(nodes),
		"totalEdges":    len(edges),
		"deletedNodes":  len(deleted),
		"rootNodes":     len(roots),
		"nodesByType":   typeCounts,
	})
	return 0
}

// --- Flag parsing ---

type flagMap map[string]string

func (f flagMap) Get(key string) string {
	return f[key]
}

func parseFlags(args []string) flagMap {
	flags := make(flagMap)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				flags[key] = "true"
			}
		}
	}
	return flags
}

func uuidsToStrings(ids []uuid.UUID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}

func main() {
	store := graph.NewStore("cli-" + uuid.New().String()[:8])
	cli := NewCLI(store)
	os.Exit(cli.Run(os.Args[1:]))
}
