// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ManagedGuestSessionPlayStore,
		Desc: "Verifies that Play Store is not available in Managed Guest Session",
		Contacts: []string{
			"arc-commercial@google.com",
			"yaohuali@google.com",
		},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		SoftwareDeps: []string{"reboot", "chrome", "play_store"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
		VarDeps: []string{
			arcCommon.ManagedAccountPoolVarName,
		},
		Fixture: fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
	})
}

func ManagedGuestSessionPlayStore(ctx context.Context, s *testing.State) {
	const (
		bootTimeout = 4 * time.Minute
	)

	fdms := s.FixtValue().(*fakedms.FakeDMS)
	policies := []policy.Policy{&policy.ArcEnabled{Val: true}}

	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
		mgs.AddPublicAccountPolicies(mgs.MgsAccountID, policies),
		mgs.ExtraChromeOptions(
			chrome.ARCEnabled(),
			chrome.ExtraArgs("--force-devtools-available"),
		),
	)
	if err != nil {
		s.Fatal("Failed to start MGS: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if err := mgs.Close(ctx); err != nil {
			s.Fatal("Failed to close MGS: ", err)
		}
	}(cleanupCtx)

	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := arcent.WaitForProvisioning(ctx, a, 1 /*attempt*/); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	appsInLauncher, err := ash.AppsInLauncher(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get apps in launcher: ", err)
	}

	for _, app := range appsInLauncher {
		if app.AppID == apps.PlayStore.ID {
			s.Fatal("Play Store should not be present in launcher")
		}
	}
}
