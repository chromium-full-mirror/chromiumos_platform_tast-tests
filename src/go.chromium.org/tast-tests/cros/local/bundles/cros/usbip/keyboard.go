// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/usbip"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Keyboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify keyboard can emulate input and still works after reattaching",
		Contacts: []string{
			"chromeos-engprod-syd@google.com",
			"ashpakov@google.com",
		},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Developer
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Fixture:      "usbipServer",
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
	})
}

// Keyboard verifies keyboard can emulate input and still works after reattaching.
func Keyboard(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	server := s.FixtValue().(*usbip.ServerFixtureData).Server
	k := usbdevice.NewUsbKeyboard(ctx)
	attach := server.AddDevice(k)

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "keyboard")

	var detach usbip.DetachFn
	testText := "This is a test message."
	promptText := "crosh>"
	window := nodewith.Name("crosh").Role(role.Window).ClassName("BrowserFrame")
	prompt := nodewith.Ancestor(window).Name(promptText).Role(role.StaticText).First()
	ui := uiauto.New(tconn)
	if err := uiauto.Repeat(2, uiauto.Combine("Open launcher and type text",
		func(ctx context.Context) error { return apps.Launch(ctx, tconn, apps.Crosh.ID) },
		ui.WaitUntilExists(prompt),
		ui.LeftClick(window),
		func(ctx context.Context) error {
			var err error
			detach, err = attach(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to attach a virtual keyboard")
			}
			k.Type(testText)
			return nil
		},
		ui.WaitUntilExists(nodewith.Ancestor(window).Name(fmt.Sprintf("%s %s", promptText, testText)).Role(role.StaticText).First()),
		// Close crosh.
		kb.AccelAction("Enter"),
		kb.TypeAction("exit"),
		kb.AccelAction("Enter"),
		func(ctx context.Context) error {
			// Need to wrap detach into a function, because its value is changed as a part of uiauto.Combine execution.
			// Otherwise uiauto.Combine would capture original value(nil) and fail.
			return detach(ctx)
		},
	))(ctx); err != nil {
		s.Fatal("Unable to attach the keyboard, type text, detach the keyboard (performed twice): ", err)
	}
}
