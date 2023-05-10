// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imetestutil provides shared struct to test IME behaviour in Crostini.
package imetestutil

import (
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
	"context"

	"go.chromium.org/tast/core/testing"
)

type enterInputActionPK func(keyboard *input.KeyboardEventWriter) uiauto.Action

// IMETestData represents a set of inputs for ime testing.
type IMETestData struct {
	InputMethod             ime.InputMethod
	InputText               string
	ExpectedText            string
	EnterTestStringActionPK enterInputActionPK
}

type imeTestDataMap map[string]IMETestData

// IMETestCases containers a list of IMEs to run the app tests against. Used in params_test.go to generate test params.
var IMETestCases = imeTestDataMap{
	"english": IMETestData{
		ime.EnglishUS,
		"Hello",
		"Hello",
		func(keyboard *input.KeyboardEventWriter) uiauto.Action {
			return keyboard.TypeAction("Hello")
		},
	},
	"japanese": IMETestData{
		ime.Japanese,
		"konnnitiha",
		"こんにちは",
		func(keyboard *input.KeyboardEventWriter) uiauto.Action {
			// TODO(b/274825850): Test the positioning of the suggestion box for Japanese input.
			return uiauto.Combine("Enter Japanese",
				keyboard.TypeAction("konnnitiha"),
				keyboard.AccelAction("Enter"),
			)
		},
	},
	"arabic": IMETestData{
		ime.Arabic,
		"lvpfh",
		"مرحبا",
		func(keyboard *input.KeyboardEventWriter) uiauto.Action {
			return keyboard.TypeAction("lvpfh")
		},
	},
}

// ResetToDefaultIME can be called to reset the input method back to default (EnglishUS) at the end of a test. If it fails, it will only log the error, and not return an error so the test will NOT fail.
func ResetToDefaultIME(ctx context.Context, tconn *chrome.TestConn) {
	if err := uiauto.Combine("reactivate default IME",
		ime.DefaultInputMethod.InstallAndActivate(tconn),
		ime.DefaultInputMethod.WaitUntilActivated(tconn),
	)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed switch back to default IME: ", err)
	}
}
