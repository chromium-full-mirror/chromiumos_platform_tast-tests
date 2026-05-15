// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type optinTestArgs struct {
	preprod          bool // whether to run against preprod versions of dependencies (default: false)
	fieldTrialConfig int  // Value for FieldTrialConfig Chrome parameter
}

// playTermsURLRedirects is a map used to redirect the Play ToS in the currently used domain (play.google.com/) to the one in the new domain (play.google/) to be tested.
// TODO(b/315504831): Switch to a staging version of the Play ToS once it is readily accessible from the test.
var playTermsURLRedirects = map[string]string{
	"//play.google/play-terms": "//play.google.com/about/play-terms",
}

func init() {
	testing.AddTest(&testing.Test{
		Func: Optin,
		Desc: "A functional test that verifies OptIn flow",
		Contacts: []string{
			// Please assign test failures to current constable on-call
			"arc-constables@google.com",
			"arc-core@google.com",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
			"gaia",
		},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraAttr:         []string{"group:arc-functional", "group:mainline", "group:cq-medium", "group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDefault},
			},
			{
				Name:              "fieldtrial_testing_config_off_vm",
				ExtraAttr:         []string{"group:arc-functional", "group:mainline", "group:cq-medium", "informational"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDisable},
			},
			{
				Name:              "fieldtrial_testing_config_on_vm",
				ExtraAttr:         []string{"group:arc-functional", "group:mainline", "group:cq-medium", "informational", "group:chrome_uprev_cbx"},
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigEnable},
			},
			{
				Name:              "preprod",
				ExtraAttr:         []string{"group:external-dependency", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraSearchFlags: []*testing.StringPair{
					testenv.SearchFlag(testenv.GFEPreprod),
					testenv.SearchFlag(testenv.PlayTermsProd),
				},
				Val: optinTestArgs{preprod: true, fieldTrialConfig: chrome.FieldTrialConfigDefault},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

// Optin tests optin flow.
func Optin(ctx context.Context, s *testing.State) {
	const (
		// If a single variant is flaky, please promote this to test params and increase the
		// attempts only for that specific variant instead of updating the constant for all.
		// See crrev.com/c/2979454 for an example.
		maxAttempts = 1
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	args := s.Param().(optinTestArgs)

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.FieldTrialConfig(args.fieldTrialConfig),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Set up the test environment to test the opt-in against external dependencies.
	if s.Param().(optinTestArgs).preprod {
		cleanup, err := preprodRedirects(ctx, cr)
		if err != nil {
			s.Fatal("Failed to setup preprod test environment: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	s.Log("Performing optin")

	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin: ", err)
	}
}

// preprodRedirects sets up the test environment redirecting to preprod hosts/URLs of external dependencies.
func preprodRedirects(ctx context.Context, cr *chrome.Chrome) (cleanup func(context.Context), retErr error) {
	var cleanups []func(context.Context) error
	cleanup = func(ctx context.Context) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	}
	defer func() {
		if retErr != nil {
			cleanup(ctx)
		}
	}()

	// Redirect all the hosts of *.google.com and *.googleapis.com to the preprod of Google frontend.
	env, err := testenv.NewPreprodEnv(ctx, testenv.RedirectMap(arc.PassThroughPreprodGFE))
	if err != nil {
		return cleanup, errors.Wrap(err, "failed to redirect to preprod GFE")
	}
	cleanups = append(cleanups, env.Close)

	// Redirect to preprod URLs of content or services used in ARC++.
	popts := []proxy.Option{
		proxy.URLRedirect(playTermsURLRedirects), // Redirect to Play ToS page to be tested for ARC++
		proxy.CustomCA(true),                     // Use the system level CA cert
		proxy.DumpFull(true),
		proxy.ForceRestartChromeToUnsetOnClose(),
		proxy.IgnorelistExceptFor([]string{
			`play.google`, // Use proxy for the Play domain only, ignoring all other traffic
		}),
	}
	mp, err := proxy.NewMitmProxy(ctx, popts...)
	if err != nil {
		return cleanup, errors.New("failed to start a proxy for new terms page")
	}
	cleanups = append(cleanups, mp.Close)
	if err := mp.Connect(ctx, cr); err != nil {
		return cleanup, errors.Wrap(err, "failed to configure chrome for proxy")
	}
	return cleanup, nil
}
