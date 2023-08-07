// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OptinHealth,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test that verifies that ARC is healthy during optin",
		Contacts: []string{
			"arc-core@google.com",
			"mhasank@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:cq-minimal"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "container_r",
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"android_container_r"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:cq-minimal", "informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 6 * time.Minute,
	})
}

// OptinHealth verifies that ARC is healthy during optin.
func OptinHealth(ctx context.Context, s *testing.State) {
	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) error {

		gaiaLogin := chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))

		cr, err := chrome.New(ctx,
			gaiaLogin,
			chrome.ARCSupported(),
			chrome.UnRestrictARCCPU(),
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

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			return rl.Retry("start ARC", err)
		}
		defer a.Close(cleanupCtx)

		out, err := a.Command(ctx, "logcat", "-d").Output(testexec.DumpLogOnError)
		if err != nil {
			return rl.Retry("run logcat", err)
		}

		r := regexp.MustCompile("ArcCrashDumpStreamer:.*process name = org.chromium.arc.*")
		m := r.FindStringSubmatch(string(out))
		if len(m) > 0 {
			s.Fatalf("Found %d org.chromium.arc.* crashes", len(m))
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Optin crash test failed: ", err)
	}
}
