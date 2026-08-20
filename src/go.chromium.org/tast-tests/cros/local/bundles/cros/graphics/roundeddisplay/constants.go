// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package roundeddisplay

const (
	// PromotedOverlayHistogramName is the name of histogram that captures
	// information about promoted overlays/
	PromotedOverlayHistogramName = "Compositing.Display.OverlayProcessorUsingStrategy.NumOverlaysPromotedV2"

	// ExpectedNumberOfPromotedOverlays is how many overlays should be promoted.
	// We expect to promote three overlays planes each frame. (one plane
	// renders low-latency canvas textures and two planes render rounded-display
	// mask textures).
	ExpectedNumberOfPromotedOverlays = 3
)
