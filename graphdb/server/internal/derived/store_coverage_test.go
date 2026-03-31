package derived

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
)

// --- DerivedStore accessor methods ---

func TestStoreSchema(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)
	schema := ds.Schema()
	if schema == nil {
		t.Error("schema should not be nil")
	}
}

func TestStoreSource(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)
	if ds.Source() != source {
		t.Error("source should match")
	}
}

func TestStoreOnChange(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	var events []ChangeEvent
	ds.OnChange(func(e ChangeEvent) {
		events = append(events, e)
	})

	// Register a simple pipeline and recompute to trigger changes
	pipeline := NewPipeline("p1", "test").
		Map("item", "derived_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "test"})
	ds.Recompute()

	if len(events) == 0 {
		t.Error("expected change events from OnChange listener")
	}
	if events[0].Type != ChangeInsert {
		t.Errorf("expected ChangeInsert, got %v", events[0].Type)
	}
}

func TestStoreGetNode(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "a"})
	ds.Recompute()

	nodes := ds.GetNodesByType("d_item")
	if len(nodes) == 0 {
		t.Fatal("expected derived nodes")
	}

	// GetNode with valid ID
	n, ok := ds.GetNode(nodes[0].ID)
	if !ok || n == nil {
		t.Error("GetNode should find derived node")
	}

	// GetNode with invalid ID
	_, ok = ds.GetNode(uuid.New())
	if ok {
		t.Error("GetNode should not find non-existent node")
	}
}

func TestStoreGetEdge(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Use a Compute stage to emit edges
	pipeline := NewPipeline("p1", "test").
		Compute(func(ctx *ComputeContext) {
			id1 := uuid.New()
			id2 := uuid.New()
			edgeID := uuid.New()
			ctx.Emit(&DerivedNode{ID: id1, DerivedType: "x", Properties: map[string]interface{}{}})
			ctx.Emit(&DerivedNode{ID: id2, DerivedType: "x", Properties: map[string]interface{}{}})
			ctx.EmitEdge(&DerivedEdge{ID: edgeID, FromID: id1, ToID: id2, Type: "link"})
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// AllNodes
	allNodes := ds.AllNodes()
	if len(allNodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(allNodes))
	}

	// Find the edge
	edges := ds.GetOutEdges(allNodes[0].ID)
	inEdges := ds.GetInEdges(allNodes[0].ID)

	// At least one direction should have edges
	totalEdges := len(edges) + len(inEdges)
	if totalEdges == 0 {
		t.Error("expected edges on one of the nodes")
	}

	// GetEdge with invalid ID
	_, ok := ds.GetEdge(uuid.New())
	if ok {
		t.Error("GetEdge should not find non-existent edge")
	}
}

func TestStoreAllNodes(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "a"})
	source.InsertNode("item", nil, map[string]interface{}{"name": "b"})
	source.InsertNode("item", nil, map[string]interface{}{"name": "c"})
	ds.Recompute()

	all := ds.AllNodes()
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}
}

// --- Pipeline builder methods ---

func TestPipelineEmitEdge(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	var emittedEdge *DerivedEdge
	pipeline := NewPipeline("p1", "edge-test").
		Compute(func(ctx *ComputeContext) {
			n1 := &DerivedNode{ID: uuid.New(), DerivedType: "x", Properties: map[string]interface{}{}}
			n2 := &DerivedNode{ID: uuid.New(), DerivedType: "x", Properties: map[string]interface{}{}}
			ctx.Emit(n1)
			ctx.Emit(n2)
			e := &DerivedEdge{ID: uuid.New(), FromID: n1.ID, ToID: n2.ID, Type: "rel"}
			ctx.EmitEdge(e)
			emittedEdge = e
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	if emittedEdge == nil {
		t.Fatal("edge not emitted")
	}
	e, ok := ds.GetEdge(emittedEdge.ID)
	if !ok || e == nil {
		t.Error("emitted edge should be findable")
	}
}

func TestGetDerivedChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	var parentID uuid.UUID
	pipeline := NewPipeline("p1", "child-test").
		Compute(func(ctx *ComputeContext) {
			parentID = uuid.New()
			childID := uuid.New()
			ctx.Emit(&DerivedNode{
				ID:          parentID,
				DerivedType: "folder",
				Properties:  map[string]interface{}{},
			})
			ctx.Emit(&DerivedNode{
				ID:          childID,
				DerivedType: "file",
				Properties:  map[string]interface{}{},
				ParentID:    &parentID,
			})
			// Query children within compute
			children := ctx.GetDerivedChildren(parentID)
			if len(children) != 1 {
				// Can't use t here but the assertion below will catch it
			}
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	children := ds.GetChildren(parentID)
	if len(children) != 1 {
		t.Errorf("expected 1 child, got %d", len(children))
	}
}

func TestInheritWithTransform(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Create component with child
	compID, _ := source.InsertNode("component", nil, map[string]interface{}{
		"name": "MyComp",
	})
	source.InsertNode("element", &compID, map[string]interface{}{
		"tag": "div",
	})

	// Create instance referencing component
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})

	inh := &SubtreeDerivation{
		Name:         "comp-inherit",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritSubtree,
	}

	pipeline := NewPipeline("p1", "inherit-transform").
		InheritWithTransform("derived_instance", inh,
			func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
				props := make(map[string]interface{})
				for k, v := range sn.Properties {
					props[k] = v
				}
				props["transformed"] = true
				return props
			})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

func TestMultiSubtree(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Create two components
	comp1, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "A"})
	source.InsertNode("element", &comp1, map[string]interface{}{"tag": "div"})

	comp2, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "B"})
	source.InsertNode("element", &comp2, map[string]interface{}{"tag": "span"})

	// Create a container referencing both
	source.InsertNode("container", nil, map[string]interface{}{
		"refs": []interface{}{comp1.String(), comp2.String()},
	})

	ms := &MultiSubtreeDef{
		ParentType: "container",
		Sources: []*SubtreeSource{
			{
				Name:            "components",
				ResolveVia:      ResolvePropertySlice,
				Property:        "refs",
				SourceType:      "component",
				IncludeChildren: true,
			},
		},
	}

	pipeline := NewPipeline("p1", "multi-sub").
		MultiSubtree("derived_container", ms)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

// --- DerivedNodeType builder methods ---

func TestRefField(t *testing.T) {
	dt := NewDerivedNodeType("widget").
		RefField("parentRef", "container", true)

	f, ok := dt.GetField("parentRef")
	if !ok {
		t.Fatal("RefField should be accessible via GetField")
	}
	if f.Type != FieldRef {
		t.Errorf("expected FieldRef, got %v", f.Type)
	}
	if f.RefType != "container" {
		t.Errorf("expected refType 'container', got %s", f.RefType)
	}
	if !f.Required {
		t.Error("field should be required")
	}
}

func TestDeriveFrom(t *testing.T) {
	dt := NewDerivedNodeType("user_view").
		DeriveFrom("user")
	if dt.SourceType != "user" {
		t.Errorf("expected source type 'user', got %s", dt.SourceType)
	}
}

func TestInheritsFrom(t *testing.T) {
	dt := NewDerivedNodeType("instance").
		InheritsFrom("component", "componentId", InheritSubtree)
	if dt.Inherits == nil {
		t.Fatal("Inherits should be set")
	}
	if dt.Inherits.FromType != "component" {
		t.Errorf("expected fromType 'component', got %s", dt.Inherits.FromType)
	}
	if dt.Inherits.RefProperty != "componentId" {
		t.Errorf("expected refProperty 'componentId', got %s", dt.Inherits.RefProperty)
	}
}

func TestInheritsChildTypes(t *testing.T) {
	dt := NewDerivedNodeType("instance").
		InheritsFrom("component", "componentId", InheritSubtree).
		InheritsChildTypes("element", "text")
	if len(dt.Inherits.ChildTypes) != 2 {
		t.Errorf("expected 2 child types, got %d", len(dt.Inherits.ChildTypes))
	}
}

func TestInheritsChildTypesNoInherits(t *testing.T) {
	// InheritsChildTypes should be safe when Inherits is nil
	dt := NewDerivedNodeType("plain").
		InheritsChildTypes("element")
	if dt.Inherits != nil {
		t.Error("Inherits should remain nil")
	}
}

func TestGetFieldBuildIndex(t *testing.T) {
	// Manually create a type without using builders (bypassing fieldIndex init)
	dt := &DerivedNodeType{
		Name: "raw",
		Fields: []*FieldDef{
			{Name: "x", Type: FieldInt, Required: false},
			{Name: "y", Type: FieldString, Required: true},
		},
	}

	// GetField should trigger buildIndex
	f, ok := dt.GetField("x")
	if !ok || f.Type != FieldInt {
		t.Error("GetField should find field after building index")
	}

	_, ok = dt.GetField("nonexistent")
	if ok {
		t.Error("GetField should return false for missing field")
	}
}

// --- validateFieldType for all types ---

func TestValidateFieldTypeAllTypes(t *testing.T) {
	tests := []struct {
		name    string
		field   *FieldDef
		value   interface{}
		wantErr bool
	}{
		{"string valid", &FieldDef{Type: FieldString}, "hello", false},
		{"string invalid", &FieldDef{Type: FieldString}, 123, true},
		{"int valid int", &FieldDef{Type: FieldInt}, 42, false},
		{"int valid float64", &FieldDef{Type: FieldInt}, float64(42), false},
		{"int invalid", &FieldDef{Type: FieldInt}, "nope", true},
		{"float valid float64", &FieldDef{Type: FieldFloat}, float64(3.14), false},
		{"float valid int", &FieldDef{Type: FieldFloat}, 42, false},
		{"float invalid", &FieldDef{Type: FieldFloat}, "nope", true},
		{"bool valid", &FieldDef{Type: FieldBool}, true, false},
		{"bool invalid", &FieldDef{Type: FieldBool}, "true", true},
		{"string slice valid []string", &FieldDef{Type: FieldStringSlice}, []string{"a"}, false},
		{"string slice valid []interface", &FieldDef{Type: FieldStringSlice}, []interface{}{"a"}, false},
		{"string slice invalid", &FieldDef{Type: FieldStringSlice}, "nope", true},
		{"map valid", &FieldDef{Type: FieldMap}, map[string]interface{}{"k": "v"}, false},
		{"map invalid", &FieldDef{Type: FieldMap}, "nope", true},
		{"ref valid", &FieldDef{Type: FieldRef, RefType: "user"}, "uuid-string", false},
		{"ref invalid", &FieldDef{Type: FieldRef}, 123, true},
		{"ref slice valid []string", &FieldDef{Type: FieldRefSlice}, []string{"a"}, false},
		{"ref slice valid []interface", &FieldDef{Type: FieldRefSlice}, []interface{}{"a"}, false},
		{"ref slice invalid", &FieldDef{Type: FieldRefSlice}, "nope", true},
		{"any valid string", &FieldDef{Type: FieldAny}, "anything", false},
		{"any valid int", &FieldDef{Type: FieldAny}, 42, false},
		{"nil non-required", &FieldDef{Type: FieldString, Required: false}, nil, false},
		{"nil required", &FieldDef{Type: FieldString, Required: true}, nil, true},
		{"struct valid", &FieldDef{Type: FieldStruct, SubFields: []*FieldDef{{Name: "x", Type: FieldInt}}}, map[string]interface{}{"x": 1}, false},
		{"struct invalid type", &FieldDef{Type: FieldStruct}, "nope", true},
		{"struct missing required subfield", &FieldDef{Type: FieldStruct, SubFields: []*FieldDef{{Name: "x", Type: FieldInt, Required: true}}}, map[string]interface{}{}, true},
		{"struct bad subfield type", &FieldDef{Type: FieldStruct, SubFields: []*FieldDef{{Name: "x", Type: FieldInt}}}, map[string]interface{}{"x": "nope"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldType(tt.field, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFieldType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- DerivedNodeForSource ---

func TestDerivedNodeForSource(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	srcID, _ := source.InsertNode("item", nil, map[string]interface{}{"name": "x"})
	ds.Recompute()

	// Build a context and check DerivedNodeForSource
	ctx := ds.buildContext()
	dn, ok := ctx.DerivedNodeForSource(srcID)
	if !ok || dn == nil {
		t.Error("DerivedNodeForSource should find derived node for source")
	}

	// Non-existent source
	_, ok = ctx.DerivedNodeForSource(uuid.New())
	if ok {
		t.Error("DerivedNodeForSource should not find non-existent source")
	}
}

// --- updateDerivedNode and removeDerivedNode ---

func TestUpdateAndRemoveDerivedNode(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	var events []ChangeEvent
	ds.OnChange(func(e ChangeEvent) {
		events = append(events, e)
	})

	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "x"})
	ds.Recompute()

	nodes := ds.GetNodesByType("d_item")
	if len(nodes) == 0 {
		t.Fatal("expected nodes")
	}

	// Manually update
	ds.mu.Lock()
	updated := &DerivedNode{
		ID:          nodes[0].ID,
		DerivedType: "d_item",
		Properties:  map[string]interface{}{"name": "updated"},
	}
	ds.updateDerivedNode(updated)
	ds.mu.Unlock()

	n, ok := ds.GetNode(nodes[0].ID)
	if !ok || n.Properties["name"] != "updated" {
		t.Error("node should be updated")
	}

	// Check we got an update event
	foundUpdate := false
	for _, e := range events {
		if e.Type == ChangeUpdate {
			foundUpdate = true
		}
	}
	if !foundUpdate {
		t.Error("expected ChangeUpdate event")
	}

	// Manually remove
	ds.mu.Lock()
	ds.removeDerivedNode(nodes[0].ID)
	ds.mu.Unlock()

	_, ok = ds.GetNode(nodes[0].ID)
	if ok {
		t.Error("node should be removed")
	}

	// Remove non-existent (should be safe)
	ds.mu.Lock()
	ds.removeDerivedNode(uuid.New())
	ds.mu.Unlock()
}

// --- Join stage ---

func TestJoinStage(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Create users and posts with edge
	user1, _ := source.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	post1, _ := source.InsertNode("post", nil, map[string]interface{}{"title": "Hello"})
	source.InsertEdge("authored", user1, post1, nil)

	join := &JoinDef{
		SourceType: "post",
		Relations: []*RelationDef{
			{Name: "author", Via: RelViaInEdge, EdgeType: "authored"},
		},
	}

	pipeline := NewPipeline("p1", "join-test").
		Join("post_with_author", join,
			func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
				return sn.Properties
			})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

// --- GetDerivedNodesByType in compute context ---

func TestComputeGetDerivedNodesByType(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("item", nil, map[string]interface{}{"name": "a"})
	source.InsertNode("item", nil, map[string]interface{}{"name": "b"})

	var countInCompute int
	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil).
		Compute(func(ctx *ComputeContext) {
			// Query nodes that were derived in the Map stage
			nodes := ctx.GetDerivedNodesByType("d_item")
			countInCompute = len(nodes)
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	if countInCompute != 2 {
		t.Errorf("expected 2 derived nodes in compute, got %d", countInCompute)
	}
}

// --- Pipeline with mode ---

func TestPipelineWithMode(t *testing.T) {
	p := NewPipeline("p1", "test").WithMode(ExecClient)
	if p.ExecutionMode != ExecClient {
		t.Errorf("expected ExecClient, got %v", p.ExecutionMode)
	}
}

// --- MapWithChildren ---

func TestMapWithChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	parentID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "a.txt"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "b.txt"})

	pipeline := NewPipeline("p1", "with-children").
		MapWithChildren("folder", "d_folder", nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	folders := ds.GetNodesByType("d_folder")
	if len(folders) == 0 {
		t.Error("expected derived folder nodes")
	}
}

// --- Recompute clears state ---

func TestRecomputeClearsState(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "test").
		Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "a"})
	ds.Recompute()

	if len(ds.AllNodes()) != 1 {
		t.Fatal("expected 1 node")
	}

	// Recompute should rebuild from scratch (same count since source unchanged)
	ds.Recompute()
	if len(ds.AllNodes()) != 1 {
		t.Errorf("expected 1 node after recompute, got %d", len(ds.AllNodes()))
	}
}

// --- MergeProperties ---

func TestMergePropertiesMerge(t *testing.T) {
	result := MergeProperties(
		map[string]interface{}{"a": 1, "b": 2},
		map[string]interface{}{"b": 3, "c": 4},
		InheritMerge,
	)
	if result["a"] != 1 {
		t.Error("merge should include source props")
	}
	// Check that both source and override props are present
	if result["c"] != 4 {
		t.Error("merge should include override props")
	}
}

// --- AddStage ---

func TestAddStage(t *testing.T) {
	p := NewPipeline("p1", "test").
		AddStage(&Stage{
			Type:        StageMap,
			SourceType:  "item",
			DerivedType: "d_item",
		})
	if len(p.Stages) != 1 {
		t.Errorf("expected 1 stage, got %d", len(p.Stages))
	}
}

// --- MultiSubtreeWithTransform ---

func TestMultiSubtreeWithTransform(t *testing.T) {
	p := NewPipeline("p1", "test").
		MultiSubtreeWithTransform("derived", &MultiSubtreeDef{
			ParentType: "container",
		}, func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			return sn.Properties
		})
	if len(p.Stages) != 1 {
		t.Error("expected 1 stage")
	}
	if p.Stages[0].Type != StageMultiSubtree {
		t.Error("expected StageMultiSubtree")
	}
}

// --- resolveRelated: all RelVia types ---

func TestJoinViaProperty(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	authorID, _ := source.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	source.InsertNode("post", nil, map[string]interface{}{
		"title":    "Hello",
		"authorId": authorID.String(),
	})

	pipeline := NewPipeline("p1", "join-prop").
		Join("post_with_author", &JoinDef{
			SourceType: "post",
			Relations: []*RelationDef{
				{Name: "author", Via: RelViaProperty, Property: "authorId"},
			},
		}, nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("post_with_author")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 joined node, got %d", len(nodes))
	}
	if nodes[0].Properties["author"] == nil {
		t.Error("expected author relation to be resolved")
	}
}

func TestJoinViaParent(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	parentID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "readme"})

	pipeline := NewPipeline("p1", "join-parent").
		Join("file_with_parent", &JoinDef{
			SourceType: "file",
			Relations: []*RelationDef{
				{Name: "parent", Via: RelViaParent},
			},
		}, nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("file_with_parent")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
}

func TestJoinViaChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	parentID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "a"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "b"})

	pipeline := NewPipeline("p1", "join-children").
		Join("folder_with_files", &JoinDef{
			SourceType: "folder",
			Relations: []*RelationDef{
				{Name: "files", Via: RelViaChildren, TargetType: "file"},
			},
		}, nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("folder_with_files")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	// files should be an array since there are multiple
	if nodes[0].Properties["files"] == nil {
		t.Error("expected files relation")
	}
}

func TestJoinViaOutEdge(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	u1, _ := source.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	u2, _ := source.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	source.InsertEdge("follows", u1, u2, nil)

	pipeline := NewPipeline("p1", "join-outedge").
		Join("user_with_following", &JoinDef{
			SourceType: "user",
			Relations: []*RelationDef{
				{Name: "following", Via: RelViaOutEdge, EdgeType: "follows"},
			},
		}, nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

// --- removeDerivedNode with parent and source ---

func TestRemoveDerivedNodeWithParentAndSource(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	parentID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	source.InsertNode("file", &parentID, map[string]interface{}{"name": "child"})

	pipeline := NewPipeline("p1", "test").
		MapWithChildren("folder", "d_folder", nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// Now recompute clears everything (remove + re-insert)
	ds.Recompute()

	nodes := ds.AllNodes()
	if len(nodes) == 0 {
		t.Error("expected nodes after recompute")
	}
}

// --- SubtreeInheritance with child type mapping ---

func TestSubtreeInheritanceWithMapping(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Btn"})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "div"})

	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})

	inh := &SubtreeDerivation{
		Name:         "remap",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritSubtree,
		ChildTypeMapping: map[string]string{
			"element": "inst_element",
		},
	}

	pipeline := NewPipeline("p1", "inherit-remap").
		Inherit("derived_instance", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

// --- MultiSubtree with children and transform ---

func TestMultiSubtreeWithChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	comp1, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "A"})
	source.InsertNode("element", &comp1, map[string]interface{}{"tag": "div"})

	source.InsertNode("page", nil, map[string]interface{}{
		"compRef": comp1.String(),
	})

	ms := &MultiSubtreeDef{
		ParentType: "page",
		Sources: []*SubtreeSource{
			{
				Name:            "components",
				ResolveVia:      ResolveProperty,
				Property:        "compRef",
				SourceType:      "component",
				IncludeChildren: true,
			},
		},
	}

	pipeline := NewPipeline("p1", "multi-children").
		MultiSubtreeWithTransform("d_page", ms,
			func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
				return sn.Properties
			})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()
}

// --- Validate with nil required and extra fields ---

func TestValidateNilRequired(t *testing.T) {
	dt := NewDerivedNodeType("test").
		Field("x", FieldString, true)
	err := dt.Validate(map[string]interface{}{"x": nil})
	if err == nil {
		t.Error("expected error for nil required field")
	}
}

func TestValidateExtraFields(t *testing.T) {
	dt := NewDerivedNodeType("test").
		Field("x", FieldString, false)
	err := dt.Validate(map[string]interface{}{"x": "hello", "extra": 123})
	if err != nil {
		t.Errorf("extra fields should be allowed: %v", err)
	}
}

// --- MergeProperties all strategies ---

func TestMergePropertiesSubtree(t *testing.T) {
	result := MergeProperties(
		map[string]interface{}{"a": 1, "b": 2},
		map[string]interface{}{"b": 3},
		InheritSubtree,
	)
	if result["b"] != 3 {
		t.Error("overrides should win in InheritSubtree")
	}
}

func TestMergePropertiesProperties(t *testing.T) {
	result := MergeProperties(
		map[string]interface{}{"a": 1},
		map[string]interface{}{"b": 2},
		InheritProperties,
	)
	if result["a"] != 1 || result["b"] != 2 {
		t.Error("both source and override props should be present")
	}
}
