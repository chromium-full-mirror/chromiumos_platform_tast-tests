// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package phonehub

import (
	"context"
	"time"

	cdcommon "go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/phonehub"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhoneStatus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Phone Hub displays the phone's battery and signal levels",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"crisrael@google.com",
		},
		BugComponent: "b:1131837",
		Attr:         []string{"group:cross-device", "cross-device_phonehub"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraAttr:         []string{"cross-device_cq"},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(cdcommon.UnstableModels...)),
			},
			{
				Name:              "unstable",
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cdcommon.UnstableModels...)),
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFlossRerun",
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
	})
}

// PhoneStatus tests that Phone Hub accurately displays the phone's battery and signal levels.
func PhoneStatus(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice
	if err := phonehub.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Phone Hub: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Phones in the lab don't have a SIM.
	ui := uiauto.New(tconn)
	if err := ui.Exists(nodewith.Role(role.Image).NameContaining("No SIM"))(ctx); err != nil {
		s.Fatal("Failed to check SIM status: ", err)
	}

	// Get the battery level from the UI.
	uiLevel, err := phonehub.BatteryLevel(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get battery level from the UI: ", err)
	}

	// Get the phone's battery level to compare.
	phoneLevel, err := androidDevice.BatteryLevel(ctx)
	if err != nil {
		s.Fatal("Failed to get battery level from adb: ", err)
	}

	if uiLevel != phoneLevel {
		s.Fatalf("Phone Hub battery level (%v) does not match level reported by ADB (%v)", uiLevel, phoneLevel)
	}
}
