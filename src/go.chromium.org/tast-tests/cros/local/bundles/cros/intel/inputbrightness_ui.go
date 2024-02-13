// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/intel/brightness"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputbrightnessUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies system Brightness increase and decrease through UI",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts: []string{
			"intel-chrome-system-automation-team@intel.com",
			"pathan.jilani@intel.com",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:mainline", "informational", "group:intel-nda"},
		Fixture:      "chromePolicyLoggedIn",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ScreenBrightnessPercent{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func InputbrightnessUI(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for _, param := range []struct {
		name           string
		wantBrightness string
		policy         *policy.ScreenBrightnessPercent
	}{
		{
			name:           "brightnessDecreaseCheck",
			wantBrightness: "0%",
			policy: &policy.ScreenBrightnessPercent{
				Val: &policy.ScreenBrightnessPercentValue{
					BrightnessAC:      0,
					BrightnessBattery: 0,
				},
			},
		},
		{
			name:           "brightnessIncreaseCheck",
			wantBrightness: "100%",
			policy: &policy.ScreenBrightnessPercent{
				Val: &policy.ScreenBrightnessPercentValue{
					BrightnessAC:      100,
					BrightnessBattery: 100,
				},
			},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			ui := uiauto.New(tconn)

			// Find the Status tray node and click to open it.
			statusAreaRegex := regexp.MustCompile(ash.StatusAreaClassNameRegex)
			statusTrayToggler := nodewith.ClassNameRegex(statusAreaRegex)
			if err := uiauto.Combine("find and click the status tray",
				ui.LeftClick(statusTrayToggler),
			)(ctx); err != nil {
				s.Fatal("Failed to find and click the status tray: ", err)
			}

			defer func() {
				// Close the Status tray again, otherwise the next subtest won't find it.
				if err := ui.LeftClick(statusTrayToggler)(ctx); err != nil {
					s.Fatal("Failed to click to close Status tray: ", err)
				}
			}()

			// Get the NodeInfo of the Brightness slider.
			brightnessSlider := nodewith.Name("Brightness").Role(role.Slider)
			sliderInfo, err := ui.Info(ctx, brightnessSlider)
			if err != nil {
				s.Fatal("Failed to find Brightness slider: ", err)
			}
			if sliderInfo.Value != param.wantBrightness {
				s.Errorf("Unexpected brightness set: got %s; want %s", sliderInfo.Value, param.wantBrightness)
			}
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				brightness, err := brightness.Percent(ctx)
				if err != nil {
					return testing.PollBreak(errors.Wrap(err, "failed to get system brightness"))
				}
				expBrightness, err := strconv.ParseFloat(strings.Trim(param.wantBrightness, "%"), 64)
				if err != nil {
					return errors.Wrap(err, "failed to convert string to floating value")
				}
				if brightness != expBrightness {
					return errors.Errorf("expected brightness %v but got %v", expBrightness, brightness)
				}
				return nil
			}, &testing.PollOptions{Timeout: 3 * time.Second}); err != nil {
				s.Fatal("Failed to decrease brightness: ", err)
			}
		})
	}
}
