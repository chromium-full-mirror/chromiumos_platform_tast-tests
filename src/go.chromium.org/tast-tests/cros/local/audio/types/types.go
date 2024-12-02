// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package types is a package to avoid circular dependencies.
package types

import (
	"fmt"

	"go.chromium.org/tast-tests/cros/common/audio/cras"
	audiopb "go.chromium.org/tast-tests/cros/services/cros/audio"
	"go.chromium.org/tast/core/errors"
)

// StreamType is used to specify the type of node we want to use for tests and
// helper functions.
type StreamType uint

const (
	// InputStream describes nodes with true IsInput attributes.
	InputStream StreamType = 1 << iota
	// OutputStream describes nodes with false IsInput attributes.
	OutputStream
)

func (t StreamType) String() string {
	switch t {
	case InputStream:
		return "InputStream"
	case OutputStream:
		return "OutputStream"
	default:
		return fmt.Sprintf("StreamType(%#x)", t)
	}
}

// CrasNode contains the metadata of Node in Cras.
// Currently fields which are actually needed by tests are defined.
// Please find src/third_party/adhd/cras/README.dbus-api for the meaning of
// each fields.
type CrasNode struct {
	ID         uint64
	Type       string
	Active     bool
	IsInput    bool
	Name       string
	DeviceName string
	NodeVolume uint64
}

// ToProto returns this CrasNode in the proto message format used by cras Tast
// services.
func (n *CrasNode) ToProto() (*audiopb.CrasNode, error) {
	nodeType, err := cras.UnmarshalNodeType(n.Type)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshall node type %q", n.Type)
	}
	return &audiopb.CrasNode{
		Id:         n.ID,
		Type:       nodeType,
		Active:     n.Active,
		IsInput:    n.IsInput,
		DeviceName: n.DeviceName,
		NodeVolume: n.NodeVolume,
	}, nil
}
