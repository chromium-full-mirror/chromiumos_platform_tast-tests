// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
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
		Func:         FullRestoreAlwaysRestore,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test full restore always restore setting",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"nancylingwang@google.com",
		},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"informational"},
			Val:               browser.TypeLacros,
		}},
		Timeout: 6 * time.Minute,
	})
}

func FullRestoreAlwaysRestore(ctx context.Context, s *testing.State) {
	const iterationCount = 2
	for i := 0; i < iterationCount; i++ {
		testing.ContextLogf(ctx, "Running: iteration %d/%d", i+1, iterationCount)

		if err := openBrowser(ctx, s.Param().(browser.Type)); err != nil {
			s.Fatal("Failed to open browser: ", err)
		}

		if err := restoreBrowser(ctx, s.Param().(browser.Type), s.OutDir(), s.HasError); err != nil {
			s.Fatal("Failed to do full restore: ", err)
		}
	}
}

func openBrowser(ctx context.Context, bt browser.Type) error {
	// TODO(crbug.com/1318180): at the moment for Lacros, we're not getting SetUpWithNewChrome
	// close closure because when used it'd close all resources, including targets and wouldn't let
	// the session to proper restore later. As a short term workaround we're closing Lacros
	// resources using CloseResources fn instead, though ideally we want to use
	// SetUpWithNewChrome close closure when it's properly implemented.
	var cr *chrome.Chrome
	var br *browser.Browser
	var err error

	// Sometimes, it fails to start Chrome.
	// Give it a retry.
	const retry = 2
	for i := 0; i < retry; i++ {
		cr, br, _, err = browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig())
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
	conn, err := br.NewConn(ctx, "https://abc.xyz")
	if err != nil {
		return errors.Wrap(err, "failed to connect to the restore URL")
	}
	defer conn.Close()

	// Open OS settings to set the 'Always restore' setting.
	if _, err = ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("Apps").Role(role.Link)); err != nil {
		return errors.Wrap(err, "failed to launch Apps Settings")
	}

	ui := uiauto.New(tconn)
	restoreCombox := nodewith.Name("Restore apps on startup").Role(role.ComboBoxSelect)
	alwaysRestoreOption := nodewith.Name("Always restore").Role(role.ListBoxOption)
	if err := uiauto.Combine("set 'Always restore' Settings",
		ui.LeftClickUntil(restoreCombox, ui.WaitUntilExists(alwaysRestoreOption)),
		ui.LeftClick(alwaysRestoreOption))(ctx); err != nil {
		return errors.Wrap(err, "failed to set 'Always restore' Settings")
	}

	// According to the PRD of Full Restore go/chrome-os-full-restore-dd,
	// it uses a throttle of 2.5s to save the app launching and window statue information to the backend.
	// Therefore, sleep 5 seconds here.
	testing.Sleep(ctx, 5*time.Second)

	if bt == browser.TypeLacros {
		l, err := lacros.Connect(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to connect to lacros-chrome")
		}
		defer l.CloseResources(ctx)
	}

	return nil
}

func restoreBrowser(ctx context.Context, bt browser.Type, outDir string, hasError func() bool) error {
	opts := []chrome.Option{
		// Set not to clear the notification after restore.
		// By default, On startup is set to ask every time after reboot
		// and there is an alertdialog asking the user to select whether to restore or not.
		chrome.RemoveNotification(false),
		chrome.DisableFeatures("ChromeWhatsNewUI"),
		chrome.EnableRestoreTabs(),
		chrome.KeepState()}

	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
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
	if err := ash.WaitForCondition(ctx, tconn, ash.BrowserTitleMatch(bt, "Alphabet"),
		&testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrapf(err, "failed to wait for the window to be open, browser: %v", bt)
	}

	// Confirm that the Settings app is restored.
	if err := uiauto.New(tconn).WaitUntilExists(ossettings.SearchBoxFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to restore the Settings app")
	}

	return nil
}
