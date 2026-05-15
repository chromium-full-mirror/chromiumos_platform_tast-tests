// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type oobeArcArgs struct {
	preprod bool // whether to run against preprod versions of dependencies (default: false)
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     OobeArc,
		Desc:     "Navigate through OOBE and Verify that PlayStore Settings Screen is launched at the end",
		Contacts: []string{"cros-arc-te@google.com", "cros-oac@google.com", "jinrongwu@google.com", "mattlui@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:arc", "arc_core", "group:arc-functional", "group:hw_agnostic"},
				Val:               oobeArcArgs{preprod: false},
			}, {
				Name:              "preprod",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:external-dependency", "group:hw_agnostic"},
				ExtraSearchFlags:  []*testing.StringPair{testenv.SearchFlag(testenv.GFEPreprod)},
				Val:               oobeArcArgs{preprod: true},
			}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 10*time.Minute,
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

func OobeArc(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	// Set up the test environment to run tests against the preprod of GAIA.
	if s.Param().(oobeArcArgs).preprod {
		env, err := testenv.NewPreprodEnv(ctx,
			testenv.RedirectMap(map[string]string{
				"google-prod":     "gfe-preprod", // GFE
				"googleapis-prod": "gfe-preprod", // GFE
			}),
			testenv.PortalDetection(false), // Disable portal detection that may conflict with a mid DNS server during network validation.
		)
		if err != nil {
			s.Fatal("Failed to init the preprod env: ", err)
		}
		defer env.Close(cleanupCtx)
	}

	cr, err := chrome.New(ctx,
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ARCSupported(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		// TODO(b/287862720): Update the test to go through CHOOBE flow and stop disabling the features.
		chrome.ExtraArgs("--disable-features=OobeChoobe,OobeDisplaySize,OobeTouchpadScrollDirection"))

	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	ui := uiauto.New(tconn)

	if err := oobe.CompleteOnboardingFlow(ctx, ui); err != nil {
		s.Fatal("Failed to go through the oobe flow: ", err)
	}

	s.Log("Verify Play Store is On")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		playStoreState, err := optin.GetPlayStoreState(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get some playstore state")
		}
		if !playStoreState["enabled"] {
			return errors.New("playstore is off")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify Play Store is On: ", err)
	}
}
