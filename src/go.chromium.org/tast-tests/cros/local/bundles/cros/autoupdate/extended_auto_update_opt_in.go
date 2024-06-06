// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	chromepkg "go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	nebraskapkg "go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtendedAutoUpdateOptIn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Triggers extended auto update consumer opt in UI, opts in through the UI, and verifies the correct flag is sent on update request",
		Contacts:     []string{"mpolzer@google.com", "artyomchen@google.com"},
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.UpdateEngine, // Need a clean instance of update engine.
	})
}

func ExtendedAutoUpdateOptIn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	nebraska, err := setUpNebraskaForExtendedOptInRequired(ctx)
	if err != nil {
		s.Fatal("Failed to set up Nebraska: ", err)
	}
	defer nebraska.Close(cleanupCtx)

	// Check for updates to propagate extended opt in requirements from Nebraska to update_engine.
	if err := testexec.CommandContext(ctx, "update_engine_client", "--check_for_update", "--skip_applying").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to check for updates: ", err)
	}

	// Logs in with a fresh owner of the device. Only device owner can see the opt in UI.
	chrome, err := chromepkg.New(
		ctx,
	)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer chrome.Close(cleanupCtx)

	// This log indicates we haven't opted in to extended auto updates yet.
	extendedNotOk := "extended_okay=\"false\""
	if err := awaitUpdateParam(ctx, nebraska, extendedNotOk); err != nil {
		s.Fatal("Failure while awaiting Nebraska logs to decline extended auto updates: ", err)
	}

	// Opt in to extended auto updates via Chrome.
	testConn, err := chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	settings, err := ossettings.LaunchAtPage(ctx, testConn, ossettings.AboutChromeOS)
	if err != nil {
		s.Fatal("Failed to launch settings: ", err)
	}
	defer settings.Close(cleanupCtx)

	s.AttachErrorHandlers(
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, chrome, "ui")
		},
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, chrome, "ui")
		})

	ui := uiauto.New(testConn)

	// TODO(b/333670429) Update to use other identifiers instead of Name once we have accessibility labels.
	extendedAutoUpdateText := nodewith.NameContaining("Turn on extended security updates")
	// Developer tools have the same button name, use first here as it's the only
	// way to distinguish the two.
	setUpButton := nodewith.Role(role.Button).Name("Set up").First()
	turnOnButton := nodewith.Role(role.Button).Name("Turn on")
	confirmButton := nodewith.Role(role.Button).Name("Confirm & turn on")
	if err := uiauto.Combine("opt in to extended auto updates",
		ui.WaitUntilExists(extendedAutoUpdateText),
		ui.LeftClick(setUpButton),
		ui.WaitUntilExists(turnOnButton),
		ui.LeftClick(turnOnButton),
		ui.WaitUntilExists(confirmButton),
		ui.LeftClick(confirmButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click on set up: ", err)
	}

	extendedOk := "extended_okay=\"true\""
	if err := awaitUpdateParam(ctx, nebraska, extendedOk); err != nil {
		s.Fatal("Failure while awaiting Nebraska logs to accept extended auto updates: ", err)
	}
}

func awaitUpdateParam(ctx context.Context, nebraska *nebraskapkg.Nebraska, log string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Check for updates.
		if err := testexec.CommandContext(ctx, "update_engine_client", "--check_for_update", "--skip_applying").Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to check for updates")
		}

		nebraskaLog, err := os.ReadFile(nebraska.LogFile)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
		}

		if !strings.Contains(string(nebraskaLog), log) {
			return errors.Errorf("%q not in the nebraska logs", log)
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		return err
	}
	return nil
}

func setUpNebraskaForExtendedOptInRequired(ctx context.Context) (*nebraskapkg.Nebraska, error) {
	nebraska, err := nebraskapkg.New(ctx, nebraskapkg.ConfigureUpdateEngine())
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Nebraska")
	}

	today := time.Now()

	// For the UI to show, eol date must be in the future, extended date in the past,
	// and opt in must be required.

	tomorrow := today.AddDate(0, 0, 1)
	if err := nebraska.SetEolDate(ctx, tomorrow); err != nil {
		return nil, errors.Wrap(err, "failed to configure end of life date")
	}

	if err := nebraska.SetExtendedOptInRequired(ctx, true); err != nil {
		return nil, errors.Wrap(err, "failed to configure extended opt in required")
	}

	yesterday := today.AddDate(0, 0, -1)
	if err := nebraska.SetExtendedDate(ctx, yesterday); err != nil {
		return nil, errors.Wrap(err, "failed to configure extended date")
	}

	return nebraska, nil
}
