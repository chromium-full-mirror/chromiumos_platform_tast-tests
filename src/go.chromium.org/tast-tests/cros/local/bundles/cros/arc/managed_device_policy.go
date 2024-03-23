// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/imagehelpers"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/externaldata"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		Data: []string{"wallpaper_image.jpeg"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.WallpaperImage{}, pci.VerifiedFunctionalityUI),
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

type arcPolicyFactory func() (policy.Policy, func(ctx context.Context), error)

func ManagedDevicePolicy(ctx context.Context, s *testing.State) {
	const (
		apk = "ArcDevicePolicyTest.apk"
		pkg = "org.chromium.arc.testapp.devicepolicy"
		cls = pkg + ".MainActivity"
	)

	packages := []string{pkg}
	arcPolicyMap := map[string]arcPolicyFactory{
		"setWallpaper": func() (policy.Policy, func(ctx context.Context), error) {
			return createWallpaperPolicy(ctx, s.DataPath("wallpaper_image.jpeg"))
		},
	}

	creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	login := chrome.GAIALogin(creds)
	fdms, err := arcent.SetupPolicyServerWithArcApps(ctx, s.OutDir(), creds.User, packages, arcent.InstallTypeAvailable, arcent.PlayStoreModeAllowList)
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

	s.Log("Testing policies without restrictions")
	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)
	for policyName := range arcPolicyMap {
		if err := testPolicyEnforcement(ctx, d, policyName, true /*shouldSucceed*/); err != nil {
			s.Fatalf("Test for policy %s failed: %v", policyName, err)
		}
	}

	s.Log("Updating policies to apply restrictions")
	arcPolicy := arcent.CreateArcPolicyWithApps(packages, arcent.InstallTypeAvailable, arcent.PlayStoreModeAllowList)
	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy}
	for policyName := range arcPolicyMap {
		newPolicy, cleanup, err := arcPolicyMap[policyName]()
		if err != nil {
			s.Fatalf("Failed to create %s policy: %v", policyName, err)
		}
		defer cleanup(cleanupCtx)
		policies = append(policies, newPolicy)
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	s.Log("Testing policies with restrictions")
	for policyName := range arcPolicyMap {
		if err := testPolicyEnforcement(ctx, d, policyName, false /*shouldSucceed*/); err != nil {
			s.Fatalf("Test for policy %s failed: %v", policyName, err)
		}
	}
}

func createWallpaperPolicy(ctx context.Context, imgPath string) (policy.Policy, func(ctx context.Context), error) {
	jpegBytes, err := imagehelpers.GetJPEGBytesFromFilePath(imgPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read wallpaper image")
	}

	eds, err := externaldata.NewServer(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create external data server")
	}
	cleanup := func(ctx context.Context) { eds.Stop(ctx) }

	iurl, ihash := eds.ServePolicyData(jpegBytes)

	policy := &policy.WallpaperImage{Val: &policy.WallpaperImageValue{Url: iurl, Hash: ihash}}

	return policy, cleanup, nil
}

func testPolicyEnforcement(ctx context.Context, d *ui.Device, policy string, shouldSucceed bool) error {
	const (
		pkg = "org.chromium.arc.testapp.devicepolicy"

		policiesListID = pkg + ":id/lstPolicies"
		testButtonID   = pkg + ":id/btnTest"
		outputTextID   = pkg + ":id/txtOutput"
		errorTextID    = pkg + ":id/txtError"
	)

	if err := selectSpinnerItem(ctx, d, policiesListID, policy); err != nil {
		return err
	}

	btnTest := d.Object(ui.ID(testButtonID))
	if err := btnTest.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click test")
	}

	txtOutput := d.Object(ui.ID(outputTextID))
	output, err := txtOutput.GetText(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get output")
	}

	txtError := d.Object(ui.ID(errorTextID))
	errMessage, err := txtError.GetText(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get error message")
	}

	if output != fmt.Sprintf("%v", shouldSucceed) {
		return errors.Errorf("unexpected output: %s, error: %s", output, errMessage)
	}

	return nil
}

func selectSpinnerItem(ctx context.Context, d *ui.Device, spinnerId, itemText string) error {
	const defaultTimeout = 30 * time.Second

	spinner := d.Object(ui.ID(spinnerId))
	if err := spinner.WaitForExists(ctx, defaultTimeout); err != nil {
		return errors.Wrap(err, "failed to find the spinner")
	}

	if err := spinner.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to open the spinner")
	}

	item := d.Object(ui.Text(itemText))
	if err := item.WaitForExists(ctx, defaultTimeout); err != nil {
		return errors.Wrapf(err, "failed to find %s in the spinner", itemText)
	}

	if err := item.Click(ctx); err != nil {
		return errors.Wrapf(err, "failed to select %s in the spinner", itemText)
	}

	return nil
}
