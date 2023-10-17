// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package terminal has tests for Terminal SSH System App.
package terminal

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Crosh,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify crosh System Web App",
		Contacts: []string{
			"chrome-hterm@google.com",
			"joelhockey@chromium.org",
		},
		BugComponent: "b:1122570",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
	})
}

func Crosh(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanup, err := terminalapp.DontModifyPS1(ctx)
	if err != nil {
		s.Fatal("Failed to set DontModifyPS1: ", err)
	}
	defer cleanup()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("TerminalAlternativeEmulator"))
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Get Test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Remove public IME if it is installed (seems to be only amd64-generic
	// boards) since it sends bad text and key events.
	const publicImeExt = "fgoepimhcoialccpbmpnnblemnepkkao"
	if err := tconn.Call(ctx, nil,
		"tast.promisify(chrome.autotestPrivate.removeComponentExtension)", publicImeExt); err != nil {
		s.Fatal("Failed to remove public IME extension: ", err)
	}

	// Launch Crosh.
	if err := apps.Launch(ctx, tconn, apps.Crosh.ID); err != nil {
		s.Fatal("Failed to launch: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	croshPrompt := nodewith.NameRegex(regexp.MustCompile(`^crosh> ?$`)).Role(role.StaticText).First()

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")
	// Run shell, verify prompt, exit.
	ui := uiauto.New(tconn)
	err = uiauto.Combine("run crosh shell",
		ui.LeftClick(nodewith.Name("crosh").Role(role.Window).ClassName("BrowserFrame")),
		ui.WaitUntilExists(croshPrompt),
		kb.TypeAction("shell"),
		kb.AccelAction("Enter"),
		ui.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(`^chronos@localhost / \$ ?$`)).Role(role.StaticText).First()),
		kb.TypeAction("exit"),
		kb.AccelAction("Enter"),
		kb.TypeAction("exit"),
		kb.AccelAction("Enter"),
		ui.WaitUntilGone(croshPrompt),
	)(ctx)
	if err != nil {
		s.Fatal("Failed: ", err)
	}
}
