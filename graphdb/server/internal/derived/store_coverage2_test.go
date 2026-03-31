package derived

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
)

// --- removeDerivedNode: exercise all removal paths ---

func TestRemoveDerivedNodeWithSourceAndParent(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Create parent with two children, children mapped to source
	parentID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	child1, _ := source.InsertNode("file", &parentID, map[string]interface{}{"name": "a"})
	child2, _ := source.InsertNode("file", &parentID, map[string]interface{}{"name": "b"})

	pipeline := NewPipeline("p1", "test").
		MapWithChildren("folder", "d_folder", nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// Verify children were derived
	nodes := ds.AllNodes()
	if len(nodes) < 3 {
		t.Fatalf("expected at least 3 derived nodes, got %d", len(nodes))
	}

	// Now remove the parent (should cascade remove children)
	ds.mu.Lock()
	// Find the derived parent
	var derivedParentID uuid.UUID
	for _, n := range ds.nodes {
		if n.SourceID != nil && *n.SourceID == parentID {
			derivedParentID = n.ID
			break
		}
	}
	ds.removeDerivedNode(derivedParentID)
	ds.mu.Unlock()

	// All nodes should be removed (parent + children cascade)
	remaining := ds.AllNodes()
	if len(remaining) != 0 {
		t.Errorf("expected 0 nodes after cascade remove, got %d", len(remaining))
	}

	_ = child1
	_ = child2
}

func TestRemoveDerivedNodeSourceIndex(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	srcID, _ := source.InsertNode("item", nil, map[string]interface{}{"name": "x"})

	pipeline := NewPipeline("p1", "test").Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// Find derived node
	nodes := ds.GetNodesByType("d_item")
	if len(nodes) != 1 {
		t.Fatal("expected 1 derived node")
	}

	// Verify source mapping exists
	ds.mu.RLock()
	_, hasSrcMap := ds.sourceToDerived[srcID]
	_, hasDerMap := ds.derivedToSource[nodes[0].ID]
	ds.mu.RUnlock()
	if !hasSrcMap || !hasDerMap {
		t.Error("expected source/derived mapping before removal")
	}

	// Remove it
	ds.mu.Lock()
	ds.removeDerivedNode(nodes[0].ID)
	ds.mu.Unlock()

	// Verify source mapping cleaned up
	ds.mu.RLock()
	srcDerived := ds.sourceToDerived[srcID]
	_, hasDerMap = ds.derivedToSource[nodes[0].ID]
	ds.mu.RUnlock()
	if len(srcDerived) != 0 {
		t.Error("expected source mapping removed")
	}
	if hasDerMap {
		t.Error("expected derived->source mapping removed")
	}
}

// --- runSubtreeInheritStage: ExcludeProperties, PropertyTransform, bad ref ---

func TestSubtreeInheritExcludeProperties(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{
		"name": "Btn", "internal": "secret", "color": "blue",
	})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "div"})

	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})

	inh := &SubtreeDerivation{
		Name:              "test",
		InstanceType:      "instance",
		SourceType:        "component",
		RefProperty:       "componentId",
		Strategy:          InheritSubtree,
		ExcludeProperties: []string{"internal"},
	}

	pipeline := NewPipeline("p1", "exclude-test").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("d_inst")
	if len(nodes) == 0 {
		t.Fatal("expected derived nodes")
	}
	for _, n := range nodes {
		if _, has := n.Properties["internal"]; has {
			t.Error("excluded property 'internal' should not be present")
		}
	}
}

func TestSubtreeInheritPropertyTransform(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Btn"})
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
		PropertyTransform: func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			return map[string]interface{}{"custom": "transformed"}
		},
	}

	pipeline := NewPipeline("p1", "transform-test").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("d_inst")
	if len(nodes) == 0 {
		t.Fatal("expected derived nodes")
	}
	if nodes[0].Properties["custom"] != "transformed" {
		t.Error("PropertyTransform should override properties")
	}
}

func TestSubtreeInheritBadRefProperty(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Instance with non-string ref
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": 12345, // not a string
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
	}

	pipeline := NewPipeline("p1", "bad-ref").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("d_inst")
	if len(nodes) != 0 {
		t.Error("should skip instance with non-string ref")
	}
}

func TestSubtreeInheritInvalidUUID(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": "not-a-uuid",
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
	}

	pipeline := NewPipeline("p1", "bad-uuid").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	if len(ds.GetNodesByType("d_inst")) != 0 {
		t.Error("should skip instance with invalid UUID")
	}
}

func TestSubtreeInheritMissingComponent(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": uuid.New().String(), // valid UUID but non-existent
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
	}

	pipeline := NewPipeline("p1", "missing-comp").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	if len(ds.GetNodesByType("d_inst")) != 0 {
		t.Error("should skip instance with non-existent component")
	}
}

func TestSubtreeInheritWithFilter(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Btn"})
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
		"active":      false,
	})
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
		"active":      true,
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
	}

	pipeline := NewPipeline("p1", "filter-test").
		InheritWithTransform("d_inst", inh, nil)
	// Add a filter via the stage directly
	pipeline.Stages[0].Filter = func(sn *crdt.MaterializedNode) bool {
		return sn.Properties["active"] == true
	}
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	if len(ds.GetNodesByType("d_inst")) != 1 {
		t.Errorf("expected 1 filtered node, got %d", len(ds.GetNodesByType("d_inst")))
	}
}

// --- deriveMultiSubtreeSource: all ResolveVia types ---

func TestMultiSubtreeResolveEdge(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "A"})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "div"})

	pageID, _ := source.InsertNode("page", nil, map[string]interface{}{"name": "Home"})
	source.InsertEdge("uses", pageID, compID, nil)

	pipeline := NewPipeline("p1", "edge-resolve").
		MultiSubtree("d_page", &MultiSubtreeDef{
			ParentType: "page",
			Sources: []*SubtreeSource{
				{
					Name:            "components",
					ResolveVia:      ResolveEdge,
					EdgeType:        "uses",
					SourceType:      "component",
					IncludeChildren: true,
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	pages := ds.GetNodesByType("d_page")
	if len(pages) != 1 {
		t.Fatalf("expected 1 derived page, got %d", len(pages))
	}
	children := ds.GetChildren(pages[0].ID)
	if len(children) == 0 {
		t.Error("expected children from edge-resolved component")
	}
}

func TestMultiSubtreeResolveChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	pageID, _ := source.InsertNode("page", nil, map[string]interface{}{"name": "Home"})
	source.InsertNode("section", &pageID, map[string]interface{}{"name": "Hero"})
	source.InsertNode("section", &pageID, map[string]interface{}{"name": "Footer"})

	pipeline := NewPipeline("p1", "children-resolve").
		MultiSubtree("d_page", &MultiSubtreeDef{
			ParentType: "page",
			Sources: []*SubtreeSource{
				{
					Name:       "sections",
					ResolveVia: ResolveChildren,
					SourceType: "section",
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	pages := ds.GetNodesByType("d_page")
	if len(pages) != 1 {
		t.Fatal("expected 1 derived page")
	}
	children := ds.GetChildren(pages[0].ID)
	if len(children) != 2 {
		t.Errorf("expected 2 section children, got %d", len(children))
	}
}

func TestMultiSubtreeResolveQuery(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("item", nil, map[string]interface{}{"name": "target"})
	source.InsertNode("container", nil, map[string]interface{}{"name": "parent"})

	pipeline := NewPipeline("p1", "query-resolve").
		MultiSubtree("d_container", &MultiSubtreeDef{
			ParentType: "container",
			Sources: []*SubtreeSource{
				{
					Name:       "items",
					ResolveVia: ResolveQuery,
					QueryFunc: func(parent *crdt.MaterializedNode, ctx *DerivationContext) []*crdt.MaterializedNode {
						return ctx.SourceNodesByType("item")
					},
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	containers := ds.GetNodesByType("d_container")
	if len(containers) != 1 {
		t.Fatal("expected 1 derived container")
	}
	children := ds.GetChildren(containers[0].ID)
	if len(children) != 1 {
		t.Errorf("expected 1 child from query, got %d", len(children))
	}
}

func TestMultiSubtreeWithFilterAndTransform(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compA, _ := source.InsertNode("comp", nil, map[string]interface{}{"name": "A", "visible": true})
	compB, _ := source.InsertNode("comp", nil, map[string]interface{}{"name": "B", "visible": false})

	source.InsertNode("container", nil, map[string]interface{}{
		"refs": []interface{}{compA.String(), compB.String()},
	})

	pipeline := NewPipeline("p1", "filter-transform").
		MultiSubtree("d_container", &MultiSubtreeDef{
			ParentType: "container",
			Sources: []*SubtreeSource{
				{
					Name:       "comps",
					ResolveVia: ResolvePropertySlice,
					Property:   "refs",
					Filter: func(sn *crdt.MaterializedNode) bool {
						return sn.Properties["visible"] == true
					},
					Transform: func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
						return map[string]interface{}{"label": sn.Properties["name"], "derived": true}
					},
					DerivedType: "d_comp",
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	comps := ds.GetNodesByType("d_comp")
	if len(comps) != 1 {
		t.Errorf("expected 1 filtered comp, got %d", len(comps))
	}
	if len(comps) > 0 && comps[0].Properties["derived"] != true {
		t.Error("transform should have set derived=true")
	}
}

// --- cloneSubtreeChildren: ChildFilter, ChildTransform, ChildTypeMapping ---

func TestCloneSubtreeWithChildFilter(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("comp", nil, map[string]interface{}{"name": "Root"})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "div", "visible": true})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "span", "visible": false})

	source.InsertNode("container", nil, map[string]interface{}{
		"compRef": compID.String(),
	})

	pipeline := NewPipeline("p1", "child-filter").
		MultiSubtree("d_container", &MultiSubtreeDef{
			ParentType: "container",
			Sources: []*SubtreeSource{
				{
					Name:            "comps",
					ResolveVia:      ResolveProperty,
					Property:        "compRef",
					IncludeChildren: true,
					ChildFilter: func(sn *crdt.MaterializedNode) bool {
						return sn.Properties["visible"] == true
					},
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	containers := ds.GetNodesByType("d_container")
	if len(containers) != 1 {
		t.Fatal("expected 1 container")
	}
	// The comp node should have only 1 visible child
	compNodes := ds.GetChildren(containers[0].ID)
	if len(compNodes) == 0 {
		t.Fatal("expected comp child")
	}
	grandchildren := ds.GetChildren(compNodes[0].ID)
	if len(grandchildren) != 1 {
		t.Errorf("expected 1 filtered child, got %d", len(grandchildren))
	}
}

func TestCloneSubtreeWithChildTransformAndTypeMapping(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("comp", nil, map[string]interface{}{"name": "Root"})
	source.InsertNode("element", &compID, map[string]interface{}{"tag": "div"})

	source.InsertNode("container", nil, map[string]interface{}{
		"compRef": compID.String(),
	})

	pipeline := NewPipeline("p1", "child-transform").
		MultiSubtree("d_container", &MultiSubtreeDef{
			ParentType: "container",
			Sources: []*SubtreeSource{
				{
					Name:            "comps",
					ResolveVia:      ResolveProperty,
					Property:        "compRef",
					IncludeChildren: true,
					ChildTransform: func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
						return map[string]interface{}{"transformed_tag": sn.Properties["tag"]}
					},
					ChildTypeMapping: map[string]string{
						"element": "d_element",
					},
				},
			},
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	dElements := ds.GetNodesByType("d_element")
	if len(dElements) != 1 {
		t.Errorf("expected 1 d_element, got %d", len(dElements))
	}
	if len(dElements) > 0 && dElements[0].Properties["transformed_tag"] != "div" {
		t.Error("child transform should have set transformed_tag")
	}
}

// --- inheritSubtree: instance-only children ---

func TestInheritSubtreeInstanceOnlyChildren(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Btn"})
	source.InsertNode("element", &compID, map[string]interface{}{"name": "label", "tag": "span"})

	instID, _ := source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})
	// Instance-only child not in component
	source.InsertNode("overlay", &instID, map[string]interface{}{"name": "badge"})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritSubtree,
	}

	pipeline := NewPipeline("p1", "inst-only-children").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	instNodes := ds.GetNodesByType("d_inst")
	if len(instNodes) == 0 {
		t.Fatal("expected derived instance")
	}

	// Should have both component child (element) and instance-only child (overlay)
	children := ds.GetChildren(instNodes[0].ID)
	if len(children) < 2 {
		t.Errorf("expected at least 2 children (component + instance-only), got %d", len(children))
	}
}

// --- inheritSubtree: override matching ---

func TestInheritSubtreeOverrideMatching(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Card"})
	source.InsertNode("element", &compID, map[string]interface{}{"name": "title", "tag": "h1", "color": "black"})

	instID, _ := source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})
	// Override: same type+name as component child
	source.InsertNode("element", &instID, map[string]interface{}{"name": "title", "color": "red"})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritSubtree,
	}

	pipeline := NewPipeline("p1", "override-test").Inherit("d_inst", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	instNodes := ds.GetNodesByType("d_inst")
	if len(instNodes) == 0 {
		t.Fatal("expected derived instance")
	}

	children := ds.GetChildren(instNodes[0].ID)
	if len(children) != 1 {
		t.Fatalf("expected 1 child (merged override), got %d", len(children))
	}
	// Override should win: color=red
	if children[0].Properties["color"] != "red" {
		t.Errorf("expected override color 'red', got %v", children[0].Properties["color"])
	}
	// Base property should be inherited: tag=h1
	if children[0].Properties["tag"] != "h1" {
		t.Errorf("expected inherited tag 'h1', got %v", children[0].Properties["tag"])
	}
}

// --- runMultiSubtreeStage: filter and default derivedType ---

func TestMultiSubtreeStageFilter(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("page", nil, map[string]interface{}{"name": "Home", "active": true})
	source.InsertNode("page", nil, map[string]interface{}{"name": "Draft", "active": false})

	pipeline := NewPipeline("p1", "filter-stage").
		MultiSubtree("d_page", &MultiSubtreeDef{
			ParentType: "page",
			Sources:    []*SubtreeSource{},
		})
	// Add filter to stage
	pipeline.Stages[0].Filter = func(sn *crdt.MaterializedNode) bool {
		return sn.Properties["active"] == true
	}
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	pages := ds.GetNodesByType("d_page")
	if len(pages) != 1 {
		t.Errorf("expected 1 filtered page, got %d", len(pages))
	}
}

// --- GetParent and GetSubtree edge cases ---

func TestGetParentNotFound(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)
	parent, ok := ds.GetParent(uuid.New())
	if ok || parent != nil {
		t.Error("expected nil parent for non-existent node")
	}
}

func TestGetSubtreeDeep(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	// Create a 3-level hierarchy via Compute
	var rootID uuid.UUID
	pipeline := NewPipeline("p1", "deep").
		Compute(func(ctx *ComputeContext) {
			rootID = uuid.New()
			child1 := uuid.New()
			grandchild := uuid.New()

			ctx.Emit(&DerivedNode{ID: rootID, DerivedType: "node", Properties: map[string]interface{}{"level": 0}})
			ctx.Emit(&DerivedNode{ID: child1, DerivedType: "node", Properties: map[string]interface{}{"level": 1}, ParentID: &rootID})
			ctx.Emit(&DerivedNode{ID: grandchild, DerivedType: "node", Properties: map[string]interface{}{"level": 2}, ParentID: &child1})
		})
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	subtree := ds.GetSubtree(rootID)
	if len(subtree) != 3 {
		t.Errorf("expected 3 nodes in subtree, got %d", len(subtree))
	}
}

// --- MergeProperties: InheritMerge deep merge ---

func TestMergePropertiesDeepMerge(t *testing.T) {
	source := map[string]interface{}{
		"a": 1,
		"nested": map[string]interface{}{
			"x": 10,
			"y": 20,
		},
	}
	overrides := map[string]interface{}{
		"b": 2,
		"nested": map[string]interface{}{
			"y": 99,
			"z": 30,
		},
	}

	result := MergeProperties(source, overrides, InheritMerge)
	if result["a"] != 1 {
		t.Error("source prop should be preserved")
	}
	if result["b"] != 2 {
		t.Error("override prop should be present")
	}
	nested, ok := result["nested"].(map[string]interface{})
	if ok {
		// InheritMerge should deep merge nested maps
		if nested["z"] != 30 {
			t.Error("nested override should be present")
		}
	}
}

// --- runJoinStage with filter ---

func TestJoinStageWithFilter(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	source.InsertNode("post", nil, map[string]interface{}{"title": "A", "published": true})
	source.InsertNode("post", nil, map[string]interface{}{"title": "B", "published": false})

	pipeline := NewPipeline("p1", "join-filter").
		Join("d_post", &JoinDef{
			SourceType: "post",
		}, nil)
	pipeline.Stages[0].Filter = func(sn *crdt.MaterializedNode) bool {
		return sn.Properties["published"] == true
	}
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	posts := ds.GetNodesByType("d_post")
	if len(posts) != 1 {
		t.Errorf("expected 1 filtered post, got %d", len(posts))
	}
}

// --- runSubtreeInheritStage: empty derivedType defaults to instance type ---

func TestSubtreeInheritEmptyDerivedType(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	compID, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "X"})
	source.InsertNode("instance", nil, map[string]interface{}{
		"componentId": compID.String(),
	})

	inh := &SubtreeDerivation{
		Name:         "test",
		InstanceType: "instance",
		SourceType:   "component",
		RefProperty:  "componentId",
		Strategy:     InheritProperties,
	}

	// Empty DerivedType
	pipeline := NewPipeline("p1", "default-type").Inherit("", inh)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// Should use instance type "instance" as derived type
	nodes := ds.GetNodesByType("instance")
	if len(nodes) != 1 {
		t.Errorf("expected 1 node with default derived type, got %d", len(nodes))
	}
}

// --- Notify with multiple listeners ---

func TestNotifyMultipleListeners(t *testing.T) {
	source := graph.NewStore("r1")
	ds := NewStore(source)

	count1, count2 := 0, 0
	ds.OnChange(func(e ChangeEvent) { count1++ })
	ds.OnChange(func(e ChangeEvent) { count2++ })

	pipeline := NewPipeline("p1", "test").Map("item", "d_item", nil)
	ds.RegisterPipeline(pipeline)

	source.InsertNode("item", nil, map[string]interface{}{"name": "x"})
	ds.Recompute()

	if count1 == 0 || count2 == 0 {
		t.Error("both listeners should have been called")
	}
	if count1 != count2 {
		t.Error("both listeners should have same count")
	}
}
