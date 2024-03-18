// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedDevicePolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This test ensure that managed policies are applied to Android",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		VarDeps: []string{
			arcent.LoginPoolVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty",
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "x",
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func ManagedDevicePolicy(ctx context.Context, s *testing.State) {
	const (
		apk = "ArcDevicePolicyTest.apk"
		pkg = "org.chromium.arc.testapp.devicepolicy"
		cls = pkg + ".MainActivity"

		inputTextID  = pkg + ":id/txtInput"
		testButtonID = pkg + ":id/btnTest"
		outputTextID = pkg + ":id/txtOutput"

		defaultTimeout = 30 * time.Second
	)

	creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	login := chrome.GAIALogin(creds)
	fdms, err := arcent.SetupPolicyServerWithArcApps(ctx, s.OutDir(), creds.User, []string{pkg}, arcent.InstallTypeAvailable, arcent.PlayStoreModeAllowList)
	if err != nil {
		s.Fatal("Failed to setup fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		login,
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(append(arc.DisableSyncFlags(), "--vmodule=arc_policy_bridge=1")...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.NewWithTimeout(ctx, s.OutDir(), arc.BootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := arcent.WaitForProvisioning(ctx, a, 1 /*attempt*/); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	s.Log("Installing app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	s.Log("Starting app")
	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed to create a new activity: ", err)
	}
	defer act.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the activity: ", err)
	}
	defer act.Stop(cleanupCtx, tconn)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	txtInput := d.Object(ui.ID(inputTextID))
	if err := txtInput.WaitForExists(ctx, defaultTimeout); err != nil {
		s.Fatal("Failed to wait for input text to exist: ", err)
	}

	if err := txtInput.SetText(ctx, "hello"); err != nil {
		s.Fatal("Failed to set input message text: ", err)
	}

	btnTest := d.Object(ui.ID(testButtonID))
	if err := btnTest.Click(ctx); err != nil {
		s.Fatal("Failed to click test: ", err)
	}

	txtOutput := d.Object(ui.ID(outputTextID))
	output, err := txtOutput.GetText(ctx)
	if err != nil {
		s.Fatal("Failed to get output: ", err)
	}

	if output != "hello" {
		s.Fatal("Unexpected output " + output)
	}
}
