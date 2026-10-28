// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"os"
	"time"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type logoutMethod int

const (
	logoutShortcut logoutMethod = iota
	logoutClickOnButton
)

const (
	logoutStartedFlagFile = "/run/bootstat/stats-logout-started.written"
	logoutStartedDoneFile = "/run/bootstat/stats-logout-started.written.done"
	sendUptimeMetricsJob  = "send-uptime-metrics"
	uptimeLogoutHistogram = "Uptime.Logout"
	// send-uptime-metrics sets CHROME_LOGOUT_STARTED=-1000000 when the
	// logout-started flag is missing, forcing samples into the 1000 s
	// (1000000 ms) overflow bucket.
	uptimeLogoutOverflowMin = 1000 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Logout,
		Desc: "Verify that the sign in page shows after signing out",
		Contacts: []string{
			"cros-oac@google.com",
			"cros-lurs@google.com",
			"awendy@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr:         []string{"group:golden_tier_secondary", "group:medium_low_tier", "group:hardware", "group:complementary", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      chrome.LoginTimeout + time.Minute,
		Params: []testing.Param{
			{
				Val: logoutShortcut,
			}, {
				Name: "button",
				Val:  logoutClickOnButton,
			},
		},
	})
}

// Logout logs out and verifies if the sign in screen is being displayed.
func Logout(ctx context.Context, s *testing.State) {
	method := s.Param().(logoutMethod)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	const (
		username = "testuser@gmail.com"
		password = "testpass"
	)

	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if cr != nil {
			cr.Close(cleanupCtx)
		}
		// Restart the ui job so the Chrome instance started below with custom
		// flags does not outlive the test.
		if err := upstart.RestartJob(cleanupCtx, "ui"); err != nil {
			s.Error("Failed to restart ui job: ", err)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	defer func() {
		faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "logout")
	}()

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	const state = "stopped"
	sw, err := sm.WatchSessionStateChanged(ctx, state)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(cleanupCtx)

	// Stop any send-uptime-metrics job still running from the initial login
	// prompt and clear stale flag and UMA files before signing out.
	if err := upstart.StopJob(ctx, sendUptimeMetricsJob); err != nil {
		s.Fatal("Failed to stop send-uptime-metrics before logout: ", err)
	}
	for _, p := range []string{logoutStartedFlagFile, logoutStartedDoneFile} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			s.Fatalf("Failed to remove %s: %v", p, err)
		}
	}
	if err := metrics.ClearHistogramTransferFile(); err != nil {
		s.Fatal("Failed to clear histogram transfer file: ", err)
	}

	switch method {
	case logoutShortcut:
		if err := logoutByShortcut(ctx, tconn); err != nil {
			s.Fatal("Failed to logout: ", err)
		}
	case logoutClickOnButton:
		if err := quicksettings.SignOut(ctx, tconn); err != nil {
			s.Fatal("Failed to logout: ", err)
		}
	}

	s.Logf("Waiting for SessionStateChanged %q D-Bus signal from session_manager", state)
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal")
	case <-ctx.Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", ctx.Err())
	}

	s.Log("Waiting for send-uptime-metrics to consume logout-started flag")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(logoutStartedDoneFile); err != nil {
			return err
		}
		return upstart.WaitForJobStatus(ctx, sendUptimeMetricsJob, upstartcommon.StopGoal, upstartcommon.WaitingState, upstart.RejectWrongGoal, 0)
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		s.Fatal("send-uptime-metrics did not consume logout-started flag: ", err)
	}

	// Close Chrome to do some cleanup.
	// It will log some errors, as the session is closed already.
	cr.Close(ctx)
	cr = nil

	if cr, err = chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing", "--external-metrics-collection-interval=1"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	); err != nil {
		s.Fatal("Failed to restart Chrome for testing: ", err)
	}

	// Restarting Chrome at the login screen emits a second login-prompt-visible
	// without a logout-started flag; stop send-uptime-metrics before its 10 s
	// timeout writes a -1000000 ms sentinel sample over the real logout's
	// Uptime.Logout sample.
	if err := upstart.StopJob(ctx, sendUptimeMetricsJob); err != nil {
		s.Fatal("Failed to stop send-uptime-metrics after restarting Chrome: ", err)
	}

	if tconn, err = cr.SigninProfileTestAPIConn(ctx); err != nil {
		s.Fatal("Failed to re-establish test API connection")
	}

	if err := lockscreen.WaitForPasswordEntry(ctx, tconn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for login screen: ", err)
	}

	if err := lockscreen.EnterPassword(ctx, tconn, username, password, kb); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if _, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.LoggedIn }, 10*time.Second); err != nil {
		s.Fatal("Failed to log in: ", err)
	}

	// --enable-oobe-test-api preloads the hidden OOBE dialog, keeping
	// BEST_EFFORT tasks like ExternalMetrics fenced until a visible page loads.
	conn, err := cr.NewConn(ctx, "about:blank")
	if err != nil {
		s.Fatal("Failed to open about:blank: ", err)
	}
	defer conn.Close()

	hist, err := metrics.WaitForHistogram(ctx, tconn, uptimeLogoutHistogram, 15*time.Second)
	if err != nil {
		s.Fatalf("Failed to get %s histogram: %v", uptimeLogoutHistogram, err)
	}
	for _, b := range hist.Buckets {
		if time.Duration(b.Min)*time.Millisecond >= uptimeLogoutOverflowMin && b.Count > 0 {
			s.Fatalf("%s recorded overflow bucket %v (full histogram: %v)", uptimeLogoutHistogram, b, hist)
		}
	}
}

func logoutByShortcut(ctx context.Context, tconn *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create the keyboard")
	}
	defer kb.Close(ctx)

	return uiauto.Combine("press the keyboard to logout",
		kb.AccelAction("Shift+Ctrl+Q"),
		kb.AccelAction("Shift+Ctrl+Q"),
	)(ctx)
}
