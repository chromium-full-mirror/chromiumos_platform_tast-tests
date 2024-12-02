// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nodematch matches cras audio nodes.
package nodematch

import (
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/local/audio/types"
)

// Matcher matches a CrasNode, used by *NodeByMatcher functions.
type Matcher interface {
	Match(*types.CrasNode) bool
	fmt.Stringer
}

// Name returns a Matcher that matches CrasNode's device name.
func Name(name string) Matcher {
	return &matchNodeName{Name: name}
}

type matchNodeName struct {
	Name string
}

var _ Matcher = matchNodeName{}

func (m matchNodeName) Match(n *types.CrasNode) bool {
	return m.Name == n.Name
}

func (m matchNodeName) String() string {
	return fmt.Sprintf("Name(%q)", m.Name)
}

// Type returns a Matcher that matches CrasNode's type.
func Type(ty string) Matcher {
	return &matchNodeType{Type: ty}
}

type matchNodeType struct {
	Type string
}

var _ Matcher = matchNodeType{}

func (m matchNodeType) Match(n *types.CrasNode) bool {
	if n.Type == m.Type {
		return true
	}
	// Regard the front mic as the internal mic.
	if m.Type == "INTERNAL_MIC" && n.Type == "FRONT_MIC" {
		return true
	}
	return false
}

func (m matchNodeType) String() string {
	return fmt.Sprintf("Type(%q)", m.Type)
}

// Direction returns a Matcher that matches CrasNode's direction.
func Direction(dir types.StreamType) Matcher {
	return &matchNodeDirection{Direction: dir}
}

var _ Matcher = matchNodeDirection{}

type matchNodeDirection struct {
	Direction types.StreamType
}

func (m matchNodeDirection) Match(n *types.CrasNode) bool {
	return (m.Direction == types.InputStream) == n.IsInput
}

func (m matchNodeDirection) String() string {
	return fmt.Sprintf("Direction(%s)", m.Direction)
}

// All returns a Matcher that matches if all matchers matches.
func All(matchers ...Matcher) Matcher {
	return &matchAll{matchers: matchers}
}

type matchAll struct {
	matchers []Matcher
}

var _ Matcher = matchAll{}

func (m matchAll) Match(n *types.CrasNode) bool {
	for _, matcher := range m.matchers {
		if !matcher.Match(n) {
			return false
		}
	}
	return true
}

func (m matchAll) String() string {
	var s strings.Builder
	s.WriteString("All(")
	for i, matcher := range m.matchers {
		if i > 0 {
			s.WriteString(", ")
		}
		s.WriteString(matcher.String())
	}
	s.WriteString(")")
	return s.String()
}
