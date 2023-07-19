// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package data

import (
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
)

// LongestInputLength is used to clean the env between sub tests.
const LongestInputLength = 10

// AppCompatPhysicalKeyboardTestCases is common test cases for appcompat inputs testing.
// Adding more test cases here if we need to cover more latin languages.
var AppCompatPhysicalKeyboardTestCases = map[ime.InputMethod][]InputData{
	ime.FrenchFrance: {
		{
			Description:    "typing number key from 0 to 2",
			LocationKeySeq: strings.Split("h0h1h2", ""),
			ExpectedText:   "hàh&hé",
		},
		{
			Description:    "typing number key 3",
			LocationKeySeq: strings.Split("3double3", ""),
			ExpectedText:   `"double"`,
		},
		{
			Description:    "typing number key 4",
			LocationKeySeq: strings.Split("4single4", ""),
			ExpectedText:   "'single'",
		},
		{
			Description:    "typing number key 5",
			LocationKeySeq: strings.Split("5hello-", ""),
			ExpectedText:   `(hello)`,
		},
		{
			Description:    "typing number key 6 and 7",
			LocationKeySeq: strings.Split("b6h7", ""),
			ExpectedText:   `b-hè`,
		},
		{
			Description:    "typing number key 8 and 9",
			LocationKeySeq: strings.Split("h8h9h", ""),
			ExpectedText:   `h_hçh`,
		},
		{
			Description:    "typing dead key [ on keyboard",
			LocationKeySeq: strings.Split("qo[ut", ""),
			ExpectedText:   "août",
		},
	},
	ime.EnglishUSWithInternationalKeyboard: {
		{
			Description:    "typing dead key ' on keyboard",
			LocationKeySeq: strings.Split("'abc", ""),
			ExpectedText:   "ábc",
		},
	},
}
