// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package phonehub

import (
	"context"
	"regexp"
	"time"

	cdcommon "go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MessageNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Android message notifications appear in Phone Hub and that inline reply works",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"kyleshima@chromium.org",
		},
		BugComponent: "b:1131837",
		Attr:         []string{"group:cross-device", "cross-device_phonehub"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(cdcommon.UnstableModels...)),
			},
			{
				Name:              "unstable",
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cdcommon.UnstableModels...)),
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFlossRerun",
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
	})
}

// MessageNotification tests receiving message notifications in Phone Hub and replying to them.
func MessageNotification(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice

	// Reserve time for deferred cleanup functions.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Clear any notifications that are currently displayed.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to clear notifications")
	}

	// Generate a message notification on the Android device.
	title := "Hello!"
	text := "Notification test"
	waitForReply, err := androidDevice.GenerateMessageNotification(ctx, 1 /*id*/, title, text)
	if err != nil {
		s.Fatal("Failed to generate Android message notification: ", err)
	}

	// Wait for the notification on ChromeOS.
	n, err := ash.WaitForNotification(ctx, tconn, 10*time.Second, ash.WaitTitle(title))
	if err != nil {
		s.Fatal("Failed waiting for the message notification to appear on CrOS: ", err)
	}
	if n.Message != text {
		s.Fatalf("Notification text does not match: wanted %v, got %v", text, n.Message)
	}

	// Open Notification Center to make sure the notification is visible.
	if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
		s.Fatal("Failed to open Notification Center to check notifications: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Reply using the notification's inline reply field.
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to set up virtual keyboard: ", err)
	}
	defer kb.Close(ctx)
	replyText := "Goodbye!"
	ui := uiauto.New(tconn)
	if err := ui.LeftClick(nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)reply")))(ctx); err != nil {
		s.Fatal("Failed to click notification's reply button: ", err)
	}
	if err := kb.Type(ctx, replyText); err != nil {
		s.Fatal("Failed to type a reply in the notification: ", err)
	}
	if err := kb.AccelAction("Enter")(ctx); err != nil {
		s.Fatal("Failed to send reply by hitting Enter: ", err)
	}

	// Wait for the Android device to receive the reply and verify the text matches.
	received, err := waitForReply(ctx)
	if err != nil {
		s.Fatal("Failed waiting to receive a reply on the Android device: ", err)
	}

	if received != replyText {
		s.Fatalf("Reply received by the snippet does not match: wanted %v, got %v", replyText, received)
	}
}
