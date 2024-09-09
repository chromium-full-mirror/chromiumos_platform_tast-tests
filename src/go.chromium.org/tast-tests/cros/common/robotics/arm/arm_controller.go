// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arm defines the interface to communicate with the robotic arm.
package arm

import (
	"context"
	"time"
)

// Controller interface defines the common function to control a robotic arm.
type Controller interface {
	Close() error
	MoveToInitialPosition(context.Context) error
	// SingleMove moves the robotic arm to the specific position.
	// |pos| is a set of numbers representing the position of the robotic arm.
	// |duration| is the duration of the movement.
	SingleMove(ctx context.Context, pos []float32, duration time.Duration) error
	// MultiMove moves the robotic arm to the specific positions in series.
	// |multiPos| contains multiple sets of numbers representing the continuous positions.
	// multiPos[0] is the first position, multiPos[1] is the second position ...
	// |duration| is the duration of each movement. The overall time consumption would be
	// |duration| * len(multiPos).
	MultiMove(ctx context.Context, multiPos [][]float32, duration time.Duration) error
}
