// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/displayvalidation"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DisplayValidation,
		Desc: "Validate the display against known CrOS internal displays",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
		Timeout:      4 * time.Minute,
	})
}

func DisplayValidation(ctx context.Context, s *testing.State) {
	var edidErr, healthErr, combinedErr error
	var display displayvalidation.DisplayInfo

	display, edidErr = displayvalidation.ValidateEdidDisplayInfo(ctx)
	if (edidErr != nil) && (display == displayvalidation.DisplayInfo{}) {
		s.Logf("Failed validate display via edid: %v; using healthd telemetry to get display info", edidErr)
		display, healthErr = displayvalidation.ValidateCrOSHealthDisplayInfo(ctx)
	}

	if (healthErr != nil) && (display == displayvalidation.DisplayInfo{}) {
		combinedErr = errors.Join(edidErr, healthErr)
		s.Fatal("Failed to get display information: ", combinedErr)
	}

	if (combinedErr != nil) && (display != displayvalidation.DisplayInfo{}) {
		s.Fatalf("Failed to match DUT display with Size: %f Resolution: %dX%d: %v", display.DisplayDiagonalSize, display.Resolution.Width, display.Resolution.Height, combinedErr)
	}

	// Record the display if we are able to fetch the information.
	s.Logf("Display validated; Display size %f Resolution %dx%d", display.DisplayDiagonalSize, display.Resolution.Width, display.Resolution.Height)
	displayString, err := json.Marshal(display)
	if err != nil {
		s.Log("Failed to construct display config string: ", err)
		return
	}

	err = os.WriteFile(filepath.Join(s.OutDir(), "display.json"), displayString, 0644)
	if err != nil {
		s.Log("Failed to write display file: ", err)
	}
}
