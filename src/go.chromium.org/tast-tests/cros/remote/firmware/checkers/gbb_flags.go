// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package checkers

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/firmware"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
)

// GBBFlags checks that the flags on DUT equals the wanted one.
// You must add `ServiceDeps: []string{"tast.cros.firmware.BiosService"}` to your `testing.Test` to use this.
func (c *Checker) GBBFlags(ctx context.Context, want *pb.GBBFlagsState) error {
	if res, err := firmware.GetGBBFlags(ctx, c.h.DUT); err != nil {
		return errors.Wrap(err, "could not get GBB flags")
	} else if !firmware.GBBFlagsStatesEqual(want, res) {
		return errors.Errorf("GBB flags: got %v, want %v", res.Set, want)
	}
	return nil
}

// GBBFlagsByServo checks that the flags on DUT equals the wanted one using servo connection
// You must add `ServiceDeps: []string{"tast.cros.firmware.BiosService"}` to your `testing.Test` to use this.
func (c *Checker) GBBFlagsByServo(ctx context.Context, want *pb.GBBFlagsState) error {
	if res, err := firmware.GetGBBFlagsByServo(ctx, c.h.ServoProxy); err != nil {
		return errors.Wrap(err, "could not get GBB flags")
	} else if !firmware.GBBFlagsStatesEqual(want, res) {
		return errors.Errorf("GBB flags: got %v, want %v", res.Set, want)
	}
	return nil
}
