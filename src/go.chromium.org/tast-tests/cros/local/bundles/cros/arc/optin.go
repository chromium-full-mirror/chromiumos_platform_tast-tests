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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type optinTestArgs struct {
	preprod          bool // whether to run against preprod versions of dependencies (default: false)
	fieldTrialConfig int  // Value for FieldTrialConfig Chrome parameter
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Optin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test that verifies OptIn flow",
		Contacts: []string{
			// Please assign test failures to current constable on-call
			"arc-constables@google.com",
			"arc-core@google.com",
			"mhasank@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:cq-medium"},
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
			"gaia",
		},
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_container"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDefault},
			},
			{
				Name:              "fieldtrial_testing_config_off",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_container"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDisable},
			},
			{
				Name:              "fieldtrial_testing_config_on",
				ExtraAttr:         []string{"informational", "group:chrome_uprev_cbx"},
				ExtraSoftwareDeps: []string{"android_container"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigEnable},
			},
			{
				Name:              "vm",
				ExtraAttr:         []string{"group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDefault},
			},
			{
				Name:              "x",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm_t"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDefault},
			},
			{
				Name:              "fieldtrial_testing_config_off_x",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm_t"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigDisable},
			},
			{
				Name:              "fieldtrial_testing_config_on_x",
				ExtraAttr:         []string{"informational", "group:chrome_uprev_cbx"},
				ExtraSoftwareDeps: []string{"android_vm_t"},
				Val:               optinTestArgs{preprod: false, fieldTrialConfig: chrome.FieldTrialConfigEnable},
			},
			{
				Name:              "preprod",
				ExtraAttr:         []string{"group:hw_agnostic", "informational"},
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				Val:               optinTestArgs{preprod: true, fieldTrialConfig: chrome.FieldTrialConfigDefault},
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
	// This will redirect *.google.com and *.googleapis.com to the preprod of Google frontend.
	if args.preprod {
		env, err := testenv.NewPreprodEnv(ctx, testenv.RedirectMap(arc.PassThroughPreprodGFE))
		if err != nil {
			s.Fatal("Failed to redirect to preprod: ", err)
		}
		defer env.Close(cleanupCtx)
	}

	s.Log("Performing optin")

	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin: ", err)
	}
}
