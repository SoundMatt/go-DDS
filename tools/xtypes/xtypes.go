// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package xtypes implements Dynamic Data for go-DDS (Milestone 9).
//
// XTypes provides a runtime type system that allows distributed systems to
// evolve their data schemas without lock-step software updates. It is
// loosely aligned with the OMG DDS-XTypes 1.3 specification but simplified
// for idiomatic Go use.
//
// # Type System
//
// A [TypeDescriptor] describes the structure of a named type: its fields,
// their primitive kinds, and optional/required status. A [TypeIdentifier]
// is a compact, content-addressed fingerprint of a descriptor derived from
// its stable canonical hash.
//
// # Dynamic Data
//
// [DynamicData] is a schema-validated property map: values may only be set
// on fields declared in the associated [TypeDescriptor]. It serialises
// transparently to/from JSON.
//
// # Type Registry
//
// [TypeRegistry] is a thread-safe store of [TypeObject] values (descriptor +
// identifier pairs). Participants use the registry to announce their types
// and to resolve types received from peers.
//
// # Compatibility Checking
//
// [CheckCompatibility] implements the standard forward/backward type evolution
// rules:
//   - New optional fields added to the writer are invisible but harmless to
//     an older reader (forward compatibility).
//   - New required fields expected by the reader but absent from the writer
//     are incompatible (the reader would receive data it cannot interpret).
//   - Renamed or type-changed fields are always incompatible.
package xtypes

//fusa:req REQ-XTYPE-001
//fusa:req REQ-XTYPE-002
//fusa:req REQ-XTYPE-003
//fusa:req REQ-XTYPE-004
//fusa:req REQ-XTYPE-005
//fusa:req REQ-XTYPE-006

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ── Type kinds ────────────────────────────────────────────────────────────────

// TypeKind identifies the primitive kind of a [FieldDescriptor].
type TypeKind uint8

const (
	KindBool    TypeKind = iota // bool
	KindInt32                   // int32
	KindInt64                   // int64
	KindFloat64                 // float64
	KindString                  // string
	KindBytes                   // []byte
	KindStruct                  // nested struct; Fields contains sub-descriptors
	KindSeq                     // variable-length sequence; Element points to element descriptor
)

// String returns a human-readable name for the kind.
func (k TypeKind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindInt32:
		return "int32"
	case KindInt64:
		return "int64"
	case KindFloat64:
		return "float64"
	case KindString:
		return "string"
	case KindBytes:
		return "bytes"
	case KindStruct:
		return "struct"
	case KindSeq:
		return "seq"
	default:
		return fmt.Sprintf("TypeKind(%d)", k)
	}
}

// ── Descriptors ───────────────────────────────────────────────────────────────

// FieldDescriptor describes one field within a [TypeDescriptor].
type FieldDescriptor struct {
	Name     string            // field name; must be unique within its parent
	Kind     TypeKind          // primitive kind
	Optional bool              // if false, the field is required
	Fields   []FieldDescriptor // for KindStruct: nested field descriptors
	Element  *FieldDescriptor  // for KindSeq: element type descriptor
}

// TypeDescriptor describes a complete named schema.
type TypeDescriptor struct {
	Name    string            // type name; must be unique within a [TypeRegistry]
	Version uint32            // schema version; 0 = unversioned
	Fields  []FieldDescriptor // top-level fields
}

// ── Type identity ─────────────────────────────────────────────────────────────

// TypeIdentifier is a compact, content-addressed reference to a [TypeDescriptor].
// Two descriptors with identical structure produce the same TypeIdentifier
// regardless of the order in which they were created.
type TypeIdentifier struct {
	Name string  // type name
	Hash [8]byte // first 8 bytes of SHA-256(canonical JSON representation)
}

// Identify computes the [TypeIdentifier] for td.
// The hash is derived from a sorted, canonical JSON encoding of the descriptor
// so that structurally identical types hash identically.
func Identify(td *TypeDescriptor) TypeIdentifier {
	b, jsonErr := json.Marshal(canonical(td))
	_ = jsonErr // json.Marshal on canonical descriptor never fails
	sum := sha256.Sum256(b)
	var h [8]byte
	copy(h[:], sum[:])
	return TypeIdentifier{Name: td.Name, Hash: h}
}

// canonical returns a stable map representation of td for hashing.
func canonical(td *TypeDescriptor) map[string]any {
	fields := make([]map[string]any, len(td.Fields))
	for i, f := range td.Fields {
		fields[i] = canonicalField(&f)
	}
	// Sort fields by name for stability.
	sort.Slice(fields, func(i, j int) bool {
		ni, _ := fields[i]["name"].(string)
		nj, _ := fields[j]["name"].(string)
		return ni < nj
	})
	return map[string]any{
		"name":    td.Name,
		"version": td.Version,
		"fields":  fields,
	}
}

func canonicalField(f *FieldDescriptor) map[string]any {
	m := map[string]any{
		"name":     f.Name,
		"kind":     f.Kind.String(),
		"optional": f.Optional,
	}
	if len(f.Fields) > 0 {
		nested := make([]map[string]any, len(f.Fields))
		for i, child := range f.Fields {
			nested[i] = canonicalField(&child)
		}
		sort.Slice(nested, func(i, j int) bool {
			ni, _ := nested[i]["name"].(string)
			nj, _ := nested[j]["name"].(string)
			return ni < nj
		})
		m["fields"] = nested
	}
	if f.Element != nil {
		m["element"] = canonicalField(f.Element)
	}
	return m
}

// TypeObject pairs a [TypeDescriptor] with its content-addressed [TypeIdentifier].
type TypeObject struct {
	ID         TypeIdentifier
	Descriptor TypeDescriptor
}

// NewTypeObject builds a TypeObject from td, computing the identifier automatically.
func NewTypeObject(td TypeDescriptor) *TypeObject {
	return &TypeObject{ID: Identify(&td), Descriptor: td}
}

// ── Dynamic Data ──────────────────────────────────────────────────────────────

// DynamicData is a schema-validated property map. Values can only be set on
// fields declared in the associated [TypeDescriptor].
//
// DynamicData is NOT safe for concurrent use from multiple goroutines.
type DynamicData struct {
	typeDesc *TypeDescriptor
	fields   map[string]any
}

// NewDynamicData creates a DynamicData backed by td.
func NewDynamicData(td *TypeDescriptor) *DynamicData {
	return &DynamicData{typeDesc: td, fields: make(map[string]any)}
}

// TypeDescriptor returns the schema backing this DynamicData.
func (d *DynamicData) TypeDescriptor() *TypeDescriptor { return d.typeDesc }

// Set sets the named field to value.
// Returns an error if name does not exist in the descriptor.
func (d *DynamicData) Set(name string, value any) error {
	if !d.hasField(name, d.typeDesc.Fields) {
		return fmt.Errorf("xtypes: unknown field %q in type %q", name, d.typeDesc.Name)
	}
	d.fields[name] = value
	return nil
}

// Get returns the value for name. The second return is false if the field
// has not been set (even if it is declared in the descriptor).
func (d *DynamicData) Get(name string) (any, bool) {
	v, ok := d.fields[name]
	return v, ok
}

// ToJSON serialises the set fields to JSON.
func (d *DynamicData) ToJSON() ([]byte, error) {
	return json.Marshal(d.fields)
}

// FromJSON populates set fields from JSON. Only keys that match declared
// fields in the descriptor are stored; unknown keys are silently ignored
// (forward compatibility: old code reads new data).
func (d *DynamicData) FromJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("xtypes: FromJSON: %w", err)
	}
	for k, v := range raw {
		if !d.hasField(k, d.typeDesc.Fields) {
			continue // forward-compat: skip unknown fields
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return fmt.Errorf("xtypes: FromJSON field %q: %w", k, err)
		}
		d.fields[k] = val
	}
	return nil
}

func (d *DynamicData) hasField(name string, fields []FieldDescriptor) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// ── Type Registry ─────────────────────────────────────────────────────────────

// ErrTypeMismatch is returned by [TypeRegistry.Register] when a type with the
// same name but a different hash is already registered.
var ErrTypeMismatch = errors.New("xtypes: type name already registered with different structure")

// TypeRegistry is a thread-safe store for [TypeObject] values.
type TypeRegistry struct {
	mu    sync.RWMutex
	types map[string]*TypeObject
}

// NewTypeRegistry creates an empty TypeRegistry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{types: make(map[string]*TypeObject)}
}

// Register stores to. Returns [ErrTypeMismatch] if a type with the same Name
// but a different hash is already registered. Registering the same TypeObject
// twice (same name and hash) is a no-op.
func (r *TypeRegistry) Register(to *TypeObject) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.types[to.ID.Name]
	if ok {
		if existing.ID.Hash != to.ID.Hash {
			return ErrTypeMismatch
		}
		return nil // identical re-registration is fine
	}
	r.types[to.ID.Name] = to
	return nil
}

// Lookup returns the TypeObject registered under name, if any.
func (r *TypeRegistry) Lookup(name string) (*TypeObject, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	to, ok := r.types[name]
	return to, ok
}

// All returns a snapshot of all registered TypeObjects in name-sorted order.
func (r *TypeRegistry) All() []*TypeObject {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*TypeObject, 0, len(r.types))
	for _, to := range r.types {
		out = append(out, to)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID.Name < out[j].ID.Name
	})
	return out
}

// ── Compatibility ─────────────────────────────────────────────────────────────

// CompatibilityResult describes whether a reader using one type descriptor can
// safely consume data produced by a writer using another.
type CompatibilityResult struct {
	Compatible bool   // true if reader and writer are compatible
	Reason     string // human-readable explanation when Compatible is false
}

// CheckCompatibility reports whether a reader using readerTD can safely consume
// data written by a writer using writerTD.
//
// The rules follow standard schema-evolution conventions:
//   - Fields present in writer but absent in reader: always OK (forward compat).
//   - Fields present in reader but absent in writer: OK only if Optional in reader.
//   - Fields present in both: compatible only if they have the same Kind.
func CheckCompatibility(writerTD, readerTD *TypeDescriptor) CompatibilityResult {
	writerFields := fieldMap(writerTD.Fields)
	for _, rf := range readerTD.Fields {
		wf, exists := writerFields[rf.Name]
		if !exists {
			if !rf.Optional {
				return CompatibilityResult{
					Compatible: false,
					Reason: fmt.Sprintf("required field %q expected by reader is absent from writer",
						rf.Name),
				}
			}
			continue
		}
		if wf.Kind != rf.Kind {
			return CompatibilityResult{
				Compatible: false,
				Reason: fmt.Sprintf("field %q: reader expects %s, writer provides %s",
					rf.Name, rf.Kind, wf.Kind),
			}
		}
	}
	return CompatibilityResult{Compatible: true}
}

func fieldMap(fields []FieldDescriptor) map[string]FieldDescriptor {
	m := make(map[string]FieldDescriptor, len(fields))
	for _, f := range fields {
		m[f.Name] = f
	}
	return m
}
