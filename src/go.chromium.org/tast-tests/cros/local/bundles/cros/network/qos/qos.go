// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package qos contains utilities for qos related tests.
package qos

import (
	"go.chromium.org/tast-tests/cros/local/network/capture"
)

const (
	// RealtimeInteractiveDSCP is DSCP for realtime interactive.
	RealtimeInteractiveDSCP = 32
	// MediaConferencingDSCP is DSCP for media conferencing.
	MediaConferencingDSCP = 34
	// DefaultDSCP is DSCP for default behavior without prioritization.
	DefaultDSCP = 0
)

// HasDSCP checks a packet has expected DSCP.
func HasDSCP(p *capture.Packet, DSCP uint8) bool {
	return p.DSCP() == DSCP
}
