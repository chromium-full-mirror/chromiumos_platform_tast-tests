// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/utc"
	"go.chromium.org/tast-tests/cros/remote/typec/typecutc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DataSwapStability,
		Desc: "Check data swap stability on a typec port",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		VarDeps:      []string{"servo"},
		Fixture:      "typecUtc",
		Contacts:     []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		Attr:         []string{"group:typec", "typec_informational"},
		// This will ever only be run on one port.
		// It requires servo to be connected, so only utc port 0 will be used.
		Params: []testing.Param{
			{
				Name:      "port0_normal",
				ExtraAttr: []string{"typec_utc274"},
				Timeout:   10 * time.Minute,
			},
		},
	})
}

func DataSwapStability(ctx context.Context, s *testing.State) {
	d := s.DUT()
	numIterations := 30
	dutTestPortID := 1
	utcTestPortID := 0

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Get utc controller from fixture.
	fixtData, ok := s.FixtValue().(*typecutc.FixtureData)
	if !ok {
		s.Fatal("Failed to get utc controller from fixture")
	}
	utcctl := fixtData.Utc

	// TODO(b/416456393) Turn off active port on utc
	if err := utcctl.SetTestPort(ctx, 1); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}

	// Prepare servo.
	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
	if err != nil {
		s.Fatal("Failed to setup servo proxy: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Get servo PD info
	if err := pxy.Servo().RequireDUTPDInfo(ctx); err != nil {
		s.Fatal("Failed to get DUT PD info for servo: ", err)
	}

	// Turn on active port on utc.
	if err := utcctl.SetTestPort(ctx, utcTestPortID); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}
	s.Logf("utc testing port was set to port %d", utcTestPortID)

	// Stress data swap.
	for i := 0; i < numIterations; i++ {
		prevRole, err := utcctl.DataRole(ctx)
		if err != nil {
			s.Fatal("Failed to get data role: ", err)
		}

		nextRole := utc.DataRoleUfp
		if prevRole == utc.DataRoleUfp {
			nextRole = utc.DataRoleDfp
		}
		s.Logf("utc data role is %s, request switch", prevRole.String())

		// Issue data swap from the EC, otherwise the swap will be rejected by the DUT in DFP role.
		if err := pxy.Servo().SendDataSwapRequestToPort(ctx, dutTestPortID); err != nil {
			s.Fatal("Failed to issue data swap: ", err)
		}

		// Verify new state on the dut.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return typecutils.CheckDataRole(ctx, d, prevRole.String(), dutTestPortID)
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed check of data role on DUT: ", err)
		}

		// Verify new state on utc.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if utcRole, err := utcctl.DataRole(ctx); err != nil {
				return errors.Wrap(err, "failed to get data role")
			} else if utcRole != nextRole {
				return errors.Errorf("Data role on utc is not %s but %s", nextRole, utcRole)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed check of data role on utc: ", err)
		}

		s.Logf("Iteration (%d/%d) OK", i+1, numIterations)
	}
}
