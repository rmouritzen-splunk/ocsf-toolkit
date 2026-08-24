// Package coderegistry implements the stable-external-representation lookup shared by this toolkit's small
// uint8-backed code enums (issue.IssueCode, validation.Code): a fixed metadata table indexed by the code itself,
// with code 0 reserved as the invalid zero value.
package coderegistry

import "fmt"

// Info is the stable external metadata common to every registry entry.
type Info struct {
	Name        string
	Description string
	Mandatory   bool
}

// Registry validates and resolves codes of type T against a fixed-size metadata table built by New. kind names T in
// error messages, for example "issue code" or "validation code".
type Registry[T ~uint8] struct {
	infos []Info
	kind  string
}

// New builds a Registry from infos, a table indexed by T's underlying value (index 0 unused).
func New[T ~uint8](kind string, infos []Info) Registry[T] {
	return Registry[T]{infos: infos, kind: kind}
}

// Valid reports whether code has an entry in the table.
func (r Registry[T]) Valid(code T) bool {
	return code > 0 && int(code) < len(r.infos)
}

// Codes returns every valid code in table order.
func (r Registry[T]) Codes() []T {
	codes := make([]T, 0, len(r.infos)-1)
	for index := 1; index < len(r.infos); index++ {
		codes = append(codes, T(index))
	}
	return codes
}

// String returns the stable external representation of code, or an empty string for an invalid code.
func (r Registry[T]) String(code T) string {
	if !r.Valid(code) {
		return ""
	}
	return r.infos[code].Name
}

// Description returns the short human-readable explanation of code, or an empty string for an invalid code.
func (r Registry[T]) Description(code T) string {
	if !r.Valid(code) {
		return ""
	}
	return r.infos[code].Description
}

// Suppressible reports whether code is valid and its metadata does not mark it mandatory.
func (r Registry[T]) Suppressible(code T) bool {
	return r.Valid(code) && !r.infos[code].Mandatory
}

// Parse resolves a stable external representation to its code.
func (r Registry[T]) Parse(value string) (T, bool) {
	for index := 1; index < len(r.infos); index++ {
		if r.infos[index].Name == value {
			return T(index), true
		}
	}
	return 0, false
}

// MarshalText returns the stable external representation of code used by JSON encoders.
func (r Registry[T]) MarshalText(code T) ([]byte, error) {
	if !r.Valid(code) {
		return nil, fmt.Errorf("invalid %s %d", r.kind, code)
	}
	return []byte(r.infos[code].Name), nil
}

// UnmarshalText resolves a stable external representation used by JSON decoders into *code.
func (r Registry[T]) UnmarshalText(text []byte, code *T) error {
	parsed, ok := r.Parse(string(text))
	if !ok {
		return fmt.Errorf("unknown %s %q", r.kind, text)
	}
	*code = parsed
	return nil
}
