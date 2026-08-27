// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"

	"go.chromium.org/tast/core/testing"
)

const (
	toTBIDFlags    = 0x20000
	prePVTBIDFlags = 0x10
	mPBIDFlags     = 0x10000
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesOpenDefaultValues,
		Desc:    "Test the OpenNoDevMode and OpenFromUSB CCD capability default values based on ToT/PrePVT/MP image",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly",
			"gsc_smoke"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func CCDCapabilitiesOpenDefaultValues(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// Reset and open CCD
	_ = b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	b.WaitUntilCCDConnected(ctx)
	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD")
	}

	// Reset CCD so capabilities are set to defaults
	if err := i.CCDReset(ctx); err != nil {
		s.Fatal("Failed to reset CCD: ", err)
	}

	info, err := i.VersionInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get version information: ", err)
	}

	var expectedOpenNoDevModeDefault ti50.CCDCapState
	var expectedOpenFromUSBDefault ti50.CCDCapState
	switch info.Build.Branch {
	case ti50.ToT:
		s.Log("Testing ToT branch defaults")
		expectedOpenNoDevModeDefault = ti50.CapAlways
		expectedOpenFromUSBDefault = ti50.CapAlways
		if (info.BID.Flags & toTBIDFlags) != toTBIDFlags {
			s.Errorf("Expected 0x%x to be set for ToT BID flags, got 0x%x", toTBIDFlags, info.BID.Flags)
		}
	case ti50.PrePvt:
		s.Log("Testing PrePVT branch defaults")
		expectedOpenNoDevModeDefault = ti50.CapAlways
		expectedOpenFromUSBDefault = ti50.CapAlways
		if (info.BID.Flags & prePVTBIDFlags) != prePVTBIDFlags {
			s.Errorf("Expected 0x%x to be set for PrePVT BID flags, got 0x%x", prePVTBIDFlags, info.BID.Flags)
		}
	case ti50.MP:
		s.Log("Testing MP branch defaults")
		expectedOpenNoDevModeDefault = ti50.CapIfOpened
		expectedOpenFromUSBDefault = ti50.CapIfOpened
		if info.BID.Flags != 0 && (info.BID.Flags&mPBIDFlags != mPBIDFlags) {
			s.Errorf("Expected 0 or 0x%x to be set for MP BID flags, got 0x%x", mPBIDFlags, info.BID.Flags)
		}
	}

	caps, err := i.CCDCapabilities(ctx)
	if err != nil {
		s.Fatal("Failed to get CCD capabilities")
	}

	openNoDevMode := caps.States[ti50.OpenNoDevMode]
	if openNoDevMode.State != expectedOpenNoDevModeDefault || !openNoDevMode.IsDefault {
		s.Errorf("Expected OpenNoDevMode default %s but got %+v ", expectedOpenNoDevModeDefault, openNoDevMode)
	}

	openFromUSB := caps.States[ti50.OpenFromUSB]
	if openFromUSB.State != expectedOpenFromUSBDefault || !openFromUSB.IsDefault {
		s.Errorf("Expected OpenFromUSB default %s but got %+v ", expectedOpenFromUSBDefault, openFromUSB)
	}
}
