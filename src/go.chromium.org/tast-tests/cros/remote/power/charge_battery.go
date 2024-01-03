// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ChargeBattery uses LocalInfoService to create a gRPC call to the DUT.
// It returns the power supply information reported by powerd's dump_power_status
// tool.
func ChargeBattery(ctx context.Context, chargeTarget float64, client pb.LocalInfoServiceClient) error {
	testing.ContextLogf(ctx, "Waiting for battery to reach %f", chargeTarget)
	return testing.Poll(ctx, func(ctx context.Context) error {
		status, err := client.GetPowerStatus(ctx, &empty.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to call gRPC service for power status")
		}

		percent := status.GetBatteryPercent()
		if percent < chargeTarget {
			testing.ContextLogf(ctx, "Charging, percentage is %f percent", percent)
			return errors.Errorf("display percentage %f, want >%f", percent, chargeTarget)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Minute, Interval: 5 * time.Second})
}
