// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type managedPlayStoreIconTestArgs struct {
	arcEnabled  *policy.ArcEnabled
	wantEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStoreIcon,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures ARC policy controls the visibility of Play Store icon",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		SoftwareDeps: []string{"chrome", "no_tablet_form_factor"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      4 * time.Minute,
		VarDeps:      []string{arcent.LoginPoolVar},
		Params: []testing.Param{{
			Name: "enabled",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Val: true},
				wantEnabled: true,
			},
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name: "disabled",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Val: false},
				wantEnabled: false,
			},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name: "unset",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Stat: policy.StatusUnset},
				wantEnabled: false,
			},
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name: "enabled_vm",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Val: true},
				wantEnabled: true,
			},
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "disabled_vm",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Val: false},
				wantEnabled: false,
			},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "unset_vm",
			Val: managedPlayStoreIconTestArgs{
				arcEnabled:  &policy.ArcEnabled{Stat: policy.StatusUnset},
				wantEnabled: false,
			},
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// ManagedPlayStoreIcon verifies that the ArcEnabled policy shows/hides the Play Store icon.
func ManagedPlayStoreIcon(ctx context.Context, s *testing.State) {
	args := s.Param().(managedPlayStoreIconTestArgs)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	// Sync needs to be disabled for shelf icons to show. See b/303237403#comment12
	policies := []policy.Policy{args.arcEnabled, &policy.SyncDisabled{Val: true}}
	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
	defer fdms.Stop(cleanupCtx)

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Chrome startup failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Look for the Play Store icon.
	// Polling till the icon is found or the timeout is reached.
	uia := uiauto.New(tconn)
	notFoundError := errors.New("Play Store icon is not found yet")
	err = testing.Poll(ctx, func(ctx context.Context) error {
		if found, err := uia.IsNodeFound(ctx, nodewith.Name(apps.PlayStore.Name).ClassName("ash/ShelfAppButton")); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return testing.PollBreak(errors.Wrap(err, "failed to find Play Store icon"))
		} else if found {
			return nil
		}
		return notFoundError
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second})

	if err != nil && !errors.Is(err, notFoundError) {
		s.Fatal("Failed to confirm the status of the Play Store icon: ", err)
	}

	if enabled := err == nil; enabled != args.wantEnabled {
		s.Errorf("Unexpected Play Store icon presence; got %t, want %t", enabled, args.wantEnabled)
	}
}
