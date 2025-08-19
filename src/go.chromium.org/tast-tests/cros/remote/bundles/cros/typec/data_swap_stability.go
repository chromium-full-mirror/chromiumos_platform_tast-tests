// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/typecunigraf"
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
		Fixture:      "typecUnigraf",
		Contacts:     []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		Attr:         []string{"group:typec", "typec_informational"},
		// This will ever only be run on one port.
		// It requires servo to be connected, so only Unigraf port 0 will be used.
		Params: []testing.Param{
			{
				Name:      "port0_normal",
				ExtraAttr: []string{"typec_unigraf274"},
				Timeout:   10 * time.Minute,
			},
		},
	})
}

func DataSwapStability(ctx context.Context, s *testing.State) {
	d := s.DUT()
	numIterations := 30
	dutTestPortID := 1
	unigrafTestPortID := 0

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Get Unigraf controller from fixture.
	fixtData, ok := s.FixtValue().(*typecunigraf.FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf controller from fixture")
	}
	unigrafctl := fixtData.Unigraf

	// TODO(b/416456393) Turn off active port on Unigraf
	if err := unigrafctl.SetTestPort(ctx, 1); err != nil {
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

	// Turn on active port on Unigraf.
	if err := unigrafctl.SetTestPort(ctx, unigrafTestPortID); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}
	s.Logf("Unigraf testing port was set to port %d", unigrafTestPortID)

	// Stress data swap.
	for i := 0; i < numIterations; i++ {
		prevRole, err := unigrafctl.DataRole(ctx)
		if err != nil {
			s.Fatal("Failed to get data role: ", err)
		}

		nextRole := unigraf.DataRoleUfp
		if prevRole == unigraf.DataRoleUfp {
			nextRole = unigraf.DataRoleDfp
		}
		s.Logf("Unigraf data role is %s, request switch", prevRole.String())

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

		// Verify new state on Unigraf.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if unigrafRole, err := unigrafctl.DataRole(ctx); err != nil {
				return errors.Wrap(err, "failed to get data role")
			} else if unigrafRole != nextRole {
				return errors.Errorf("Data role on Unigraf is not %s but %s", nextRole, unigrafRole)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed check of data role on Unigraf: ", err)
		}

		s.Logf("Iteration (%d/%d) OK", i+1, numIterations)
	}
}
