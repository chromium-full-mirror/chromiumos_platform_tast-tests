// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"math"
)

// transformToNormalizedCoordinates transforms the points so that the start point is at the origin, and the endpoint is at x > 0, y = ~0.
// The y value of all other points will correspond to the perpendicular distance from a straight line between the start and endpoint.
func transformToNormalizedCoordinates(points []*hmrNode) []*hmrNode {
	offsetPoints := applyOffset(points, -points[0].coorX, -points[0].coorY)
	angle := getAngle(offsetPoints)
	rotatedPoints := applyRotation(offsetPoints, angle)
	return rotatedPoints
}

// applyOffset applies an offset to all coorX and coorY coordinates.
func applyOffset(points []*hmrNode, x, y float64) []*hmrNode {
	for i := range points {
		points[i].coorX += x
		points[i].coorY += y
	}
	return points
}

// applyRotation rotates all points around the start point. Angle provided is in radians. Positive direction is counter clockwise.
func applyRotation(points []*hmrNode, angle float64) []*hmrNode {
	for i := range points {
		var tempX, tempY float64
		tempX = points[i].coorX
		tempY = points[i].coorY
		points[i].coorX = math.Cos(angle)*tempX + -math.Sin(angle)*tempY
		points[i].coorY = math.Sin(angle)*tempX + math.Cos(angle)*tempY
	}
	return points
}

// applyScale scales the coorX and coorY of all points by x and y.
func applyScale(points []*hmrNode, x, y float64) []*hmrNode {
	for i := range points {
		points[i].coorX *= x
		points[i].coorY *= y
	}
	return points
}

// getAngle returns the rotation angle needed to align the origin point and end point at ~y = 0 and endpoint x > 0.
// Angle is in radians. Positive direction is counter clockwise.
func getAngle(points []*hmrNode) float64 {
	xEndPoint := points[len(points)-1].coorX - points[0].coorX
	yEndPoint := points[len(points)-1].coorY - points[0].coorY
	tanAngle := math.Atan2(yEndPoint, xEndPoint)
	if tanAngle < 0 {
		return math.Abs(tanAngle)
	}
	return 2*math.Pi - tanAngle
}
