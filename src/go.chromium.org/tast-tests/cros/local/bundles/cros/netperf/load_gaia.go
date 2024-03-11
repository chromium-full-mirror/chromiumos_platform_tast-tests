// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netperf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoadGAIA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Try to load GAIA during OOBE to monitor the internet connection health",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1263917", // ChromeOS > Software > Commercial (Enterprise) > Testing
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{
			"chrome",
		},
		VarDeps: []string{
			"ui.gaiaPoolDefault",
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

func skipToLogin(ctx context.Context, cr *chrome.Chrome) error {
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return errors.Wrap(err, "no oobe connection")
	}
	defer oobeConn.Close()

	if err := oobeConn.Call(ctx, nil, "OobeAPI.skipToLoginForTesting"); err != nil {
		return err
	}

	return nil
}

func waitForGaia(ctx context.Context, cr *chrome.Chrome) error {
	testing.ContextLog(ctx, "Waiting for GAIA login screen to load")
	return testing.Poll(ctx, func(ctx context.Context) error {
		conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix("https://accounts.google.com/"))
		if err != nil {
			return errors.Wrap(err, "failed to connect to target")
		}
		defer conn.Close()

		noUsername := true
		if err = conn.Eval(ctx, "document.querySelector('#identifierId') == null", &noUsername); err != nil {
			conn.Close()
			return errors.Wrap(err, "failed to wait for GAIA title")
		} else if noUsername {
			return errors.New("username field missing")
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	})
}

func LoadGAIA(ctx context.Context, s *testing.State) {
	const totalTimeout = 1 * time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Make sure we have enough time to run this test.
	expectedDeadline := time.Now().Add(totalTimeout)
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline && expectedDeadline.After(deadline) {
		s.Fatalf("Test does not have enough time to run. Want to run until %s, can only run until %s", expectedDeadline.Format(time.RFC3339), deadline.Format(time.RFC3339))
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed saving perf data: ", err)
		}
	}()

	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on the login screen: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Don't count Chrome startup.
	start := time.Now()

	if err := skipToLogin(ctx, cr); err != nil {
		s.Fatal("Failed to skip to login: ", err)
	}

	// Wait for GAIA to be visible.
	if err := waitForGaia(ctx, cr); err != nil {
		s.Fatal("Login screen not found: ", err)
	}

	elapsed := time.Now().Sub(start)

	pv.Set(perf.Metric{
		Name:      "time_to_load",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, (float64)(elapsed.Milliseconds()))

	dumpfile := "network_dump_" + time.Now().Format("030405000") + ".txt"
	if err := dumputil.DumpNetworkInfo(ctx, dumpfile); err != nil {
		s.Error(ctx, "Failed to dump network info: ", err)
	}
}
