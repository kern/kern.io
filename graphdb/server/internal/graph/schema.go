package graph

import (
	"encoding/json"
	"fmt"
)

// Schema defines the structure of the graph: what node types exist,
// what properties they have, and what edges/hierarchy relationships are allowed.
type Schema struct {
	NodeTypes map[string]*NodeTypeDef `json:"nodeTypes"`
	EdgeTypes map[string]*EdgeTypeDef `json:"edgeTypes"`
}

// NodeTypeDef defines a node type.
type NodeTypeDef struct {
	Name       string                    `json:"name"`
	Properties map[string]*PropertyDef   `json:"properties"`
	// AllowedChildren lists which node types can be children in the hierarchy
	AllowedChildren []string             `json:"allowedChildren,omitempty"`
	// AllowedParents lists which node types can be parents in the hierarchy
	AllowedParents  []string             `json:"allowedParents,omitempty"`
}

// PropertyDef defines a property on a node type.
type PropertyDef struct {
	Name     string      `json:"name"`
	Type     PropertyType `json:"type"`
	Required bool        `json:"required"`
	// Indexed means a secondary index is maintained for this property
	Indexed  bool        `json:"indexed"`
	// Unique means no two nodes of the same type can have the same value
	Unique   bool        `json:"unique"`
}

// PropertyType is the type of a property value.
type PropertyType string

const (
	PropString  PropertyType = "string"
	PropNumber  PropertyType = "number"
	PropBool    PropertyType = "boolean"
	PropArray   PropertyType = "array"
	PropObject  PropertyType = "object"
	PropRef     PropertyType = "ref"
	PropAny     PropertyType = "any"
)

// EdgeTypeDef defines an edge type with source/target type constraints.
type EdgeTypeDef struct {
	Name       string   `json:"name"`
	FromTypes  []string `json:"fromTypes"`  // allowed source node types
	ToTypes    []string `json:"toTypes"`    // allowed target node types
	Properties map[string]*PropertyDef `json:"properties,omitempty"`
}

// NewSchema creates a new empty schema.
func NewSchema() *Schema {
	return &Schema{
		NodeTypes: make(map[string]*NodeTypeDef),
		EdgeTypes: make(map[string]*EdgeTypeDef),
	}
}

// DefineNode adds a node type definition to the schema.
func (s *Schema) DefineNode(def *NodeTypeDef) {
	s.NodeTypes[def.Name] = def
}

// DefineEdge adds an edge type definition to the schema.
func (s *Schema) DefineEdge(def *EdgeTypeDef) {
	s.EdgeTypes[def.Name] = def
}

// ValidateNode validates a node's properties against the schema.
func (s *Schema) ValidateNode(nodeType string, properties map[string]interface{}) error {
	typeDef, ok := s.NodeTypes[nodeType]
	if !ok {
		// If no schema defined for this type, allow anything
		return nil
	}

	// Check required properties
	for propName, propDef := range typeDef.Properties {
		if propDef.Required {
			if _, ok := properties[propName]; !ok {
				return fmt.Errorf("required property %q missing on node type %q", propName, nodeType)
			}
		}
	}

	// Validate property types
	for key, value := range properties {
		if err := s.ValidateProperty(nodeType, key, value); err != nil {
			return err
		}
	}

	return nil
}

// ValidateProperty validates a single property value.
func (s *Schema) ValidateProperty(nodeType, key string, value interface{}) error {
	typeDef, ok := s.NodeTypes[nodeType]
	if !ok {
		return nil // no schema
	}

	propDef, ok := typeDef.Properties[key]
	if !ok {
		return nil // allow extra properties
	}

	return validateType(propDef.Type, key, value)
}

func validateType(expected PropertyType, key string, value interface{}) error {
	if expected == PropAny {
		return nil
	}

	switch expected {
	case PropString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("property %q: expected string, got %T", key, value)
		}
	case PropNumber:
		switch value.(type) {
		case int, int32, int64, float32, float64, json.Number:
			// ok
		default:
			return fmt.Errorf("property %q: expected number, got %T", key, value)
		}
	case PropBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("property %q: expected boolean, got %T", key, value)
		}
	case PropArray:
		switch value.(type) {
		case []interface{}, []string, []int, []float64:
			// ok
		default:
			return fmt.Errorf("property %q: expected array, got %T", key, value)
		}
	case PropObject:
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("property %q: expected object, got %T", key, value)
		}
	case PropRef:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("property %q: expected ref (string UUID), got %T", key, value)
		}
	}
	return nil
}

// ValidateHierarchy checks if a child type is allowed under a parent type.
func (s *Schema) ValidateHierarchy(parentType, childType string) error {
	parentDef, ok := s.NodeTypes[parentType]
	if !ok {
		return nil // no schema, allow anything
	}

	// If AllowedChildren is empty, allow anything
	if len(parentDef.AllowedChildren) == 0 {
		return nil
	}

	for _, allowed := range parentDef.AllowedChildren {
		if allowed == childType || allowed == "*" {
			return nil
		}
	}

	return fmt.Errorf("node type %q cannot have children of type %q", parentType, childType)
}

// ValidateEdge checks if an edge type is allowed between two node types.
func (s *Schema) ValidateEdge(edgeType, fromType, toType string) error {
	edgeDef, ok := s.EdgeTypes[edgeType]
	if !ok {
		return nil // no schema, allow anything
	}

	// Check source type
	if len(edgeDef.FromTypes) > 0 {
		found := false
		for _, t := range edgeDef.FromTypes {
			if t == fromType || t == "*" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("edge type %q cannot originate from node type %q", edgeType, fromType)
		}
	}

	// Check target type
	if len(edgeDef.ToTypes) > 0 {
		found := false
		for _, t := range edgeDef.ToTypes {
			if t == toType || t == "*" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("edge type %q cannot target node type %q", edgeType, toType)
		}
	}

	return nil
}

