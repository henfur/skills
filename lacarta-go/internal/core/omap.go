package core

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OMap is a JSON object that keeps its keys in insertion order, like a Python dict.
type OMap[V any] struct {
	keys []string
	vals map[string]V
}

func (m *OMap[V]) Get(k string) (V, bool) {
	v, ok := m.vals[k]
	return v, ok
}

func (m *OMap[V]) Set(k string, v V) {
	if m.vals == nil {
		m.vals = map[string]V{}
	}
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

func (m *OMap[V]) Delete(k string) {
	if _, ok := m.vals[k]; !ok {
		return
	}
	delete(m.vals, k)
	for i, x := range m.keys {
		if x == k {
			m.keys = append(m.keys[:i:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *OMap[V]) Keys() []string { return append([]string(nil), m.keys...) }
func (m *OMap[V]) Len() int       { return len(m.keys) }

func (m OMap[V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *OMap[V]) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected an object")
	}
	*m = OMap[V]{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var v V
		if err := dec.Decode(&v); err != nil {
			return err
		}
		m.Set(t.(string), v)
	}
	_, err = dec.Token()
	return err
}
