// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/util"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cCDCapabilitiesFlashEC struct {
	capState                     servo.CCDCapState
	expectWpEnabledWhenCCDLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CCDCapabilitiesFlashEC,
		Desc: "Test to verify FlashEC CCD capability",
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		// TODO(b:240149552): Reenable this test by adding the proper groups
		// once we have a stable way to verify this CCD capability
		Attr: []string{},
		Contacts: []string{
			"chromeos-faft@google.com",
			"cros-hwsec@google.com",
			"mvertescher@google.com",
		},
		BugComponent: "b:1188704",
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.GSCUART(), hwdep.ChromeEC()),
		SoftwareDeps: []string{"gsc", "reboot"},
		Timeout:      2 * time.Minute,
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesFlashEC{
				capState:                     servo.CapDefault,
				expectWpEnabledWhenCCDLocked: true,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesFlashEC{
				capState:                     servo.CapAlways,
				expectWpEnabledWhenCCDLocked: false,
			},
		}, {
			Name:              "cap_unless_locked",
			ExtraHardwareDeps: hwdep.D(hwdep.HasGSCCr50()),
			Val: cCDCapabilitiesFlashEC{
				capState:                     servo.CapUnlessLocked,
				expectWpEnabledWhenCCDLocked: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesFlashEC{
				capState:                     servo.CapIfOpened,
				expectWpEnabledWhenCCDLocked: true,
			},
		}},
	})
}

func CCDCapabilitiesFlashEC(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	userParams := s.Param().(cCDCapabilitiesFlashEC)

	// Open CCD when finished
	defer func() {
		if err := h.OpenCCD(ctx, true, true); err != nil {
			s.Fatal("Failed to open CCD: ", err)
		}
	}()

	ccdSettings := map[servo.CCDCap]servo.CCDCapState{"FlashAP": servo.CapDefault, "FlashEC": userParams.capState}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set `FlashEC` capability state: ", err)
	}

	if err := verifyFlashromEcWpStatus(ctx, s, false); err != nil {
		s.Fatal("Failed to verify flashrom WP status when CCD is open: ", err)
	}

	if err := h.Servo.LockCCD(ctx); err != nil {
		s.Fatal("Failed to lock CCD: ", err)
	}

	if err := verifyFlashromEcWpStatus(ctx, s, userParams.expectWpEnabledWhenCCDLocked); err != nil {
		s.Fatal("Failed to verify flashrom WP status when CCD is locked: ", err)
	}
}

// verifyFlashromEcWpStatus checks that the EC write protect state reported by
// `flashrom` matches the expectation `expectWpEnabled`.
func verifyFlashromEcWpStatus(ctx context.Context, s *testing.State, expectWpEnabled bool) error {
	isWpEnabled, err := util.FlashromWpStatus(ctx, s, "EC")
	if err != nil {
		return errors.Wrap(err, "failed to get WP flashrom status")
	}

	if expectWpEnabled != isWpEnabled {
		return errors.New("Expected EC WP to be enabled = " + strconv.FormatBool(expectWpEnabled) + ", but flashrom reported enabled = " + strconv.FormatBool(isWpEnabled))
	}

	return nil
}
