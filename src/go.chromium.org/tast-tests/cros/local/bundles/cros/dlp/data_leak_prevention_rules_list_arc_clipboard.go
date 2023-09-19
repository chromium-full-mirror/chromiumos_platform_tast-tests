// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	policyBlob "go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListArcClipboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with clipboard blocked restriction on ARC",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"chromeos-dlp@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Attr:         []string{"group:golden_tier"},
		Data:         []string{"text_1.html", "text_2.html", "editable_text_box.html"},
		Fixture:      "fakeDMS",
		Timeout:      4 * time.Minute,
	},
	)
}

func DataLeakPreventionRulesListArcClipboard(ctx context.Context, s *testing.State) {
	fakeDMS := s.FixtValue().(*fakedms.FakeDMS)

	sourceServer := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer sourceServer.Close()

	path := "/text_1.html"
	// Update the policy blob.
	pb := policyBlob.NewBlob()
	pb.AddPolicies(policy.PopulateDLPPolicyBlockClipboardArc(sourceServer.URL + path))
	if err := fakeDMS.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	// Policies are only updated after Chrome startup.
	cr, err := chrome.New(ctx,
		chrome.ARCEnabled(),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fakeDMS.URL))
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	const (
		// Simple app with text field.
		apk          = "ArcKeyboardTest.apk"
		pkg          = "org.chromium.arc.testapp.keyboard"
		activityName = ".MainActivity"
		// Used to identify text field in the app.
		fieldID = pkg + ":id/text"
	)

	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed to install the app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, activityName)
	if err != nil {
		s.Fatalf("Failed to create a new activity %q", activityName)
	}
	defer act.Close(ctx)

	handler := faillog.DumpUITreeWithScreenshotHandler(ctx, tconn, "ui_tree_error")
	s.AttachErrorHandlers(handler, handler)

	conn, err := cr.NewConn(ctx, sourceServer.URL+path)
	if err != nil {
		s.Error("Failed to open page: ", err)
	}
	defer conn.Close()

	if err := kb.Accel(ctx, "Ctrl+A"); err != nil {
		s.Fatal("Failed to press Ctrl+A to select all content: ", err)
	}

	if err := kb.Accel(ctx, "Ctrl+C"); err != nil {
		s.Fatal("Failed to press Ctrl+C to copy all content: ", err)
	}

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatalf("Failed to start the activity %q due to error: %v", activityName, err)
	}
	defer act.Stop(ctx, tconn)

	// Focus the input field and paste the text.
	if err := d.Object(ui.ID(fieldID)).WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to find the input field: ", err)
	}
	if err := d.Object(ui.ID(fieldID)).Click(ctx); err != nil {
		s.Fatal("Failed to click the input field: ", err)
	}
	if err := d.Object(ui.ID(fieldID), ui.Focused(true)).WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to focus on the input field: ", err)
	}
	// Set the initial state of the input.
	if err := d.Object(ui.ID(fieldID), ui.Focused(true)).SetText(ctx, ""); err != nil {
		s.Fatal("Failed to empty field: ", err)
	}
	if err := kb.Accel(ctx, "Ctrl+V"); err != nil {
		s.Fatal("Failed to press Ctrl+V to paste all content: ", err)
	}

	var copiedString string
	copiedString, err = d.Object(ui.ID(fieldID), ui.Focused(true)).GetText(ctx)
	if err != nil {
		s.Fatal("Failed to get input field text: ", err)
	}

	if copiedString != "" {
		s.Fatalf("String copied: got %q, want none", copiedString)
	}
}
