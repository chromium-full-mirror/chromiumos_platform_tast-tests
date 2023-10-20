// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPChromeLogin,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when device is in idle with Chrome login",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      1*time.Minute + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: pnp.StablePowerAshGAIA,
		}, {
			Name:    "lacros",
			Fixture: pnp.StablePowerLacrosGAIA,
		}},
	})
}

func pnpChromeLoginWorkload(ctx context.Context, browserType browser.Type, cr *chrome.Chrome) ([]action.Action, error) {

	var cleanupFuncs []action.Action

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, "about:blank")
	if err != nil {
		return cleanupFuncs, errors.Wrap(err, "failed to open a blank new tab")
	}
	cleanupFuncs = append(cleanupFuncs, cleanup)
	cleanupFuncs = append(cleanupFuncs, func(ctx context.Context) error {
		return conn.Close()
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return cleanupFuncs, errors.Wrap(err, "failed to get test API connection")
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(browserType))
	if err != nil {
		return cleanupFuncs, errors.Wrap(err, "failed to open a browser window")
	}

	return cleanupFuncs, ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
}

func PNPChromeLogin(ctx context.Context, s *testing.State) {
	browserType := s.FixtValue().(powersetup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr
	if err := pnp.PowerEstimationRoutine(ctx, pnpChromeLoginWorkload, browserType, cr, s.OutDir(), s.TestName()); err != nil {
		s.Fatal("Failed to estimate power: ", err)
	}
}
