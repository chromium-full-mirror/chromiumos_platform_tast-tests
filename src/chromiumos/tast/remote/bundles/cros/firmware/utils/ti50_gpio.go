// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains functionality shared by tests that
// exercise firmware restoration.
package utils

import (
	"context"

	"chromiumos/tast/common/firmware/ti50"
	"chromiumos/tast/testing"
)

// MustSetGpio sets a well-defined gpio to a value, and if there are any errors, set a fatal
// condition on the test state
func MustSetGpio(ctx context.Context, board ti50.DevBoard, s *testing.State, g ti50.GpioName, val bool) {
	if err := board.GpioWrite(ctx, g, val); err != nil {
		s.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

// MustGetGpio gets a well-defined gpio value, and if there are any errors, set a fatal
// condition on the test state
func MustGetGpio(ctx context.Context, board ti50.DevBoard, s *testing.State, g ti50.GpioName) bool {
	val, err := board.GpioRead(ctx, g)
	if err != nil {
		s.Fatalf("Failed to get gpio %s: %s", g, err)
	}
	return val
}
