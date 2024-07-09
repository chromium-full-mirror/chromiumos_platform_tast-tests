// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           InlineReply,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verify inline reply for Chrome notification works",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Notifications
		BugComponent: "b:1246021",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-1f4894b3-96dd-44f1-a8c5-800eb7aedcbd",
		}},
		Fixture: "chromeLoggedIn",
	})
}

// InlineReply verifies that inline reply for Chrome notification works.
func InlineReply(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// TODO(crbug.com/1311030): Update this website with the notification test page when it is fully functional.
	conn, err := cr.NewConn(ctx, "https://tests.peter.sh/notification-generator")
	if err != nil {
		s.Fatal("Failed to open the web page: ", err)
	}
	defer conn.Close()
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := webutil.WaitForQuiescence(ctx, conn, 15*time.Second); err != nil {
		s.Fatal("Failed to wait for page to achieve quiescence: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	ui := uiauto.New(tconn)

	// The notification permission prompt will not appear automatically,
	// actively grant the notification permission for upcoming tests.
	permissionBtn := nodewith.Name("Request permission").Role(role.Button)
	if err := uiauto.Combine("request permission",
		ui.WaitUntilExists(permissionBtn),
		ui.MakeVisible(permissionBtn),
		ui.LeftClick(permissionBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to grant the notification permission: ", err)
	}

	if err := prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.ShowNotificationsPrompt)(ctx); err != nil {
		s.Fatal("Failed to clear notification prompt dialog: ", err)
	}

	selectSettingExpr := `
		var setting = document.getElementById("%s");
		var options = setting.options;
		var targetOptionText = "%s";
		Array.from(options).forEach(option => {
			if (option.text == targetOptionText) {
				setting.value = option.value;
				return;
			}
		});

		var currentOption = options[options.selectedIndex]
		if (currentOption.text != targetOptionText) {
			throw new Error("expected: " + targetOptionText + ", got: " + currentOption.text)
		}
	`
	// Select notification setting.
	if err := conn.Eval(ctx, fmt.Sprintf(selectSettingExpr, "actions", "One action (type text)"), nil); err != nil {
		s.Fatal("Failed to select notification setting: ", err)
	}
	// Select reaction setting.
	if err := conn.Eval(ctx, fmt.Sprintf(selectSettingExpr, "action", "Display an alert()."), nil); err != nil {
		s.Fatal("Failed to select reaction setting: ", err)
	}

	sendBtn := nodewith.Name("Display the notification").Role(role.Button)
	if err := uiauto.Combine("send notification",
		ui.WaitUntilExists(sendBtn), // This is to make sure that the node exists in the UI tree before calling MakeVisible on it.
		ui.MakeVisible(sendBtn),
		ui.LeftClick(sendBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to complete the combined steps: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	const replyMsg = "Test reply message"
	ashNotificationWindow := nodewith.HasClass("ash/message_center/MessagePopup").Role(role.Window)
	browserNotificationWindow := nodewith.Name("tests.peter.sh says").HasClass("JavaScriptTabModalDialogViewViews").Role(role.Window)
	if err := uiauto.Combine("send reply",
		ui.WaitUntilExists(ashNotificationWindow),
		ui.LeftClick(nodewith.NameRegex(regexp.MustCompile("(?i)reply")).Role(role.Button).Ancestor(ashNotificationWindow)),
		ui.EnsureFocused(nodewith.HasClass("Textfield").Role(role.TextField).Ancestor(ashNotificationWindow)),
		kb.TypeAction(replyMsg),
		kb.AccelAction("Enter"),
		ui.WaitUntilExists(nodewith.Name(fmt.Sprintf(`Clicked on "Notification title" (action: "0", reply: "%s")`, replyMsg)).Ancestor(browserNotificationWindow)),
	)(ctx); err != nil {
		s.Fatal("Failed to complete the combined steps: ", err)
	}
}
