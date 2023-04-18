// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/inputs/fixture"
	"chromiumos/tast/local/bundles/cros/inputs/testserver"
	"chromiumos/tast/local/bundles/cros/inputs/util"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/useractions"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardLayoutChanged,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test of the layout changed event",
		Contacts: []string{
			"essential-inputs-gardener-oncall@google.com",
			"essential-inputs-team@google.com",
			"giovax@google.com",
		},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		Attr:         []string{},
		Timeout:      45 * time.Second,
		Params: []testing.Param{
			{
				Fixture: fixture.ClamshellNonVK,
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
	// TODO: This test is disabled as the main feature is still in development.
	// Change the Attr value to "{"group:mainline", "informational"}" once
	// https://crrev.com/c/3094212 is merged.
}

func PhysicalKeyboardLayoutChanged(ctx context.Context, s *testing.State) {
	fixtureData := s.FixtValue().(fixture.FixtData)
	testConn := fixtureData.TestAPIConn
	userContext := fixtureData.UserContext

	additionalLanguages := []ime.InputMethod{ime.Swedish, ime.FrenchFrance}
	for _, lang := range additionalLanguages {
		if err := lang.Install(testConn)(ctx); err != nil {
			s.Fatal("Failed to add additional IME: ", err)
		}
	}
	expected := "Keyboard Layout changed!"
	inputIdentifier := "textInputField"
	inputFinder := testserver.InputField(inputIdentifier).Finder()
	html :=
		`<html>
			<head>
				<meta charset="utf-8">
				<title>E14s test page</title>
			</head>
			<body>
				<input type="text" id="inputBox" aria-label="` + inputIdentifier + `">
				<script>
					var inputBox = document.getElementById("inputBox")
					navigator.keyboard.addEventListener(
							"layoutchange",
							() => inputBox.value = "` + expected + `")
				</script>
			</body>
		</html>`

	inputTestServer, err := testserver.LaunchBrowserWithHTML(
		ctx,
		fixtureData.BrowserType,
		false,
		fixtureData.Chrome,
		testConn,
		html)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer inputTestServer.CloseAll(ctx)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	if err := uiauto.UserAction("Change keyboard layout",
		uiauto.Combine("Change keyboard layout",
			util.WaitForFieldEmpty(testConn, inputFinder), // Verify initial conditions.
			keyboard.AccelAction("Ctrl+Space"),            // Switch keyboard layout.
			util.WaitForFieldTextToBe(testConn, inputFinder, expected),
		),
		userContext,
		&useractions.UserActionCfg{},
	)(ctx); err != nil {
		s.Fatal("Failed during change keyboard layout: ", err)
	}
}
