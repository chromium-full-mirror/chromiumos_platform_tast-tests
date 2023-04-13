// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/syslog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LacrosRestartOnCrash,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that Kiosk Lacros properly restarts",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"zubeil@google.com", // Test author
		},
		// Only golden suite as this test keeps failing - product failure.
		// Only one suite reduces infa load.
		Attr:         []string{"group:golden_tier"},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "chrome", "lacros"},
		Fixture:      fixture.KioskAutoLaunchCleanup,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Relaunch PWA kiosk after OS crash.
				Value: "screenplay-86fc814e-2bdd-4680-bbb2-defed8bde33c",
			},
		},
	})
}

func LacrosRestartOnCrash(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
		kioskmode.PublicAccountPolicies(kioskmode.WebKioskAccountID,
			[]policy.Policy{
				&policy.LacrosAvailability{Val: "lacros_only"},
			}),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(ctx)

	testing.ContextLog(ctx, "Waiting for splash screen to be  gone")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return kioskmode.IsKioskAppStarted(ctx)
	}, &testing.PollOptions{Interval: 10 * time.Millisecond, Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Splash screen is not gone: ", err)
	}

	// Start reader for /var/log/messages to check kiosk mode has started.
	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		s.Fatal("Failed to start log reader: ", err)
	}
	defer reader.Close()

	// Start reader for chrome log to check that lacros was used for kiosk mode.
	chromeReader, err := syslog.NewReader(ctx, syslog.SourcePath(cr.LogFilename()))
	if err != nil {
		s.Fatal("Failed to start log reader: ", err)
	}
	defer chromeReader.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Kill lacros using SIGSEGV signal to simulate a browser crash.
	s.Log("Killing lacros")
	proc, err := lacrosproc.Root(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get lacros proc: ", err)
	}

	if err := proc.SendSignalWithContext(ctx, unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash chrome: ", err)
	}

	// Check that the kiosk recovers and lacros is used as browser.
	if err := kioskmode.ConfirmKioskStarted(ctx, reader); err != nil {
		s.Fatal("Failed to start kiosk mode after killing: ", err)
	}

	// Check that lacros was used for kiosk mode.
	const expectedLogMsg = "Using lacros-chrome for web kiosk session."
	s.Log("Waiting for lacros log message")
	if _, err := chromeReader.Wait(ctx, 60*time.Second,
		func(e *syslog.Entry) bool {
			return strings.Contains(e.Content, expectedLogMsg)
		}); err != nil {
		s.Errorf("Failed to wait for log msg \"%q\": %v", expectedLogMsg, err)
	}
}
