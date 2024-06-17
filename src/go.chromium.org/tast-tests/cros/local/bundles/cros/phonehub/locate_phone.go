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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocatePhone,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that toggling Phone Hub's \"Locate phone\" pod will toggle the ringer on the Android phone",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"jonmann@chromium.org",
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

// LocatePhone tests toggling the connected Android phone's "Locate phone" feature using Phone Hub.
func LocatePhone(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := phonehub.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Phone Hub: ", err)
	}

	// Time to wait for "Locate phone" status changes to take effect across devices.
	locateTimeout := 5 * time.Second

	// Disable the pod if it's already active.
	active, err := phonehub.LocatePhoneEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to check initial 'Locate phone' status: ", err)
	}
	if active {
		if err := phonehub.ToggleLocatePhonePod(ctx, tconn, false /*enable*/); err != nil {
			s.Fatal("Failed to turn off 'Locate phone': ", err)
		}

		if err := androidDevice.WaitForFindMyPhone(ctx, false /*active*/, locateTimeout); err != nil {
			s.Fatal("Failed waiting for 'Find my phone' alarm to be inactive on the phone: ", err)
		}
	}

	if err := phonehub.ToggleLocatePhonePod(ctx, tconn, true /*enable*/); err != nil {
		s.Fatal("Failed to turn on 'Locate phone': ", err)
	}
	// Defer toggling the Android screen off and back on to silence the ringer in case of failure.
	defer androidDevice.ToggleScreen(cleanupCtx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := androidDevice.WaitForFindMyPhone(ctx, true /*active*/, locateTimeout); err != nil {
		s.Fatal("Failed waiting for 'Find my phone' alarm to be active on the phone: ", err)
	}

	if err := phonehub.ToggleLocatePhonePod(ctx, tconn, false /*enable*/); err != nil {
		s.Fatal("Failed to turn off 'Locate phone': ", err)
	}

	if err := androidDevice.WaitForFindMyPhone(ctx, false /*active*/, locateTimeout); err != nil {
		s.Fatal("Failed waiting for 'Find my phone' alarm to be inactive on the phone: ", err)
	}
}
