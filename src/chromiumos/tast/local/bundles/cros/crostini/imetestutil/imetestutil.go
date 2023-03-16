// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imetestutil provides shared struct to test IME behaviour in Crostini.
package imetestutil

import (
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
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
