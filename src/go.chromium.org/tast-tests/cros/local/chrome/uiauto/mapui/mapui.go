// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mapui

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

type node struct {
	Name             string    `json:"name"`
	NameStartingWith string    `json:"nameStartingWith"`
	Role             role.Role `json:"role"`
	ClassName        string    `json:"className"`
}

// nodeFromStringVar creates a node finder from a raw JSON string.
func nodeFromStringVar(rawNode *testing.VarString) *nodewith.Finder {

	// node contains only the defined fields in the struct.
	var node node
	if err := json.Unmarshal([]byte(rawNode.Value()), &node); err != nil {
		panic(fmt.Sprintf("Failed to unmarshall the node string %q: %v", rawNode.Name(), err))
	}

	// nodeMap can contain any field, even ones that are not defined in the node struct.
	var nodeMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawNode.Value()), &nodeMap); err != nil {
		panic(fmt.Sprintf("Failed to unmarshall the node string %q: %v", rawNode.Name(), err))
	}

	//
	if len(nodeMap) == 0 {
		panic(fmt.Sprintf("The node descriptor %q cannot be empty", rawNode.Name()))
	}

	// Build up the returned node finder.
	finder := nodewith.Empty()

	if _, ok := nodeMap["name"]; ok {
		finder = finder.Name(node.Name)
		delete(nodeMap, "name")
	}

	if _, ok := nodeMap["nameStartingWith"]; ok {
		finder = finder.NameStartingWith(node.Name)
		delete(nodeMap, "nameStartingWith")
	}

	if _, ok := nodeMap["role"]; ok {
		finder = finder.Role(node.Role)
		delete(nodeMap, "role")
	}

	if _, ok := nodeMap["className"]; ok {
		finder = finder.ClassName(node.ClassName)
		delete(nodeMap, "className")
	}

	// Check if every value in the rawNode is defined in the node struct.
	// This can catch typos in the rawNode.
	if len(nodeMap) != 0 {
		keys := make([]string, 0, len(nodeMap))
		for k := range nodeMap {
			keys = append(keys, k)
		}
		panic(fmt.Sprintf("Failed to parse the following field(s): %s", strings.Join(keys, ", ")))
	}

	return finder
}
