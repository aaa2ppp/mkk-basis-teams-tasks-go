package model

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
)

// Nullable allows you to distinguish the absence of a field in the incoming JSON from the null value.
// If the field is present in JSON, Defined = true. If the field is null or missing, Valid = false.
// For data coming from the database, Defined = Valid always.
// Use 'omitzero' in the 'json' tag for proper serialization.
type Nullable[T any] struct {
	sql.Null[T] `json:"-"`
	Defined     bool `json:"-"`
}

// MarshalJSON implements [json.Marshaler].
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.V)
}

// UnmarshalJSON implements [json.Unmarshaler].
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*n = Nullable[T]{}
		n.Defined = true
		return nil
	}
	if err := json.Unmarshal(b, &n.V); err != nil {
		return err
	}
	n.Defined = true
	n.Valid = true
	return nil
}

func (n Nullable[T]) IsZero() bool {
	return !n.Defined
}

func (n *Nullable[T]) Scan(value any) error {
	if err := n.Null.Scan(value); err != nil {
		return err
	}
	n.Defined = n.Valid
	return nil
}

var _ json.Unmarshaler = &Nullable[int]{}
var _ json.Marshaler = Nullable[int]{}
var _ sql.Scanner = &Nullable[int]{}
var _ driver.Valuer = Nullable[int]{}
