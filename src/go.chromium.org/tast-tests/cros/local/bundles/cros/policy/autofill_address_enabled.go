// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	autofillQueryAnnotationHash  = "88863520"  // autofill_query
	autofillUploadAnnotationHash = "104798869" // autofill_upload
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutofillAddressEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of AutofillAddressEnabled policy, checking the correspoding toggle button states (restriction and checked) after setting the policy",
		Contacts: []string{
			"chrome-autofill@google.com", // Feature owner
		},
		BugComponent: "crbug:UI>Browser>Autofill",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.FakeDMS,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.PersistentLacros, // FakeDMS with lacros policy.
			Val:               browser.TypeLacros,
		}},
		Data: []string{"autofill_address_enabled.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AutofillAddressEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func AutofillAddressEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	opts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL)}
	// Add lacros opts for lacros runs.
	if s.Param().(browser.Type) == browser.TypeLacros {
		lacrosOpts, err := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute lacros chrome options: ", err)
		}
		opts = lacrosOpts
	}

	addressValues := []struct {
		// The field's name on the settings page.
		fieldName string
		// The value which is set on the settings page and which should have been filled into the html input field after the autofill has been triggered.
		fieldValue string
		// The field's corresponding id on autofill_address_enabled.html
		htmlFieldID string
	}{
		{
			fieldName:   "Name",
			fieldValue:  "Tester",
			htmlFieldID: "name",
		},
		{
			fieldName:   "Street address",
			fieldValue:  "Some address 123",
			htmlFieldID: "street-address",
		},
		{
			fieldName:   "City",
			fieldValue:  "City",
			htmlFieldID: "city",
		},
		{
			fieldName:   "ZIP code",
			fieldValue:  "11111",
			htmlFieldID: "postal-code",
		},
		{
			fieldName:   "Phone",
			fieldValue:  "0441231234",
			htmlFieldID: "phone",
		},
		{
			fieldName:   "Email",
			fieldValue:  "test@gmail.com",
			htmlFieldID: "email",
		},
	}

	for _, param := range []struct {
		name                 string
		wantRestriction      restriction.Restriction
		wantChecked          checked.Checked
		shouldFindAnnotation bool
		policy               *policy.AutofillAddressEnabled
	}{
		{
			name:                 "unset",
			wantRestriction:      restriction.None,
			wantChecked:          checked.True,
			shouldFindAnnotation: true,
			policy:               &policy.AutofillAddressEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                 "allow",
			wantRestriction:      restriction.None,
			wantChecked:          checked.True,
			shouldFindAnnotation: true,
			policy:               &policy.AutofillAddressEnabled{Val: true},
		},
		{
			name:                 "deny",
			wantRestriction:      restriction.Disabled,
			wantChecked:          checked.False,
			shouldFindAnnotation: false,
			policy:               &policy.AutofillAddressEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Login to chrome. We create a new Chrome session for each policy value
			// in order to reliably trigger autofill upload/upvote. Otherwise, upload
			// may not occur every time due to caching.
			cr, err := chrome.New(ctx, opts...)
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}
			defer cr.Close(ctx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create Test API connection: ", err)
			}

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := policyutil.SettingsPage(ctx, cr, br, "addresses").
				SelectNode(ctx, nodewith.
					Name("Save and fill addresses").
					Role(role.ToggleButton)).
				Restriction(param.wantRestriction).
				Checked(param.wantChecked).
				Verify(); err != nil {
				s.Error("Unexpected settings state: ", err)
			}

			if param.wantChecked == checked.True {
				ui := uiauto.New(tconn)

				if err := uiauto.Combine("open the add address dialog",
					ui.DoDefault(nodewith.Name("Add address").Role(role.Button)),
					ui.WaitUntilExists(nodewith.Name("Save").Role(role.Button)),
				)(ctx); err != nil {
					s.Fatal("Failed to open the add address dialog: ", err)
				}

				kb, err := input.Keyboard(ctx)
				if err != nil {
					s.Fatal(errors.Wrap(err, "failed to get the keyboard"))
				}
				defer kb.Close(ctx)

				// Fill in the address input fields and click on the save button.
				for _, address := range addressValues {
					addressField := nodewith.Role(role.TextField).Name(address.fieldName)
					if err := uiauto.Combine("fill in address text field",
						ui.MakeVisible(addressField),
						ui.FocusAndWait(addressField),
					)(ctx); err != nil {
						s.Fatal("Failed to click the text field: ", err)
					}
					if err := kb.Type(ctx, address.fieldValue); err != nil {
						s.Fatal("Failed to type to the text field: ", err)
					}
				}
				if err := ui.DoDefault(nodewith.Role(role.Button).Name("Save"))(ctx); err != nil {
					s.Fatal("Failed to click the Save button: ", err)
				}

				// Open the website with the address form.
				conn, err := br.NewConn(ctx, server.URL+"/"+"autofill_address_enabled.html")
				if err != nil {
					s.Fatal("Failed to open website: ", err)
				}
				defer conn.Close()

				// Trigger the autofill by clicking the email field and choosing the suggested address (this could be any of the address fields).
				suggestionPopup := nodewith.Role(role.ListBoxOption).ClassName("PopupCellView").First()
				emailTextBox := nodewith.Role(role.TextField).Name("Email")
				if err := uiauto.Combine("clicking the Email field and choosing the suggested address",
					ui.WaitUntilExists(nodewith.Name("OK").Role(role.Button).ClassName("test-target-button")),
					ui.MakeVisible(emailTextBox),
					ui.DoDefault(emailTextBox),
					ui.WithTimeout(45*time.Second).WaitUntilExists(suggestionPopup),
					ui.DoDefaultUntil(suggestionPopup, ui.Exists(nodewith.Role(role.InlineTextBox).Name(addressValues[1].fieldValue))),
				)(ctx); err != nil {
					s.Fatal("Failed to trigger and use address autofill: ", err)
				}

				// Run JavaScript checks to confirm that all the address fields are set correctly.
				for _, address := range addressValues {
					var valueFromHTML string
					if err := conn.Eval(ctx, "document.getElementById('"+address.htmlFieldID+"').value", &valueFromHTML); err != nil {
						s.Fatal("Failed to complete the JS test for htmlFieldID="+address.htmlFieldID, err)
					}
					if valueFromHTML != address.fieldValue {
						s.Fatal("Address was not set properly. Actual value " + valueFromHTML + " doesnt match with expected " + address.fieldValue)
					}
				}

				// Submit the form to trigger autofill upload/upvote.
				submitButton := nodewith.Name("OK").Role(role.Button)
				if err := ui.DoDefault(submitButton)(ctx); err != nil {
					s.Fatal("Failed to submit form: ", err)
				}
			}

			hashCodes := []string{autofillQueryAnnotationHash, autofillUploadAnnotationHash}
			foundAnnotations, err := netExport.FindMultipleAnnotationsUntil(ctx, hashCodes,
				&testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second})
			if err != nil {
				s.Fatal("Failed to poll hashcode in log: ", err)
			}

			for _, annotationID := range hashCodes {
				if _, exists := foundAnnotations[annotationID]; exists != param.shouldFindAnnotation {
					s.Errorf("Unexpected status of annotation = %s, got %t, want %t", annotationID, exists, param.shouldFindAnnotation)
				}
			}
		})
	}
}
