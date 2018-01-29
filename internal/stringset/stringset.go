// Copyright © 2018 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package stringset implements a very basic set collection of strings. Items
// are unique, and basic set operations of addition, deletion, and existence
// testing are supported. Additionally, supports exporting the elements as a
// slice, optionally with the values sorted in ascending order.
//
// While the functionality of this package could easily be expanded, it would
// likely soon be better to use a more full-featured, public set package.
package stringset

import (
	"sort"
)

// Set represents a set collection of strings.
type Set map[string]struct{}

// New returns a new Set object.
func New(values ...string) Set {
	s := make(Set)
	s.Add(values...)
	return s
}

// Add adds one or more strings to the set
func (s *Set) Add(values ...string) {
	for _, v := range values {
		(*s)[v] = struct{}{}
	}
}

// Delete removes one or more strings from the set
func (s *Set) Delete(values ...string) {
	for _, v := range values {
		delete(*s, v)
	}
}

// Contains checks if the set contains a string
func (s *Set) Contains(value string) bool {
	_, c := (*s)[value]
	return c
}

// Values returns the strings in the set as a slice of strings
func (s *Set) Values() []string {
	keys := make([]string, 0, len(*s))
	for k := range *s {
		keys = append(keys, k)
	}
	return keys
}

// Sort returns the strings in the set as a sorted slice of strings
func (s *Set) Sort() []string {
	v := s.Values()
	sort.Strings(v)
	return v
}
