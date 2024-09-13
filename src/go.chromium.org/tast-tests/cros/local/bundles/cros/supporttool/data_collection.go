// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package supporttool

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const uiDetectionTimeout = 45 * time.Second
const defaultUser = "testuser@gmail.com"

// A pattern to match detected PII UI fields. We use Regex pattern because
// each data collector set and device may different PII data on the logs.
// See chrome://support-tool UI for reference.
const piiItemRegexPattern = `(.+) (\d+)`

type dataCollectionParam struct {
	// Cusomized Support Tool url which will contain optional case ID and
	// requested data collectors.
	url string
	// Support case ID.
	caseID string
	// List of requested data collector names.
	dataCollectorNamePatterns []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DataCollection,
		Desc: "Verifies that chrome://support-tool collects requested logs",
		Contacts: []string{
			"chromeos-commercial-supportability@google.com", // Team
			"iremuguz@google.com",                           // Test author
		},
		BugComponent: "b:1111615",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "all_data_collectors",
				Val: dataCollectionParam{
					url:    "chrome://support-tool/?case_id=test-case-id&module=Cg8BAgMEBQYHCAkKCwwNDg8",
					caseID: "test-case-id",
					dataCollectorNamePatterns: []string{`Chrome System Information`, `Crash IDs`,
						`Memory Details`, `Policies`,
						`Device Event`, `UI Hierarchy`,
						// We use regex pattern to match both ChromeOS and Chrome OS since they tend to differ according to version.
						`Additional Chrome(?:\s|)OS Platform Logs`,
						`Intel WiFi NICs Debug Dump`,
						`Touch Events`, `DBus Details`,
						`Chrome(?:\s|)OS Network Routes`,
						`Chrome(?:\s|)OS Shill \(Connection Manager\) Logs`},
				},
			},
		},
	})
}

func DataCollection(ctx context.Context, s *testing.State) {
	param := s.Param().(dataCollectionParam)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("SupportTool"))
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_dump")
	s.AttachErrorHandlers(handler, handler)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	ui := uiauto.New(tconn)

	conn, err := cr.NewConn(ctx, param.url)
	if err != nil {
		s.Fatal("Failed to open Support Tool page: ", err)
	}
	defer conn.Close()

	s.Log("Checking the case ID and email address on issue details page")
	// Check the support case ID field.
	if err := ui.WaitUntilExists(nodewith.NameContaining(param.caseID).First())(ctx); err != nil {
		s.Fatal("Failed to verify support case ID: ", err)
	}

	// Check the user's email field in UI.
	if err := ui.WaitUntilExists(nodewith.Attribute("value", defaultUser).NameContaining("Email").First())(ctx); err != nil {
		s.Fatal("Failed to verify email: ", err)
	}

	if err := ui.LeftClick(nodewith.NameContaining("Continue").First())(ctx); err != nil {
		s.Fatal("Failed to click Continue button: ", err)
	}

	s.Log("Checking the checkboxes for requested data collectors")
	for _, dataCollectorNamePattern := range param.dataCollectorNamePatterns {
		r, _ := regexp.Compile(dataCollectorNamePattern)
		checkbox := nodewith.NameRegex(r).Role(role.CheckBox).First()
		if err := ui.WaitUntilExists(checkbox.Attribute("checked", "true"))(ctx); err != nil {
			s.Fatalf("Failed to find checked checkbox for the data collector %s: %v", dataCollectorNamePattern, err)
		}
	}

	buttonNode := nodewith.Name("Continue").Role(role.Button)
	if err := uiauto.Combine("click continue button",
		ui.MakeVisible(buttonNode),
		ui.LeftClick(buttonNode),
	)(ctx); err != nil {
		s.Fatal("Failed to click continue button: ", err)
	}

	piiOption := nodewith.NameContaining("Manually select personal information you want to include").First()
	if err := ui.LeftClick(piiOption)(ctx); err != nil {
		s.Fatal("Failed to click manual PII removal option button: ", err)
	}

	detectedPiiRegex, _ := regexp.Compile(piiItemRegexPattern)
	if err := ui.WaitUntilExists(nodewith.NameRegex(detectedPiiRegex).First())(ctx); err != nil {
		s.Fatal("Failed to find any checkbox for detected PII category: ", err)
	}

	piiOption = nodewith.NameContaining("Automatically remove most personal information").First()
	if err := ui.LeftClick(piiOption)(ctx); err != nil {
		s.Fatal("Failed to click all PII removal option button: ", err)
	}

	if err := ui.LeftClick(nodewith.NameContaining("Export").Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to click Export button: ", err)
	}

	// Support Tool will try to name the exported file as "support_packet_<case id>_<timestamp>.zip",
	// change it to "support_packet.zip" before saving.
	filenameNode := nodewith.NameContaining("support_packet_" + param.caseID).First()
	if err := uiauto.Combine("change exported file name",
		ui.LeftClick(filenameNode),
		keyboard.AccelAction("Ctrl+A"),
		keyboard.AccelAction("Backspace"),
		keyboard.TypeAction("support_packet"),
	)(ctx); err != nil {
		s.Fatal("Failed to change filename: ", err)
	}

	if err := ui.LeftClick(nodewith.NameContaining("Save").Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to click Save button: ", err)
	}

	if err := ui.WithTimeout(uiDetectionTimeout).WaitUntilExists(
		nodewith.Name("support_packet.zip").First())(ctx); err != nil {
		s.Fatal("Failed to verify the file exported message on the UI: ", err)
	}

	// Support Tool will export the file into user's Download's directory.
	cryptohomeUserPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatalf("Failed to get the cryptohome user path for %s: %v", cr.NormalizedUser(), err)
	}
	path := filepath.Join(cryptohomeUserPath, "MyFiles", "Downloads", "support_packet.zip")
	if fileInfo, err := os.Stat(path); err != nil {
		s.Fatal("Failed to verify that the exported file exists in the filesystem: ", err)
	} else if fileInfo.Size() == 0 {
		s.Fatal("The exported support packet file is empty")
	}
}
