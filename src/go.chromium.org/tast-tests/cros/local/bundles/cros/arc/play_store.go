// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type playStoreTestArgs struct {
	preprod bool // whether to run against preprod versions of dependencies (default: false)
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStore,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test of the Play Store that installs Google Calculator",
		Contacts: []string{
			// Please assign test failures to current constable on-call
			"arc-constables@google.com",
			"arc-core@google.com",
			"mhasank@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:arc-functional", "group:mainline"},
		SoftwareDeps: []string{"play_store", "chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
			Val:               playStoreTestArgs{preprod: false},
		}, {
			Name:              "betty",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container", "qemu"},
			Val:               playStoreTestArgs{preprod: false},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "no_qemu", "no_android_vm_t"},
			Val:               playStoreTestArgs{preprod: false},
		}, {
			Name:              "x",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "no_qemu", "android_vm_t"},
			Val:               playStoreTestArgs{preprod: false},
		}, {
			Name:              "betty_vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm", "qemu"},
			Val:               playStoreTestArgs{preprod: false},
		}, {
			Name:              "preprod",
			ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm", "qemu"}, // Use betty_vm configuration for googleapis
			Val:               playStoreTestArgs{preprod: true},
		}},
		Timeout: 15 * time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func PlayStore(ctx context.Context, s *testing.State) {
	const (
		pkgName             = "com.google.android.keep"
		installationTimeout = 10 * time.Minute
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
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

		if err := optin.PerformWithRetry(ctx, cr, 2 /*maxAttempts*/); err != nil {
			return rl.Retry("optin to Play Store", err)
		}

		// Set up the test environment for external dependencies post opt-in.
		// This will redirect *.google.com and *.googleapis.com to the preprod of Google frontend
		// to see that PlayStore works with any changes coming in this environment.
		if s.Param().(playStoreTestArgs).preprod {
			env, err := testenv.NewPreprodEnv(ctx,
				testenv.RedirectMap(arc.PassThroughPreprodGFE))
			if err != nil {
				return rl.Retry("set up the preprod env", err)
			}
			defer env.Close(cleanupCtx)
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return rl.Retry("create test API Connection", err)
		}
		if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
			return rl.Retry("wait for Play Store to show", err)
		}

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			return rl.Retry("start ARC", err)
		}
		defer a.Close(cleanupCtx)
		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), func() bool {
			return s.HasError() || retErr != nil
		})
		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Retry("create UIAutomator", err)
		}
		defer d.Close(cleanupCtx)

		recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
		defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)

		s.Log("Installing app")
		if err := playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{TryLimit: -1, InstallationTimeout: installationTimeout}); err != nil {
			return rl.Exit("install the app", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Play Store test failed: ", err)
	}
}
