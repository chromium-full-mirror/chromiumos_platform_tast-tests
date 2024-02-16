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
		Attr:         []string{"group:mainline"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:cq-minimal"},
				ExtraSoftwareDeps: []string{"android_container"},
			},
			{
				Name:              "vm",
				ExtraAttr:         []string{"group:cq-minimal", "informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t"},
			},
			{
				Name:              "x",
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"android_vm_t"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

// OptinHealth verifies that ARC is healthy during optin.
func OptinHealth(ctx context.Context, s *testing.State) {
	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
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
			return rl.Exit("create test API Connection", err)
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

		r := regexp.MustCompile("ArcCrashDumpStreamer: (.*) found [(]process name = (org.chromium.arc.*)[)]")
		m := r.FindAllStringSubmatch(string(out), -1)

		for _, match := range m {
			s.Errorf("Found %s crash in %s", match[1], match[2])
		}

		if len(m) > 0 {
			s.Fatalf("Found total %d crashes", len(m))
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Optin crash test failed: ", err)
	}
}
