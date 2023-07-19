// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStore,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test of the Play Store that installs Google Calculator",
		Contacts:     []string{"arc-core@google.com", "cros-arc-te@google.com", "mhasank@chromium.org"},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:arc-functional", "group:mainline"},
		SoftwareDeps: []string{"play_store", "chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 15 * time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func PlayStore(ctx context.Context, s *testing.State) {
	const (
		pkgName = "com.google.android.calculator"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		cr, err := chrome.New(ctx,
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			chrome.UnRestrictARCCPU(),
			chrome.ARCSupported(),
			chrome.ExtraArgs(arc.DisableSyncFlags()...))
		if err != nil {
			return rl.Retry("connect to Chrome", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			rl.Exit("create test API Connection", err)
		}

		if err := optin.Perform(ctx, cr, tconn); err != nil {
			return rl.Retry("optin to Play Store", err)
		}

		a, err := arc.New(ctx, s.OutDir())
		if err != nil {
			return rl.Retry("start ARC", err)
		}
		defer a.Close(cleanupCtx)
		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), func() bool {
			return s.HasError() || retErr != nil
		})

		if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
			rl.Exit("wait for Play Store to show", err)
		}

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			rl.Exit("create UIAutomator", err)
		}
		defer d.Close(cleanupCtx)

		s.Log("Installing app")
		if err := playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{TryLimit: -1}); err != nil {
			rl.Exit("install the app", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Play Store test failed: ", err)
	}
}
