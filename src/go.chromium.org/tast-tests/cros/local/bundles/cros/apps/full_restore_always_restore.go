// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FullRestoreAlwaysRestore,
		Desc: "Test full restore always open setting",
		Contacts: []string{
			"lt-web-apps-team@google.com",
			"renkens@google.com",
		},
		BugComponent: "b:1389907",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      6 * time.Minute,
	})
}

func FullRestoreAlwaysRestore(ctx context.Context, s *testing.State) {
	const iterationCount = 2
	for i := 0; i < iterationCount; i++ {
		testing.ContextLogf(ctx, "Running: iteration %d/%d", i+1, iterationCount)

		if err := openBrowser(ctx); err != nil {
			s.Fatal("Failed to open browser: ", err)
		}

		if err := restoreBrowser(ctx, s.OutDir(), s.HasError); err != nil {
			s.Fatal("Failed to do full restore: ", err)
		}
	}
}

func openBrowser(ctx context.Context) error {
	var cr *chrome.Chrome
	var err error

	// Sometimes, it fails to start Chrome.
	// Give it a retry.
	const retry = 2
	for i := 0; i < retry; i++ {
		cr, err = chrome.New(ctx)
		if err == nil {
			break
		}
	}
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect Test API")
	}

	// Open browser.
	// The opened browser is not closed before reboot so that it could be restored after reboot.
	conn, err := cr.NewConn(ctx, "https://abc.xyz")
	if err != nil {
		return errors.Wrap(err, "failed to connect to the restore URL")
	}
	defer conn.Close()

	// Open OS settings to set the 'Always open' setting.
	if _, err = ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("System preferences").Role(role.Link)); err != nil {
		return errors.Wrap(err, "failed to launch system preferences settings")
	}

	ui := uiauto.New(tconn)
	recapCombox := nodewith.Name("Welcome Recap").Role(role.ComboBoxSelect)
	alwaysOpenOption := nodewith.Name("Always open").Role(role.MenuListOption)
	if err := uiauto.Combine("set 'Always open' settings",
		ui.WaitUntilExists(recapCombox),
		ui.ScrollToVisible(recapCombox),
		ui.LeftClickUntil(recapCombox, ui.WaitUntilExists(alwaysOpenOption)),
		ui.LeftClick(alwaysOpenOption))(ctx); err != nil {
		return errors.Wrap(err, "failed to set 'Always open' settings")
	}

	// GoBigSleepLint: According to the PRD of Full Restore go/chrome-os-full-restore-dd,
	// it uses a throttle of 2.5s to save the app launching and window statue information to the backend.
	// Therefore, sleep 5 seconds here.
	// TODO(b/394258768): Find a way to wait for the sync instead of sleeping.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	return nil
}

func restoreBrowser(ctx context.Context, outDir string, hasError func() bool) error {
	opts := []chrome.Option{
		// Set not to clear the notification after restore.
		// By default, On startup is set to ask every time after reboot
		// and there is a window asking the user to select whether to restore or not.
		chrome.RemoveNotification(false),
		chrome.DisableFeatures("ChromeWhatsNewUI"),
		chrome.EnableRestoreTabs(),
		chrome.KeepState()}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect Test API")
	}

	defer faillog.DumpUITreeOnError(ctx, outDir, hasError, tconn)

	// Confirm that the browser is restored.
	if err := ash.WaitForCondition(ctx, tconn, ash.BrowserTitleMatch("Alphabet"),
		&testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for the browser window to be open")
	}

	// Confirm that the Settings app is restored.
	if err := uiauto.New(tconn).WaitUntilExists(ossettings.SearchBoxFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to restore the Settings app")
	}

	return nil
}
