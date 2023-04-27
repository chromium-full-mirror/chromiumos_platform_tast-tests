// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package supporttool

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/local/uidetection"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const uiDetectionTimeout = 45 * time.Second
const defaultUser = "testuser@gmail.com"

type dataCollectionParam struct {
	browserType browser.Type
	// Cusomized Support Tool url which will contain optional case ID and
	// requested data collectors.
	url string
	// Support case ID.
	caseID string
	// List of requested data collector names.
	dataCollectors []string
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
		},
		SoftwareDeps: []string{"chrome"},
		LacrosStatus: testing.LacrosVariantExists,
		Params: []testing.Param{
			{
				Name: "all_data_collectors_lacros",
				Val: dataCollectionParam{
					browserType: browser.TypeLacros,
					url:         "chrome://support-tool/?case_id=test-case-id&module=CgUBAgMGDw",
					caseID:      "test-case-id",
					dataCollectors: []string{"Chrome System Information", "Crash IDs",
						"Memory Details", "Policies",
						"Device Event"},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "all_data_collectors_ash",
				Val: dataCollectionParam{
					browserType: browser.TypeAsh,
					url:         "chrome://support-tool/?case_id=test-case-id&module=Cg8BAgMEBQYHCAkKCwwNDg8",
					caseID:      "test-case-id",
					dataCollectors: []string{"Chrome System Information", "Crash IDs",
						"Memory Details", "Policies",
						"Device Event", "UI Hierarchy",
						"Additional Chrome OS Platform Logs",
						"Intel WiFi NICs Debug Dump",
						"Touch Events", "DBus Details",
						"Chrome OS Network Routes",
						"Chrome OS Shill (Connection Manager) Logs"},
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

	// Enable the support tool feature flag for both Lacros and Ash Chrome.
	opts := []chrome.Option{chrome.EnableFeatures("SupportTool")}
	lacrosConfig := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("SupportTool")))
	// Start Chrome with SupportTool flag.
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, param.browserType, lacrosConfig, opts...)
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	ud := uidetection.NewDefault(tconn)
	ui := uiauto.New(tconn)

	// Support Tool UI is implemented for the light-mode and dark-mode
	// is not supported yet. Some UI items might be hard to detect in
	// the dark-mode.
	// TODO(b/261156588): Remove this section when Support Tool UI
	// supports dark-mode.
	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui))(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	conn, err := br.NewConn(ctx, param.url)
	if err != nil {
		s.Fatal("Failed to open Support Tool page: ", err)
	}
	defer conn.Close()

	// Check the support case ID field.
	if err := ud.WithTimeout(uiDetectionTimeout).WaitUntilExists(
		uidetection.Word(param.caseID).First())(ctx); err != nil {
		s.Fatal("Failed to verify support case ID: ", err)
	}

	// Check the user's email field in UI.
	if err := ud.WithTimeout(uiDetectionTimeout).WaitUntilExists(
		uidetection.Word(defaultUser).First())(ctx); err != nil {
		s.Fatal("Failed to verify email: ", err)
	}

	if err := ui.LeftClick(nodewith.NameContaining("Continue").First())(ctx); err != nil {
		s.Fatal("Failed to click Continue button: ", err)
	}

	s.Log("Checking the checkboxes for requested data collectors")
	for _, dataCollector := range param.dataCollectors {
		checkbox := nodewith.NameContaining(dataCollector).Role(role.CheckBox)
		if err := ui.WaitUntilExists(checkbox.Attribute("checked", "true"))(ctx); err != nil {
			s.Fatalf("Failed to find checked checkbox for the data collector %s: %v", dataCollector, err)
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

	if err := ui.WaitUntilExists(nodewith.Role(role.CheckBox).First())(ctx); err != nil {
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
