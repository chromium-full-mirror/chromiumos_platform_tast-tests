// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     SecondBoot,
		Desc:     "A functional test to verify that ARC can successfully boot after provisioning",
		Contacts: []string{"arc-core@google.com", "jhorwich@google.com"},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:arc-functional", "group:mainline"},
		SoftwareDeps: []string{"play_store", "chrome", "gaia"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
		Timeout: (chrome.LoginTimeout * 2) + (arc.BootTimeout * 2) + 5*time.Minute,
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

func SecondBoot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	creds, _ := credconfig.PickRandomCreds(dma.CredsFromPool(ui.GaiaPoolDefaultVarName))
	options := []chrome.Option{
		chrome.FakeLogin(creds),
		chrome.UnRestrictARCCPU(),
		chrome.ARCEnabled(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	}
	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		cr.Close(cleanupCtx)
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}

	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store to show: ", err)
	}

	cr.Close(ctx)

	s.Log("First session complete. Starting second session")

	cr, err = chrome.New(ctx,
		append(options, chrome.KeepState())...)
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
