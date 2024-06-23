// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CfmGAIAEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that clicks through CFM OOBE to enrollment screen and verifies device enterprise enrollment",
		Contacts: []string{
			"core-devices@google.com",
			"joshuapius@google.com", // Test author
		},
		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
		Vars: []string{
			"meet.CfmGAIAEnrollment.user",     // GAIA username.
			"meet.CfmGAIAEnrollment.password", // GAIA password.
		},
	})
}

func CfmGAIAEnrollment(ctx context.Context, s *testing.State) {
	// Set default tags for a fresh ash-chrome instance (cr).
	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	cfmUser := s.RequiredVar("meet.CfmGAIAEnrollment.user")
	cfmPassword := s.RequiredVar("meet.CfmGAIAEnrollment.password")

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging", "--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin(),
		chrome.GAIAEnterpriseEnroll(chrome.Creds{User: cfmUser, Pass: cfmPassword}))

	opts = append(opts, chrome.DontSkipOOBEAfterLogin(),
		chrome.RemoveNotification(false),
		chrome.DontWaitForCryptohome())

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	oobeConn, err := cr.WaitForCFMConnection(ctx)
	if err != nil {
		s.Fatal("Failed to load CFM OOBE connection: ", err)
	}
	defer oobeConn.Close()
}
