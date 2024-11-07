// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/crd"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteDesktopManual,
		Desc:         "Connect to Chrome Remote Desktop for manual test",
		Contacts:     []string{"chromoting-team@google.com", "jinrongwu@google.com"},
		BugComponent: "b:47377", // Chrome > Chromoting
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"user", "pass", "contact", "mode", "extra_args",
		},
		Timeout: 10 * time.Minute,
	})
}

// RemoteDesktopManual is similar to RemoteDesktop.
// It is for manual test on the lab devices when the tester do not have
// the required devices locally.
// Some testers do not have access to the internal repo.
// Therefore make a clean copy for them.
func RemoteDesktopManual(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	user, hasUser := s.Var("user")
	if !hasUser {
		s.Fatal("You must provide a user")
	}

	pass, hasPass := s.Var("pass")

	contact, hasContact := s.Var("contact")
	if !hasPass && !hasContact {
		s.Fatal("You must ether provide password or contact for the user to login")
	}

	mode, hasMode := s.Var("mode")
	if !hasMode {
		mode = "clamshell"
	}

	extraArgsStr, ok := s.Var("extra_args")
	if !ok {
		extraArgsStr = ""
	}
	extraArgs := strings.Fields(extraArgsStr)

	var opts []chrome.Option

	chromeARCOpt := chrome.ARCDisabled()
	if arc.Supported() {
		chromeARCOpt = chrome.ARCSupported()
	}
	opts = append(opts, chromeARCOpt)
	opts = append(opts, chrome.GAIALogin(chrome.Creds{
		User:    user,
		Pass:    pass,
		Contact: contact,
	}))

	opts = append(opts, chrome.ExtraArgs(extraArgs...))

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		// In case of authentication error, provide a more informative message to the user.
		if strings.Contains(err.Error(), "chrome.Auth") {
			err = errors.Wrap(err, "please supply a password with -var=pass=<password>")
		} else if strings.Contains(err.Error(), "chrome.Contact") {
			err = errors.Wrap(err, "please supply a contact email with -var=contact=<contact>")
		}
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Change the mode to clamshell / tablet accordingly.
	if _, err := ash.EnsureTabletModeEnabled(ctx, tconn, mode == "tablet"); err != nil {
		s.Fatalf("Failed to set the DUT into %s mode: %s", mode, err)
	}

	if _, err := shortcutcustomization.Launch(ctx, tconn); err != nil {
		s.Log("Failed to open Settings page for shortcut: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Minute).WithInterval(5 * time.Second)
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Log("Failed to get keyboard: ", ctx)
	}
	defer kb.Close(cleanupCtx)

	// Add shortcut for open launcher.
	launcherContainer := nodewith.NameContaining("close Launcher").ClassName("edit-icon-container")
	launcherEditButton := nodewith.Role(role.Button).Ancestor(launcherContainer)
	addButton := nodewith.Name("Add shortcut").Role(role.Button)
	shortCut := "Ctrl+Shift+l"
	warnMsg := nodewith.NameContaining("Shortcut without launcher key might conflict").Role(role.StaticText).First()
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := uiauto.Combine("Add custom shortcut",
		ui.LeftClick(launcherEditButton),
		ui.LeftClick(addButton),
		kb.AccelAction(shortCut),
		ui.WaitUntilExists(warnMsg),
		kb.AccelAction(shortCut),
		ui.LeftClick(doneButton),
	)(ctx); err != nil {
		s.Log("Failed to add shortcut Ctrl+Shift+s to open launcher: ", err)
	}

	// Add shortcut for taking screenshots.
	screenshotContainer := nodewith.NameContaining("Edit button for Take partial screenshot or screen recording").ClassName("edit-icon-container")
	screenshotEditButton := nodewith.Role(role.Button).Ancestor(screenshotContainer)
	shortCut = "Ctrl+Shift+s"
	if err := uiauto.Combine("Add custom shortcut",
		ui.FocusAndWait(screenshotEditButton),
		ui.LeftClickUntil(screenshotEditButton, ui.WaitUntilExists(addButton)),
		ui.LeftClick(addButton),
		kb.AccelAction(shortCut),
		ui.WaitUntilExists(warnMsg),
		kb.AccelAction(shortCut),
		ui.LeftClick(doneButton),
	)(ctx); err != nil {
		s.Log("Failed to add shortcut Ctrl+Shift+s: ", err)
	}

	if err := crd.Launch(ctx, cr.Browser(), tconn); err != nil {
		s.Fatal("Failed to Launch: ", err)
	}

	s.Log("Waiting connection")
	if err := crd.WaitConnection(ctx, tconn); err != nil {
		s.Fatal("No client connected: ", err)
	}

	// Open the Settings page to show the shortcut.
	if _, err := shortcutcustomization.Launch(ctx, tconn); err != nil {
		s.Log("Failed to open Settings page for shortcut: ", err)
	}
}
