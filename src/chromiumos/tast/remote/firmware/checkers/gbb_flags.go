// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package checkers

import (
	"context"

	"chromiumos/tast/common/firmware"
	"chromiumos/tast/errors"
	pb "chromiumos/tast/services/cros/firmware"
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
