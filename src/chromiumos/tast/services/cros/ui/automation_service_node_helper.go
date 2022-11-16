// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

// NodeHelper is a helper that can easier generates *ui.Finder, used for automation service.
//
// Typical usage without this helper will be like:
//
//	dialog := &ui.Finder{
//		NodeWiths: []*ui.NodeWith{
//			{Value: &ui.NodeWith_Name{Name: "Join Wi-Fi network"}},
//			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_DIALOG}},
//		},
//	}
//	button := &ui.Finder{
//		NodeWiths: []*ui.NodeWith{
//			{Value: &ui.NodeWith_Name{Name: "Connect"}},
//			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
//			{Value: &ui.NodeWith_Ancestor{Ancestor: dialog}},
//		},
//	}
//	(ui.AutomationServiceClient).LeftClick(ctx, &ui.LeftClickRequest{Finder: button})
//
// With this helper, caller can simplify the same action as follow:
//
//	dialog := ui.Node().Name("Join Wi-Fi network").Role(ui.Role_ROLE_DIALOG).Finder()
//	button := ui.Node().Name("Connect").Role(ui.Role_ROLE_BUTTON).Ancestor(d.rootNode).Finder()
//	(ui.AutomationServiceClient).LeftClick(ctx, &ui.LeftClickRequest{Finder: button})
//
// Repeating any of the criteria will result in only the last one being used, i.e. these two finders are identical:
//
//	button1 := ui.Node().Name("Disconnect").Name("Connect").Finder()
//	button2 := ui.Node().Name("Connect").Finder()
type NodeHelper struct {
	name           string
	nameContaining string
	nameRegex      string
	role           Role
	hasClass       string
	ancestor       *Finder

	mask int
}

// Node creates a new NodeHelper instance.
func Node() *NodeHelper {
	return &NodeHelper{}
}

const (
	maskName = 1 << iota
	maskNameContaining
	maskNameRegex
	maskRole
	maskHasClass
	maskAncestor
)

// Name sets a specified name to NodeHelper.
func (n *NodeHelper) Name(name string) *NodeHelper {
	n.name = name
	n.mask |= maskName
	return n
}

// NameContaining sets a specified name segment to NodeHelper.
func (n *NodeHelper) NameContaining(s string) *NodeHelper {
	n.nameContaining = s
	n.mask |= maskNameContaining
	return n
}

// NameRegex sets a specified regex name to NodeHelper.
func (n *NodeHelper) NameRegex(nameRegex string) *NodeHelper {
	n.nameRegex = nameRegex
	n.mask |= maskNameRegex
	return n
}

// Role sets a specified role to NodeHelper.
func (n *NodeHelper) Role(role Role) *NodeHelper {
	n.role = role
	n.mask |= maskRole
	return n
}

// HasClass sets a specified class to NodeHelper.
func (n *NodeHelper) HasClass(hasClass string) *NodeHelper {
	n.hasClass = hasClass
	n.mask |= maskHasClass
	return n
}

// Ancestor sets a specified ancestor to NodeHelper.
func (n *NodeHelper) Ancestor(ancestor *Finder) *NodeHelper {
	n.ancestor = ancestor
	n.mask |= maskAncestor
	return n
}

// Finder returns the Finder.
func (n *NodeHelper) Finder() *Finder {
	var nodeWiths []*NodeWith

	if n.mask&maskName != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_Name{Name: n.name}})
	}

	if n.mask&maskNameContaining != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_NameContaining{NameContaining: n.nameContaining}})
	}

	if n.mask&maskNameRegex != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_NameRegex{NameRegex: n.nameRegex}})
	}

	if n.mask&maskRole != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_Role{Role: n.role}})
	}

	if n.mask&maskHasClass != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_HasClass{HasClass: n.hasClass}})
	}

	if n.mask&maskAncestor != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_Ancestor{Ancestor: n.ancestor}})
	}

	return &Finder{NodeWiths: nodeWiths}
}
