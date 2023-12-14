// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SecondBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test to verify that ARC can successfully boot after provisioning",
		Contacts:     []string{"arc-core@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:arc-functional", "group:mainline"},
		SoftwareDeps: []string{"play_store", "chrome"},
		Params: []testing.Param{{
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
		}, {
			Name:              "betty",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container", "qemu"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "no_qemu", "no_android_vm_t"},
		}, {
			Name:              "x",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "no_qemu", "android_vm_t"},
		}, {
			Name:              "betty_vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "qemu"},
		}},
		Timeout: (chrome.LoginTimeout * 2) + (arc.BootTimeout * 2) + 5*time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func SecondBoot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	loginPool := s.RequiredVar("ui.gaiaPoolDefault")

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(loginPool),
		chrome.UnRestrictARCCPU(),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		cr.Close(cleanupCtx)
	}()

	if err := optin.PerformWithRetry(ctx, cr, 2 /*maxAttempts*/); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store to show: ", err)
	}

	cr.Close(ctx)

	s.Log("First session complete. Starting second session")

	cr, err = chrome.New(ctx,
		chrome.GAIALogin(cr.Creds()),
		chrome.UnRestrictARCCPU(),
		chrome.ARCSupported(),
		chrome.KeepState(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to re-connect to Chrome: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test connection in the second session: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to re-launch Play Store: ", err)
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store to show in the second session: ", err)
	}
}
