// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/serial"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DefaultSerialGuardSetting,
		Desc: "Tests the behavior of the DefaultSerialGuardSetting policy by checking that it correctly configures access to the serial port selection prompt",
		Contacts: []string{
			"cros-engprod-muc@google.com",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Data:         []string{serial.SerialTestPage},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DefaultSerialGuardSetting{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// DefaultSerialGuardSetting tests the DefaultSerialGuardSetting policy.
func DefaultSerialGuardSetting(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	httpServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer httpServer.Close()

	for _, param := range []struct {
		name             string
		wantSerialDialog bool
		policy           *policy.DefaultSerialGuardSetting
	}{
		{
			name:             "blocked",
			wantSerialDialog: false,
			policy:           &policy.DefaultSerialGuardSetting{Val: serial.DefaultSerialGuardSettingBlock},
		},
		{
			name:             "non_blocked",
			wantSerialDialog: true,
			policy:           &policy.DefaultSerialGuardSetting{Val: serial.DefaultSerialGuardSettingAsk},
		},
		{
			name:             "unset",
			wantSerialDialog: true,
			policy:           &policy.DefaultSerialGuardSetting{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Reserve ten seconds for cleanup.
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := serial.TestSerialPortRequest(ctx, cr, httpServer.URL, param.wantSerialDialog); err != nil {
				s.Fatal("Failed while testing serial port request: ", err)
			}
		})
	}
}
