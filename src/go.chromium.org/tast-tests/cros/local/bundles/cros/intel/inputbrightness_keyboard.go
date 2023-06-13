// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/intel/brightness"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputbrightnessKeyboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies system Brightness increase and decrease through onboard keyboard",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts: []string{
			"intel.chrome.automation.team@intel.com",
			"pathan.jilani@intel.com",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:mainline", "informational", "group:intel-nda"},
		Pre:          chrome.LoggedIn(),
	})
}

func InputbrightnessKeyboard(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	initialBrightness, err := brightness.Percent(ctx)
	if err != nil {
		s.Fatal("Failed to get initial brightness: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Resetting brightness")
		if err := brightness.SetPercent(ctx, initialBrightness); err != nil {
			s.Fatal("Failed to restore DUTs initial brightness: ", err)
		}
	}(cleanupCtx)

	if err := brightness.SetPercent(ctx, 100); err != nil {
		s.Fatal("Failed to set brightness to maximmum: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard object: ", err)
	}
	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		s.Fatal("Failed to obtain the top-row layout: ", err)
	}
	// Decreasing brightness level with on-board keyboard key press.
	for {
		preBrightness, err := brightness.Percent(ctx)
		if err != nil {
			s.Fatal("Failed to get brightness: ", err)
		}
		decBrightness, err := waitForChangeInBrightness(ctx, func() error {
			return kb.Accel(ctx, topRow.BrightnessDown)
		})
		if err != nil {
			s.Fatal(`Failed to change brightness after pressing "BrightnessDown": `, err)
		}
		if decBrightness >= preBrightness {
			s.Fatal("Failed to decrease the brightness")
		}
		if decBrightness == 0.0 {
			break
		}
	}
	// Increasing brightness level with on-board keyboard key press.
	for {
		preBrightness, err := brightness.Percent(ctx)
		if err != nil {
			s.Fatal("Failed to get brightness: ", err)
		}
		incBrightness, err := waitForChangeInBrightness(ctx, func() error {
			return kb.Accel(ctx, topRow.BrightnessUp)
		})
		if err != nil {
			s.Fatal(`Failed to change brightness after pressing "BrightnessUp": `, err)
		}
		if incBrightness <= preBrightness {
			s.Fatal("Failed to increase the brightness")
		}
		if incBrightness == 100.0 {
			break
		}
	}

}

// waitForChangeInBrightness waits for change in brightness value while calling doBrightnessChange function.
// doBrightnessChange does brightness value change with keyboard BrightnessUp/BrightnessDown keypress.
func waitForChangeInBrightness(ctx context.Context, doBrightnessChange func() error) (float64, error) {
	var curBrightness float64
	preBrightness, err := brightness.Percent(ctx)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to get system brightness")
	}
	if err := doBrightnessChange(); err != nil {
		return 0.0, errors.Wrap(err, "failed in calling doBrightnessChange function")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		curBrightness, err = brightness.Percent(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get brightness after doBrightnessChange function is called"))
		}
		if preBrightness == curBrightness {
			return errors.New("brightness not changed")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return 0.0, errors.Wrap(err, "failed to wait for brightness change")
	}
	return curBrightness, nil
}
