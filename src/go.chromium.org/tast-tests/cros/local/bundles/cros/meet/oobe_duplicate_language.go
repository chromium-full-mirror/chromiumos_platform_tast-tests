// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OOBEDuplicateLanguage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that verifies there are no duplicate languages in the language selection menu",
		Contacts: []string{
			"core-devices@google.com",
			"joshuapius@google.com", // Test author
		},
		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
	})
}

func OOBEDuplicateLanguage(ctx context.Context, s *testing.State) {
	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging", "--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin())
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to wait for OOBE connection: ", err)
	}
	defer conn.Close()

	if err := conn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the Welcome screen to be visible: ", err)
	}

	var langArr []string
	const f = `Array.from(
			document.querySelector("#connect")
			.shadowRoot.querySelector("#languageSelect")
			.shadowRoot.querySelector("#select").options)
			.map((e) => e.value + " || " + e.text)
			`

	if err := conn.Eval(ctx, f, &langArr); err != nil {
		s.Fatal("Failed to get languages: ", err)
	}

	var langCodeMap = make(map[string]string)
	var langFullMap = make(map[string]string)

	var errLog []string

	for _, lang := range langArr {
		langSlice := strings.Split(lang, " || ")
		langCode := langSlice[0]
		langFull := langSlice[1]

		dupLangFull, dupFullFound := langCodeMap[langCode]
		dupLangCode, dupCodeFound := langFullMap[langFull]
		if dupFullFound {
			errMsg := fmt.Sprintf(`Duplicate entry for language code "%s" found. ("%s", "%s")`, langCode, langFull, dupLangFull)
			errLog = append(errLog, errMsg)
		} else if dupCodeFound {
			errMsg := fmt.Sprintf(`Duplicate entry for language name "%s" found. ("%s", "%s")`, langFull, langCode, dupLangCode)
			errLog = append(errLog, errMsg)
		}

		langCodeMap[langCode] = langFull
		langFullMap[langFull] = langCode
	}

	if len(errLog) > 0 {
		errMsg := strings.Join(errLog, "\n")
		s.Fatal("Test encountered the following duplicate languages: ", errMsg)
	}
}
