package derived

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
)

func setupStore() *graph.Store {
	return graph.NewStore("test-replica")
}

// --- Type system tests ---

func TestDerivedNodeTypeValidation(t *testing.T) {
	nodeType := NewDerivedNodeType("user").
		Field("name", FieldString, true).
		Field("age", FieldInt, false).
		FieldWithDefault("role", FieldString, "viewer")

	// Valid: all required fields present
	err := nodeType.Validate(map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})
	if err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	// Invalid: missing required field
	err = nodeType.Validate(map[string]interface{}{
		"age": 30,
	})
	if err == nil {
		t.Error("expected validation error for missing required field")
	}

	// Invalid: wrong type
	err = nodeType.Validate(map[string]interface{}{
		"name": 123, // should be string
	})
	if err == nil {
		t.Error("expected validation error for wrong type")
	}
}

func TestDerivedNodeTypeDefaults(t *testing.T) {
	nodeType := NewDerivedNodeType("user").
		Field("name", FieldString, true).
		FieldWithDefault("role", FieldString, "viewer").
		FieldWithDefault("active", FieldBool, true)

	props := nodeType.ApplyDefaults(map[string]interface{}{
		"name": "Alice",
	})

	if props["role"] != "viewer" {
		t.Errorf("expected default role 'viewer', got %v", props["role"])
	}
	if props["active"] != true {
		t.Errorf("expected default active true, got %v", props["active"])
	}
	if props["name"] != "Alice" {
		t.Errorf("expected name 'Alice', got %v", props["name"])
	}
}

func TestNestedStructField(t *testing.T) {
	nodeType := NewDerivedNodeType("widget").
		StructField("position", true,
			F("x", FieldFloat, true),
			F("y", FieldFloat, true),
			F("z", FieldFloat, false),
		)

	// Valid
	err := nodeType.Validate(map[string]interface{}{
		"position": map[string]interface{}{"x": 1.0, "y": 2.0},
	})
	if err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	// Invalid: missing required sub-field
	err = nodeType.Validate(map[string]interface{}{
		"position": map[string]interface{}{"x": 1.0},
	})
	if err == nil {
		t.Error("expected validation error for missing required sub-field")
	}
}

// --- Basic derivation tests ---

func TestMapDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	ds.RegisterType(NewDerivedNodeType("user_view").
		Field("displayName", FieldString, true).
		Field("isAdmin", FieldBool, false))

	pipeline := NewPipeline("p1", "user-views").
		Map("user", "user_view", func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			return map[string]interface{}{
				"displayName": sn.Properties["name"],
				"isAdmin":     sn.Properties["role"] == "admin",
			}
		})

	ds.RegisterPipeline(pipeline)

	source.InsertNode("user", nil, map[string]interface{}{"name": "Alice", "role": "admin"})
	source.InsertNode("user", nil, map[string]interface{}{"name": "Bob", "role": "viewer"})

	ds.Recompute()

	views := ds.GetNodesByType("user_view")
	if len(views) != 2 {
		t.Fatalf("expected 2 user_view nodes, got %d", len(views))
	}

	// Check properties were transformed
	for _, v := range views {
		if v.Properties["displayName"] == "Alice" {
			if v.Properties["isAdmin"] != true {
				t.Error("Alice should be admin")
			}
		}
		if v.Properties["displayName"] == "Bob" {
			if v.Properties["isAdmin"] != false {
				t.Error("Bob should not be admin")
			}
		}
	}
}

func TestFilteredDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "active-users").
		MapFiltered("user", "active_user",
			func(sn *crdt.MaterializedNode) bool {
				return sn.Properties["active"] == true
			},
			nil, // no transform = copy properties
		)

	ds.RegisterPipeline(pipeline)

	source.InsertNode("user", nil, map[string]interface{}{"name": "Alice", "active": true})
	source.InsertNode("user", nil, map[string]interface{}{"name": "Bob", "active": false})
	source.InsertNode("user", nil, map[string]interface{}{"name": "Charlie", "active": true})

	ds.Recompute()

	nodes := ds.GetNodesByType("active_user")
	if len(nodes) != 2 {
		t.Errorf("expected 2 active users, got %d", len(nodes))
	}
}

// --- Component-instance inheritance tests ---

func TestSubtreeInheritance(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	// Create a component with children
	compID, _ := source.InsertNode("component", nil, map[string]interface{}{
		"name":  "Button",
		"color": "blue",
	})
	source.InsertNode("prop_def", &compID, map[string]interface{}{
		"name":    "label",
		"type":    "string",
		"default": "Click me",
	})
	source.InsertNode("prop_def", &compID, map[string]interface{}{
		"name":    "size",
		"type":    "number",
		"default": 14,
	})

	// Create an instance referencing the component, with overrides
	instID, _ := source.InsertNode("instance", nil, map[string]interface{}{
		"name":        "MyButton",
		"componentId": compID.String(),
		"color":       "red", // override
	})
	// Instance-level override of a child
	source.InsertNode("prop_def", &instID, map[string]interface{}{
		"name":    "label",
		"type":    "string",
		"default": "Submit", // override default
	})

	pipeline := NewPipeline("p1", "instances").
		Inherit("derived_instance", &SubtreeDerivation{
			Name:         "button-instances",
			InstanceType: "instance",
			SourceType:   "component",
			RefProperty:  "componentId",
			Strategy:     InheritSubtree,
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	// Check derived instance
	instances := ds.GetNodesByType("derived_instance")
	if len(instances) != 1 {
		t.Fatalf("expected 1 derived instance, got %d", len(instances))
	}

	inst := instances[0]
	// Override should win
	if inst.Properties["color"] != "red" {
		t.Errorf("expected overridden color 'red', got %v", inst.Properties["color"])
	}
	// Inherited property
	if inst.Properties["name"] != "MyButton" {
		t.Errorf("expected name 'MyButton', got %v", inst.Properties["name"])
	}

	// Check inherited children
	children := ds.GetChildren(inst.ID)
	if len(children) != 2 {
		t.Fatalf("expected 2 inherited children, got %d", len(children))
	}

	// The "label" child should have the instance override
	for _, child := range children {
		if child.Properties["name"] == "label" {
			if child.Properties["default"] != "Submit" {
				t.Errorf("expected overridden default 'Submit', got %v", child.Properties["default"])
			}
		}
		if child.Properties["name"] == "size" {
			if child.Properties["default"] != 14 {
				t.Errorf("expected inherited default 14, got %v", child.Properties["default"])
			}
		}
	}
}

// --- Hierarchy derivation with recursive children ---

func TestMapWithChildrenDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	rootID, _ := source.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	childID, _ := source.InsertNode("folder", &rootID, map[string]interface{}{"name": "src"})
	source.InsertNode("file", &childID, map[string]interface{}{"name": "main.go"})
	source.InsertNode("file", &childID, map[string]interface{}{"name": "util.go"})

	pipeline := NewPipeline("p1", "tree-view").
		MapWithChildren("folder", "folder_view", func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			children := ctx.SourceChildren(sn.ID)
			return map[string]interface{}{
				"name":       sn.Properties["name"],
				"childCount": len(children),
			}
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	roots := ds.GetNodesByType("folder_view")
	// Should have derived root and "src" folder
	found := false
	for _, r := range roots {
		if r.Properties["name"] == "root" {
			found = true
			children := ds.GetChildren(r.ID)
			if len(children) == 0 {
				t.Error("root should have children")
			}
		}
	}
	if !found {
		t.Error("expected to find 'root' folder_view")
	}
}

// --- Join derivation (derived properties from related nodes) ---

func TestJoinDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	orgID, _ := source.InsertNode("org", nil, map[string]interface{}{"name": "Acme"})
	userID, _ := source.InsertNode("user", &orgID, map[string]interface{}{
		"name": "Alice",
	})
	// Edge: user -> org
	source.InsertEdge("member_of", userID, orgID, nil)

	pipeline := NewPipeline("p1", "user-enriched").
		Join("enriched_user", &JoinDef{
			SourceType: "user",
			Relations: []*RelationDef{
				{Name: "organization", Via: RelViaParent},
				{Name: "memberships", Via: RelViaOutEdge, EdgeType: "member_of"},
			},
		}, func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			parent, _ := ctx.SourceParent(sn.ID)
			orgName := ""
			if parent != nil {
				orgName, _ = parent.Properties["name"].(string)
			}
			return map[string]interface{}{
				"name":    sn.Properties["name"],
				"orgName": orgName,
			}
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	enriched := ds.GetNodesByType("enriched_user")
	if len(enriched) != 1 {
		t.Fatalf("expected 1 enriched_user, got %d", len(enriched))
	}
	if enriched[0].Properties["orgName"] != "Acme" {
		t.Errorf("expected orgName 'Acme', got %v", enriched[0].Properties["orgName"])
	}
}

// --- Multi-subtree merging ---

func TestMultiSubtreeDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	// Create two separate component trees
	comp1, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Header"})
	source.InsertNode("element", &comp1, map[string]interface{}{"name": "Title"})
	source.InsertNode("element", &comp1, map[string]interface{}{"name": "Logo"})

	comp2, _ := source.InsertNode("component", nil, map[string]interface{}{"name": "Footer"})
	source.InsertNode("element", &comp2, map[string]interface{}{"name": "Copyright"})

	// Create a page that references both components
	pageID, _ := source.InsertNode("page", nil, map[string]interface{}{
		"name":     "Home",
		"headerRef": comp1.String(),
		"footerRef": comp2.String(),
	})
	_ = pageID

	pipeline := NewPipeline("p1", "page-view").
		MultiSubtreeWithTransform("page_view", &MultiSubtreeDef{
			ParentType: "page",
			Sources: []*SubtreeSource{
				{
					Name:            "header",
					ResolveVia:      ResolveProperty,
					Property:        "headerRef",
					DerivedType:     "section",
					IncludeChildren: true,
				},
				{
					Name:            "footer",
					ResolveVia:      ResolveProperty,
					Property:        "footerRef",
					DerivedType:     "section",
					IncludeChildren: true,
				},
			},
		}, func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			return map[string]interface{}{
				"name": sn.Properties["name"],
			}
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	pages := ds.GetNodesByType("page_view")
	if len(pages) != 1 {
		t.Fatalf("expected 1 page_view, got %d", len(pages))
	}

	// Should have 2 section children (header + footer)
	sections := ds.GetChildren(pages[0].ID)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}

	// Each section should have its component's children
	totalElements := 0
	for _, section := range sections {
		children := ds.GetChildren(section.ID)
		totalElements += len(children)
	}
	if totalElements != 3 { // Title, Logo, Copyright
		t.Errorf("expected 3 total elements, got %d", totalElements)
	}
}

// --- Computed (fully programmatic) derivation ---

func TestComputedDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	source.InsertNode("product", nil, map[string]interface{}{"name": "Widget", "price": 10.0, "qty": 5})
	source.InsertNode("product", nil, map[string]interface{}{"name": "Gadget", "price": 20.0, "qty": 3})

	pipeline := NewPipeline("p1", "inventory").
		Compute(func(ctx *ComputeContext) {
			products := ctx.SourceNodesByType("product")
			totalValue := 0.0
			for _, p := range products {
				price, _ := p.Properties["price"].(float64)
				qty, _ := p.Properties["qty"].(int)
				totalValue += price * float64(qty)
			}

			summaryID := uuid.New()
			ctx.Emit(&DerivedNode{
				ID:          summaryID,
				DerivedType: "inventory_summary",
				Properties: map[string]interface{}{
					"totalProducts": len(products),
					"totalValue":    totalValue,
				},
			})
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	summaries := ds.GetNodesByType("inventory_summary")
	if len(summaries) != 1 {
		t.Fatalf("expected 1 inventory_summary, got %d", len(summaries))
	}
	if summaries[0].Properties["totalProducts"] != 2 {
		t.Errorf("expected 2 products, got %v", summaries[0].Properties["totalProducts"])
	}
	if summaries[0].Properties["totalValue"] != 110.0 {
		t.Errorf("expected total value 110, got %v", summaries[0].Properties["totalValue"])
	}
}

// --- Recursive derivation: chaining pipelines ---

func TestRecursiveDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	// Layer 1: Source tree
	rootID, _ := source.InsertNode("config", nil, map[string]interface{}{
		"name": "root",
		"env":  "production",
	})
	svcID, _ := source.InsertNode("service", &rootID, map[string]interface{}{
		"name": "api",
		"port": 8080,
	})
	source.InsertNode("endpoint", &svcID, map[string]interface{}{
		"name": "/users",
		"method": "GET",
	})

	// Pipeline 1: config tree -> deployment tree (first expansion)
	p1 := NewPipeline("p1", "deployment").
		MapWithChildren("config", "deployment_config", func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			return map[string]interface{}{
				"name":        sn.Properties["name"],
				"environment": sn.Properties["env"],
				"version":     "v1.0",
			}
		})
	ds.RegisterPipeline(p1)

	// Pipeline 2: runs on top of pipeline 1's output (second expansion)
	// This uses Compute to read derived nodes from p1 and expand further
	p2 := NewPipeline("p2", "runtime-expansion").
		Compute(func(ctx *ComputeContext) {
			deployments := ctx.GetDerivedNodesByType("deployment_config")
			for _, dep := range deployments {
				// For each deployment, create runtime monitoring nodes
				monitorID := uuid.New()
				pid := dep.ID
				ctx.Emit(&DerivedNode{
					ID:          monitorID,
					DerivedType: "monitor",
					ParentID:    &pid,
					Properties: map[string]interface{}{
						"target":   dep.Properties["name"],
						"interval": "30s",
						"env":      dep.Properties["environment"],
					},
				})

				// Create alert config
				alertID := uuid.New()
				ctx.Emit(&DerivedNode{
					ID:          alertID,
					DerivedType: "alert_config",
					ParentID:    &pid,
					Properties: map[string]interface{}{
						"target":    dep.Properties["name"],
						"threshold": 95.0,
					},
				})
			}
		})
	ds.RegisterPipeline(p2)

	ds.Recompute()

	// Layer 1 results
	deployments := ds.GetNodesByType("deployment_config")
	if len(deployments) == 0 {
		t.Fatal("expected deployment_config nodes")
	}

	// Layer 2 results (expanded from layer 1)
	monitors := ds.GetNodesByType("monitor")
	if len(monitors) == 0 {
		t.Fatal("expected monitor nodes from recursive derivation")
	}

	alerts := ds.GetNodesByType("alert_config")
	if len(alerts) == 0 {
		t.Fatal("expected alert_config nodes from recursive derivation")
	}

	// Verify hierarchy: monitor should be child of deployment_config
	for _, m := range monitors {
		parent, ok := ds.GetParent(m.ID)
		if !ok {
			t.Error("monitor should have a parent")
			continue
		}
		if parent.DerivedType != "deployment_config" {
			t.Errorf("monitor parent should be deployment_config, got %s", parent.DerivedType)
		}
	}
}

// --- Reactive recomputation ---

func TestReactiveRecomputation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)
	defer ds.Stop()

	pipeline := NewPipeline("p1", "users").
		Map("user", "user_view", nil)

	ds.RegisterPipeline(pipeline)

	// Insert initial data and compute
	source.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	ds.Recompute()

	if len(ds.GetNodesByType("user_view")) != 1 {
		t.Fatal("expected 1 user_view after initial compute")
	}

	// Insert another user — reactive recomputation happens async
	source.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	// Wait for async recomputation
	time.Sleep(50 * time.Millisecond)

	if len(ds.GetNodesByType("user_view")) != 2 {
		t.Errorf("expected 2 user_view after reactive update, got %d", len(ds.GetNodesByType("user_view")))
	}
}

// --- Source-to-derived tracking ---

func TestSourceToDerivedMapping(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	pipeline := NewPipeline("p1", "users").
		Map("user", "user_view", nil)

	ds.RegisterPipeline(pipeline)

	id, _ := source.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	ds.Recompute()

	derived := ds.GetDerivedForSource(id)
	if len(derived) != 1 {
		t.Fatalf("expected 1 derived node for source, got %d", len(derived))
	}
	if derived[0].Properties["name"] != "Alice" {
		t.Errorf("expected derived name 'Alice', got %v", derived[0].Properties["name"])
	}
}

// --- Conditional subtree derivation ---

func TestConditionalSubtreeDerivation(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	rootID, _ := source.InsertNode("app", nil, map[string]interface{}{"name": "MyApp"})
	source.InsertNode("feature", &rootID, map[string]interface{}{
		"name":    "dark_mode",
		"enabled": true,
	})
	source.InsertNode("feature", &rootID, map[string]interface{}{
		"name":    "beta_feature",
		"enabled": false,
	})
	source.InsertNode("feature", &rootID, map[string]interface{}{
		"name":    "notifications",
		"enabled": true,
	})

	// Only derive enabled features
	stage := &Stage{
		Type:           StageMap,
		SourceType:     "app",
		DerivedType:    "app_view",
		DeriveChildren: true,
		Filter:         nil, // derive all apps
		ChildRules: map[string]*Stage{
			"feature": {
				Type:        StageMap,
				DerivedType: "feature_view",
				Filter: func(sn *crdt.MaterializedNode) bool {
					return sn.Properties["enabled"] == true
				},
				Transform: func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
					return map[string]interface{}{
						"name":   sn.Properties["name"],
						"status": "active",
					}
				},
			},
		},
	}

	pipeline := NewPipeline("p1", "app-view").AddStage(stage)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	apps := ds.GetNodesByType("app_view")
	if len(apps) != 1 {
		t.Fatalf("expected 1 app_view, got %d", len(apps))
	}

	// Should only have 2 children (enabled features)
	children := ds.GetChildren(apps[0].ID)
	if len(children) != 2 {
		t.Errorf("expected 2 enabled features, got %d", len(children))
	}

	for _, child := range children {
		if child.Properties["status"] != "active" {
			t.Errorf("expected status 'active', got %v", child.Properties["status"])
		}
	}
}

// --- Deep merge strategy ---

func TestDeepMergeProperties(t *testing.T) {
	source := map[string]interface{}{
		"name": "Base",
		"style": map[string]interface{}{
			"color":      "blue",
			"fontSize":   14,
			"background": "white",
		},
		"data": "original",
	}
	overrides := map[string]interface{}{
		"name": "Override",
		"style": map[string]interface{}{
			"color":  "red",
			"border": "1px solid",
		},
	}

	merged := MergeProperties(source, overrides, InheritMerge)

	if merged["name"] != "Override" {
		t.Errorf("expected name 'Override', got %v", merged["name"])
	}
	if merged["data"] != "original" {
		t.Errorf("expected data 'original', got %v", merged["data"])
	}

	style := merged["style"].(map[string]interface{})
	if style["color"] != "red" {
		t.Errorf("expected style.color 'red', got %v", style["color"])
	}
	if style["fontSize"] != 14 {
		t.Errorf("expected style.fontSize 14, got %v", style["fontSize"])
	}
	if style["background"] != "white" {
		t.Errorf("expected style.background 'white', got %v", style["background"])
	}
	if style["border"] != "1px solid" {
		t.Errorf("expected style.border '1px solid', got %v", style["border"])
	}
}

// --- Execution mode ---

func TestPipelineExecutionModes(t *testing.T) {
	p := NewPipeline("p1", "test").WithMode(ExecClient)
	if p.ExecutionMode != ExecClient {
		t.Errorf("expected ExecClient, got %d", p.ExecutionMode)
	}

	p2 := NewPipeline("p2", "test2").WithMode(ExecBoth)
	if p2.ExecutionMode != ExecBoth {
		t.Errorf("expected ExecBoth, got %d", p2.ExecutionMode)
	}
}

// --- Edge derivation ---

func TestSubtreeWithEdges(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	a, _ := source.InsertNode("node", nil, map[string]interface{}{"name": "A"})
	b, _ := source.InsertNode("node", nil, map[string]interface{}{"name": "B"})
	edgeID, _ := source.InsertEdge("link", a, b, map[string]interface{}{"weight": 5})

	pipeline := NewPipeline("p1", "graph-view").
		Map("node", "node_view", func(sn *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{} {
			outEdges := ctx.SourceOutEdges(sn.ID)
			inEdges := ctx.SourceInEdges(sn.ID)
			return map[string]interface{}{
				"name":      sn.Properties["name"],
				"outDegree": len(outEdges),
				"inDegree":  len(inEdges),
			}
		})

	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	nodes := ds.GetNodesByType("node_view")
	if len(nodes) != 2 {
		t.Fatalf("expected 2 node_view, got %d", len(nodes))
	}

	for _, n := range nodes {
		if n.Properties["name"] == "A" {
			if n.Properties["outDegree"] != 1 {
				t.Errorf("A should have outDegree 1, got %v", n.Properties["outDegree"])
			}
		}
		if n.Properties["name"] == "B" {
			if n.Properties["inDegree"] != 1 {
				t.Errorf("B should have inDegree 1, got %v", n.Properties["inDegree"])
			}
		}
	}

	_ = edgeID
}

// --- GetSubtree ---

func TestGetSubtree(t *testing.T) {
	source := setupStore()
	ds := NewStore(source)

	rootID, _ := source.InsertNode("org", nil, map[string]interface{}{"name": "Root"})
	childID, _ := source.InsertNode("team", &rootID, map[string]interface{}{"name": "Eng"})
	source.InsertNode("member", &childID, map[string]interface{}{"name": "Alice"})

	pipeline := NewPipeline("p1", "org-view").
		MapWithChildren("org", "org_view", nil)
	ds.RegisterPipeline(pipeline)
	ds.Recompute()

	orgs := ds.GetNodesByType("org_view")
	if len(orgs) == 0 {
		t.Fatal("expected org_view nodes")
	}

	subtree := ds.GetSubtree(orgs[0].ID)
	if len(subtree) < 2 {
		t.Errorf("expected subtree with at least 2 nodes, got %d", len(subtree))
	}
}
