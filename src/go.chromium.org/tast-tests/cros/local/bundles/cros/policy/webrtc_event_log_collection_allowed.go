// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/webrtclogupload"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebrtcEventLogCollectionAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test network annotation by turning on and off WebRTCEventLogCollectionAllowed policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"sugandhagoyal@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.bond_credentials"},
		Timeout:      15 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebRtcEventLogCollectionAllowed{}, pci.VerifiedValue),
		},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyRealUserLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosPolicyRealUserLoggedIn,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func WebrtcEventLogCollectionAllowed(ctx context.Context, s *testing.State) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	bondCreds := s.RequiredVar("ui.bond_credentials")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	for _, param := range []webrtclogupload.TestCase{
		{
			Name:                  "unset",
			AnnotationLogExpected: true,
			Policies: []policy.Policy{
				&policy.WebRtcEventLogCollectionAllowed{Stat: policy.StatusUnset},
			},
		}, {
			Name:                  "enabled",
			AnnotationLogExpected: true,
			Policies: []policy.Policy{
				&policy.WebRtcEventLogCollectionAllowed{Val: true},
			},
		},
		{
			Name:                  "disabled",
			AnnotationLogExpected: false,
			Policies: []policy.Policy{
				&policy.WebRtcEventLogCollectionAllowed{Val: false},
			},
		},
	} {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			runParam := webrtclogupload.AnnotationTestParam{
				AnnotationHashCode: webrtclogupload.EventLogCollectionHashID,
				Bt:                 s.Param().(browser.Type),
				Creds:              bondCreds,
				Tc:                 param,
				Timeout:            2 * time.Minute,
				Interval:           1 * time.Millisecond,
			}

			if err := webrtclogupload.NetLogAnnotationTest(ctx, fdms, cr, tconn, runParam); err != nil {
				s.Fatal("Error encountered while running test: ", err)
			}

		})
	}
}
