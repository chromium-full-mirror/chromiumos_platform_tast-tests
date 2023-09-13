// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayZoomChange,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verfifies display zoom to small and large",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:mainline", "informational", "group:intel-nda"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
	})
}

// DisplayZoomChange set display zoom to smaller and larger from the available display zoom factors.
func DisplayZoomChange(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	info, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get internal display info: ", err)
	}
	// Set to default display zoom.
	defer changeDisplayZoom(ctx, tconn, info.ID, info.DisplayZoomFactor)

	// Checking minimum 2 display zoom factors to set small and large.
	length := len(info.AvailableDisplayZoomFactors)
	if length < 2 {
		s.Fatalf("Avaialble display zoom factors found %d; want array with at least 2 values", length)
	}

	displayZoomTiny := info.AvailableDisplayZoomFactors[0]
	if err := changeDisplayZoom(ctx, tconn, info.ID, displayZoomTiny); err != nil {
		s.Fatalf("Failed to set display zoom to %f: %v", displayZoomTiny, err)
	}

	displayZoomHuge := info.AvailableDisplayZoomFactors[length-1]
	if err := changeDisplayZoom(ctx, tconn, info.ID, displayZoomHuge); err != nil {
		s.Fatalf("Failed to set display zoom to %f: %v", displayZoomHuge, err)
	}
}

// changeDisplayZoom changes the ChromeOS display zoom.
func changeDisplayZoom(ctx context.Context, tconn *chrome.TestConn, dispID string, zoomFactor float64) error {
	p := display.DisplayProperties{DisplayZoomFactor: &zoomFactor}
	if err := display.SetDisplayProperties(ctx, tconn, dispID, p); err != nil {
		return errors.Wrap(err, "failed to set zoom factor")
	}

	info, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get internal display info")
	}

	if info.DisplayZoomFactor != zoomFactor {
		return errors.Errorf("failed to change display zoom to %f", zoomFactor)
	}
	return nil
}
