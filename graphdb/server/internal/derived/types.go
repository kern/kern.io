// Package derived provides a type-safe derived graph layer that sits on top
// of the persistence layer (graph.Store). It allows clients to define:
//
//   - Typed node definitions with strongly-typed properties
//   - Derivation rules that compute derived nodes from persistent source nodes
//   - Subtree inheritance: an "instance" derives its subtree from a "component"
//   - Overlay semantics: instance-local overrides on top of inherited properties
//
// The derived layer is reactive: when source nodes change, affected derived
// nodes are automatically recomputed.
package derived

import (
	"fmt"
)

// FieldType is the type of a field in a derived node definition.
type FieldType int

const (
	FieldString FieldType = iota
	FieldInt
	FieldFloat
	FieldBool
	FieldStringSlice
	FieldMap       // map[string]interface{}
	FieldRef       // reference to another node (UUID string)
	FieldRefSlice  // slice of node references
	FieldAny       // untyped
	FieldStruct    // nested typed struct (uses SubFields)
)

// FieldDef defines a single field on a derived node type.
type FieldDef struct {
	Name         string    `json:"name"`
	Type         FieldType `json:"type"`
	Required     bool      `json:"required"`
	DefaultValue interface{} `json:"defaultValue,omitempty"`
	// For FieldStruct: nested field definitions
	SubFields    []*FieldDef `json:"subFields,omitempty"`
	// For FieldRef/FieldRefSlice: the target node type
	RefType      string    `json:"refType,omitempty"`
}

// DerivedNodeType defines a typed node in the derived graph.
// It specifies which fields exist and how they're typed,
// and optionally how this node is derived from the persistence layer.
type DerivedNodeType struct {
	Name   string      `json:"name"`
	Fields []*FieldDef `json:"fields"`

	// Source mapping: which persistent node type(s) this derives from
	SourceType string `json:"sourceType,omitempty"`

	// Inheritance: if set, instances of this type inherit subtrees from another type
	Inherits *InheritanceDef `json:"inherits,omitempty"`

	// field index for fast lookup
	fieldIndex map[string]*FieldDef
}

// InheritanceDef defines how a derived node type inherits from another.
// This is the core of component-instance relationships.
type InheritanceDef struct {
	// The node type to inherit from (e.g., "component")
	FromType string `json:"fromType"`
	// Property on the instance that references the source (e.g., "componentId")
	RefProperty string `json:"refProperty"`
	// What to inherit
	Strategy InheritStrategy `json:"strategy"`
	// Which child types to inherit (empty = all)
	ChildTypes []string `json:"childTypes,omitempty"`
}

// InheritStrategy determines how inheritance works.
type InheritStrategy int

const (
	// InheritSubtree: clone the entire subtree from the source.
	// Instance overrides take precedence over inherited values.
	InheritSubtree InheritStrategy = iota
	// InheritProperties: only inherit direct properties (no children).
	InheritProperties
	// InheritMerge: deep-merge source properties with instance properties.
	InheritMerge
)

// NewDerivedNodeType creates a new derived node type definition.
func NewDerivedNodeType(name string) *DerivedNodeType {
	return &DerivedNodeType{
		Name:       name,
		fieldIndex: make(map[string]*FieldDef),
	}
}

// Field adds a field to the node type.
func (d *DerivedNodeType) Field(name string, fieldType FieldType, required bool) *DerivedNodeType {
	f := &FieldDef{Name: name, Type: fieldType, Required: required}
	d.Fields = append(d.Fields, f)
	d.fieldIndex[name] = f
	return d
}

// FieldWithDefault adds a field with a default value.
func (d *DerivedNodeType) FieldWithDefault(name string, fieldType FieldType, defaultVal interface{}) *DerivedNodeType {
	f := &FieldDef{Name: name, Type: fieldType, Required: false, DefaultValue: defaultVal}
	d.Fields = append(d.Fields, f)
	d.fieldIndex[name] = f
	return d
}

// RefField adds a reference field pointing to another node type.
func (d *DerivedNodeType) RefField(name string, targetType string, required bool) *DerivedNodeType {
	f := &FieldDef{Name: name, Type: FieldRef, Required: required, RefType: targetType}
	d.Fields = append(d.Fields, f)
	d.fieldIndex[name] = f
	return d
}

// StructField adds a nested struct field.
func (d *DerivedNodeType) StructField(name string, required bool, subFields ...*FieldDef) *DerivedNodeType {
	f := &FieldDef{Name: name, Type: FieldStruct, Required: required, SubFields: subFields}
	d.Fields = append(d.Fields, f)
	d.fieldIndex[name] = f
	return d
}

// DeriveFrom configures this type to derive from a persistent source type.
func (d *DerivedNodeType) DeriveFrom(sourceType string) *DerivedNodeType {
	d.SourceType = sourceType
	return d
}

// InheritsFrom configures subtree inheritance (component-instance pattern).
func (d *DerivedNodeType) InheritsFrom(fromType, refProperty string, strategy InheritStrategy) *DerivedNodeType {
	d.Inherits = &InheritanceDef{
		FromType:    fromType,
		RefProperty: refProperty,
		Strategy:    strategy,
	}
	return d
}

// InheritsChildTypes restricts which child types are inherited.
func (d *DerivedNodeType) InheritsChildTypes(types ...string) *DerivedNodeType {
	if d.Inherits != nil {
		d.Inherits.ChildTypes = types
	}
	return d
}

// GetField looks up a field definition by name.
func (d *DerivedNodeType) GetField(name string) (*FieldDef, bool) {
	if d.fieldIndex == nil {
		d.buildIndex()
	}
	f, ok := d.fieldIndex[name]
	return f, ok
}

func (d *DerivedNodeType) buildIndex() {
	d.fieldIndex = make(map[string]*FieldDef, len(d.Fields))
	for _, f := range d.Fields {
		d.fieldIndex[f.Name] = f
	}
}

// Validate checks that a property map conforms to this type definition.
func (d *DerivedNodeType) Validate(properties map[string]interface{}) error {
	if d.fieldIndex == nil {
		d.buildIndex()
	}

	// Check required fields
	for _, f := range d.Fields {
		if f.Required {
			if _, ok := properties[f.Name]; !ok {
				return fmt.Errorf("required field %q missing on derived type %q", f.Name, d.Name)
			}
		}
	}

	// Type-check present fields
	for key, val := range properties {
		f, ok := d.fieldIndex[key]
		if !ok {
			continue // allow extra fields
		}
		if err := validateFieldType(f, val); err != nil {
			return fmt.Errorf("field %q on type %q: %w", key, d.Name, err)
		}
	}
	return nil
}

// ApplyDefaults fills in default values for missing fields.
func (d *DerivedNodeType) ApplyDefaults(properties map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(properties))
	for k, v := range properties {
		result[k] = v
	}
	for _, f := range d.Fields {
		if _, ok := result[f.Name]; !ok && f.DefaultValue != nil {
			result[f.Name] = f.DefaultValue
		}
	}
	return result
}

func validateFieldType(f *FieldDef, val interface{}) error {
	if val == nil {
		if f.Required {
			return fmt.Errorf("value is nil but field is required")
		}
		return nil
	}

	switch f.Type {
	case FieldString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case FieldInt:
		switch val.(type) {
		case int, int32, int64, float64:
		default:
			return fmt.Errorf("expected int, got %T", val)
		}
	case FieldFloat:
		switch val.(type) {
		case float32, float64, int, int64:
		default:
			return fmt.Errorf("expected float, got %T", val)
		}
	case FieldBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", val)
		}
	case FieldStringSlice:
		switch val.(type) {
		case []string, []interface{}:
		default:
			return fmt.Errorf("expected string slice, got %T", val)
		}
	case FieldMap:
		if _, ok := val.(map[string]interface{}); !ok {
			return fmt.Errorf("expected map, got %T", val)
		}
	case FieldRef:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected ref (string), got %T", val)
		}
	case FieldRefSlice:
		switch val.(type) {
		case []string, []interface{}:
		default:
			return fmt.Errorf("expected ref slice, got %T", val)
		}
	case FieldStruct:
		m, ok := val.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected struct (map), got %T", val)
		}
		// Validate sub-fields
		for _, sf := range f.SubFields {
			if sv, ok := m[sf.Name]; ok {
				if err := validateFieldType(sf, sv); err != nil {
					return fmt.Errorf("sub-field %q: %w", sf.Name, err)
				}
			} else if sf.Required {
				return fmt.Errorf("required sub-field %q missing", sf.Name)
			}
		}
	case FieldAny:
		// anything goes
	}
	return nil
}

// F is a shorthand for creating field definitions (used in StructField).
func F(name string, fieldType FieldType, required bool) *FieldDef {
	return &FieldDef{Name: name, Type: fieldType, Required: required}
}
