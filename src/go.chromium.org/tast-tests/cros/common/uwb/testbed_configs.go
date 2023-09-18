// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"math"

	"go.chromium.org/tast/core/errors"
)

// Device enum
type Device int

// Types of UWB Devices supported in lab
const (
	CrOS Device = iota
	Android
)

// Position contains the variables that define a configuration position
type Position struct {
	Normal float64 // degrees
	X      int     // cm
	Y      int
	Z      int
	Device Device
}

// AllConfigs is a map of the existing UWB Testbed configurations
// Key is the name of the config, array index indicates the position number
// e.g. Config A Position 1 = AllConfigsMap()["A"][1]
var AllConfigs = map[string][]Position{
	"A": {
		{Normal: 0, X: 0, Y: 0, Z: 0, Device: CrOS},
		{Normal: -135, X: -50, Y: 0, Z: 50, Device: CrOS},
		{Normal: 135, X: 50, Y: 0, Z: 50, Device: Android},
	},
}

// Distance gives the euclidean distance between the 2 positions
func Distance(p1, p2 *Position) uint32 {
	x := math.Pow(float64(p1.X-p2.X), 2)
	y := math.Pow(float64(p1.Y-p2.Y), 2)
	z := math.Pow(float64(p1.Z-p2.Z), 2)

	distance := math.Sqrt(x + y + z)
	return uint32(math.Round(distance))
}

// AzimuthAngle gives the expected azimuth reading that p1 receives of p2
func AzimuthAngle(p1, p2 *Position) float64 {
	xdiff := p2.X - p1.X
	zdiff := p2.Z - p1.Z
	angle := math.Atan2(-float64(xdiff), float64(zdiff))

	final := math.Round((180.0/math.Pi)*angle) - p1.Normal
	if final < -180 {
		return final + 360
	}
	if final > 180 {
		return final - 360
	}
	return -final
}

// ElevationAngle gives the expected Elevation reading that p1 receives of p2
func ElevationAngle(p1, p2 *Position) float64 {
	ydiff := p2.Y - p1.Y
	dist := Distance(p1, p2)
	angle := math.Atan2(float64(ydiff), float64(dist))
	return math.Round((180.0 / math.Pi) * angle)
}

// GetBoundsFromPosition returns an ExpectedRangingBounds to be used in GetRangingSummary based on
// 2 given Positions, an acceptable +- in distance and angle readings and a minimum FOM value
func GetBoundsFromPosition(p1, p2 *Position, distanceError uint32, angleError float64, minFOM uint32) ExpectedRangingBounds {
	distance := Distance(p1, p2)
	azimuth := AzimuthAngle(p1, p2)
	elevation := ElevationAngle(p1, p2)

	return ExpectedRangingBounds{
		DistanceUpperBound:  distance + distanceError,
		DistanceLowerBound:  distance - distanceError,
		AzimuthUpperBound:   azimuth + angleError,
		AzimuthLowerBound:   azimuth - angleError,
		ElevationUpperBound: elevation + angleError,
		ElevationLowerBound: elevation - angleError,
		MinimumFOM:          minFOM,
	}
}

// GetBoundsFromNames calls GetBoundsPositions with the positions of the given device names
func GetBoundsFromNames(n1, n2 string, distanceError uint32, angleError float64, minFOM uint32) (ExpectedRangingBounds, error) {
	positions := DeviceNamesToPosition()
	var p1, p2 Position

	p1, exists := positions[n1]
	if !exists {
		return ExpectedRangingBounds{}, errors.Errorf("%s not found in DeviceNamesToPosition map", n1)
	}

	p2, exists = positions[n2]
	if !exists {
		return ExpectedRangingBounds{}, errors.Errorf("%s not found in DeviceNamesToPosition map", n2)
	}
	return GetBoundsFromPosition(&p1, &p2, distanceError, angleError, minFOM), nil
}

// DeviceNamesToPosition returns the map mapping the unique lab DUT names to their configuration positions
func DeviceNamesToPosition() map[string]Position {
	return map[string]Position{
		"chromeos1-sinclair-unit1-host1a":  AllConfigs["A"][0],
		"chromeos1-sinclair-unit1-host1b":  AllConfigs["A"][1],
		"chromeos1-sinclair-unit1-phone1a": AllConfigs["A"][2],
	}
}
