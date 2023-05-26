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
	nth            int32
	expanded       bool

	mask int
}

// Node creates a new NodeHelper instance.
func Node() *NodeHelper {
	return &NodeHelper{}
}

func (n *NodeHelper) copy() *NodeHelper {
	return &NodeHelper{
		name:           n.name,
		nameContaining: n.nameContaining,
		nameRegex:      n.nameRegex,
		role:           n.role,
		hasClass:       n.hasClass,
		ancestor:       n.ancestor,
		nth:            n.nth,
		expanded:       n.expanded,
		mask:           n.mask,
	}
}

const (
	maskName = 1 << iota
	maskNameContaining
	maskNameRegex
	maskRole
	maskHasClass
	maskAncestor
	maskNth
	maskExpanded
)

// Name sets a specified name to NodeHelper.
func (n *NodeHelper) Name(name string) *NodeHelper {
	c := n.copy()
	c.name = name
	c.mask |= maskName
	return c
}

// NameContaining sets a specified name segment to NodeHelper.
func (n *NodeHelper) NameContaining(s string) *NodeHelper {
	c := n.copy()
	c.nameContaining = s
	c.mask |= maskNameContaining
	return c
}

// NameRegex sets a specified regex name to NodeHelper.
func (n *NodeHelper) NameRegex(nameRegex string) *NodeHelper {
	c := n.copy()
	c.nameRegex = nameRegex
	c.mask |= maskNameRegex
	return c
}

// Role sets a specified role to NodeHelper.
func (n *NodeHelper) Role(role Role) *NodeHelper {
	c := n.copy()
	c.role = role
	c.mask |= maskRole
	return c
}

// HasClass sets a specified class to NodeHelper.
func (n *NodeHelper) HasClass(hasClass string) *NodeHelper {
	c := n.copy()
	c.hasClass = hasClass
	c.mask |= maskHasClass
	return c
}

// Ancestor sets a specified ancestor to NodeHelper.
func (n *NodeHelper) Ancestor(ancestor *Finder) *NodeHelper {
	c := n.copy()
	c.ancestor = ancestor
	c.mask |= maskAncestor
	return c
}

// Nth sets a specified Nth to NodeHelper.
func (n *NodeHelper) Nth(i int32) *NodeHelper {
	c := n.copy()
	c.nth = i
	c.mask |= maskNth
	return c
}

// Expanded sets a specified Expanded state to NodeHelper.
func (n *NodeHelper) Expanded(expanded bool) *NodeHelper {
	c := n.copy()
	c.expanded = expanded
	c.mask |= maskExpanded
	return c
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

	if n.mask&maskNth != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_Nth{Nth: int32(n.nth)}})
	}

	if n.mask&maskExpanded != 0 {
		nodeWiths = append(nodeWiths, &NodeWith{Value: &NodeWith_Expanded{Expanded: n.expanded}})
	}

	return &Finder{NodeWiths: nodeWiths}
}
