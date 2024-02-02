// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesOpenDefaultValues,
		Desc:    "Test the OpenNoDevMode and OpenFromUSB CCD capability default values based on ToT/PrePVT/MP image",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func CCDCapabilitiesOpenDefaultValues(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// Reset and open CCD
	_ = b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)
	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD")
	}

	// Reset CCD so capabilities are set to defaults
	if err := i.CCDReset(ctx); err != nil {
		s.Fatal("Failed to reset CCD: ", err)
	}

	info, err := i.GetVersionInfo(ctx)
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
		if (info.Bid.Flags & 0x10) != 0x10 {
			s.Errorf("Expected 0x10 to be set for ToT BID flags, got 0x%x", info.Bid.Flags)
		}
	case ti50.PrePvt:
		s.Log("Testing PrePVT branch defaults")
		expectedOpenNoDevModeDefault = ti50.CapAlways
		expectedOpenFromUSBDefault = ti50.CapAlways
		if (info.Bid.Flags & 0x10) != 0x10 {
			s.Errorf("Expected 0x10 to be set for PrePVT BID flags, got 0x%x", info.Bid.Flags)
		}
	case ti50.MP:
		s.Log("Testing MP branch defaults")
		expectedOpenNoDevModeDefault = ti50.CapIfOpened
		expectedOpenFromUSBDefault = ti50.CapIfOpened
		if info.Bid.Flags != 0 && (info.Bid.Flags&0x10000 != 0x10000) {
			s.Errorf("Expected 0 or 0x10000 to be set for MP BID flags, got 0x%x", info.Bid.Flags)
		}
	}

	states, err := i.GetCCDCapabilities(ctx)
	if err != nil {
		s.Fatal("Failed to get CCD capabilities")
	}

	openNoDevMode := states[ti50.OpenNoDevMode]
	if openNoDevMode != expectedOpenNoDevModeDefault {
		s.Error("Expected OpenNoDevMode default " + expectedOpenNoDevModeDefault + ", but got " + openNoDevMode)
	}

	openFromUSB := states[ti50.OpenFromUSB]
	if openFromUSB != expectedOpenFromUSBDefault {
		s.Error("Expected OpenFromUSB default " + expectedOpenFromUSBDefault + ", but got " + openFromUSB)
	}
}
