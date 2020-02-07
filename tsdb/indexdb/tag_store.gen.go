// Generated by tmpl
// https://github.com/benbjohnson/tmpl
//
// DO NOT EDIT!
// Source: int_map.tmpl

package indexdb

import (
	"github.com/lindb/roaring"
)

// TagStore represents int map using roaring bitmap
type TagStore struct {
	keys   *roaring.Bitmap     // store all keys
	values [][]*roaring.Bitmap // store all values by high/low key
}

// NewTagStore creates a int map
func NewTagStore() *TagStore {
	return &TagStore{
		keys: roaring.New(),
	}
}

// Get returns value by key, if exist returns it, else returns nil, false
func (m *TagStore) Get(key uint32) (*roaring.Bitmap, bool) {
	if len(m.values) == 0 {
		return nil, false
	}
	found, highIdx, lowIdx := m.keys.ContainsAndRank(key)
	if !found {
		return nil, false
	}
	return m.values[highIdx][lowIdx-1], true
}

// Put puts the value by key
func (m *TagStore) Put(key uint32, value *roaring.Bitmap) {
	if len(m.values) == 0 {
		// if values is empty, append new low container directly
		m.keys.Add(key)
		m.values = append(m.values, []*roaring.Bitmap{value})
		return
	}

	// try find key if exist
	found, highIdx, lowIdx := m.keys.ContainsAndRank(key)
	if !found {
		// not found
		m.keys.Add(key)
		if highIdx >= 0 {
			// high container exist
			stores := m.values[highIdx]
			// insert operation
			stores = append(stores, nil)
			copy(stores[lowIdx+1:], stores[lowIdx:len(stores)-1])
			stores[lowIdx] = value
			m.values[highIdx] = stores
		} else {
			// high container not exist, append operation
			m.values = append(m.values, []*roaring.Bitmap{value})
		}
	}
}

// Keys returns the all keys
func (m *TagStore) Keys() *roaring.Bitmap {
	return m.keys
}

// Values returns the all values
func (m *TagStore) Values() [][]*roaring.Bitmap {
	return m.values
}

// size returns the size of keys
func (m *TagStore) Size() int {
	return int(m.keys.GetCardinality())
}