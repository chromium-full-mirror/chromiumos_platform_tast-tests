// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/removablemedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalStorageDisabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that ExternalStorageDisabled policy is correctly applied to ARC",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		VarDeps:      []string{"arc.managedAccountPool"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ExternalStorageDisabled{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

func ExternalStorageDisabled(ctx context.Context, s *testing.State) {
	// Actual username and password are read from vars/arc.yaml.
	creds, err := credconfig.PickRandomCreds(s.RequiredVar("arc.managedAccountPool"))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	policies := []policy.Policy{
		&policy.ArcEnabled{Val: true, Stat: policy.StatusSet},
		&policy.ExternalStorageDisabled{Val: true, Stat: policy.StatusSet},
	}

	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
	if err != nil {
		s.Fatal("Failed to set up fake policy server: ", err)
	}
	defer fdms.Stop(ctx)

	// If fdms forces ARC opt-in, then ARC opt-in will start in background, right after chrome is created.
	cr, err := chrome.New(ctx,
		chrome.GAIALogin(creds),
		chrome.DMSPolicy(fdms.URL),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	s.Log("Performing MyFiles sharing check")

	if err := arc.WaitForARCMyFilesVolumeMount(ctx, a); err != nil {
		s.Fatal("Failed to wait for MyFiles to be mounted in ARC: ", err)
	}

	const (
		imageSize = 64 * 1024 * 1024
		diskName  = "MyDisk"
	)

	// Create a filesystem image and mount it on the host side. This should work
	// even when the ExternalStorageDisabled policy is set to true since we
	// directly ask CrosDisks to mount it without having it check the policy
	// with Chrome here.
	_, cleanupFunc, err := removablemedia.CreateAndMountImage(ctx, imageSize, diskName)
	if err != nil {
		s.Fatal("Failed to set up image: ", err)
	}
	defer cleanupFunc(ctx)

	s.Log("Performing removable media sharing check")

	// Check that the image is not mounted on ARC.
	if err := arc.WaitForARCRemovableMediaVolumeMount(ctx, a); err == nil {
		s.Fatal("The volume is unexpectedly mounted on ARC")
	}
}
