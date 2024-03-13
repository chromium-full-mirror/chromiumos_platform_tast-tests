// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

// applyScale scales the coorX and coorY of all points by x and y.
func applyScale(points []*hmrNode, x, y float64) []*hmrNode {
	for i := range points {
		points[i].coorX *= x
		points[i].coorY *= y
	}
	return points
}
