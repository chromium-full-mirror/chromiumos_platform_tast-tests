// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains fixtures used by XDR tests.
package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

const (
	// LoggedInWithFileEventsEnabled logged in user with the File event feature enabled.
	LoggedInWithFileEventsEnabled = "loggedInWithFileEventsEnabled"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LoggedInWithFileEventsEnabled,
		Desc: "Logged in with file events enabled",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"jasonling@google.com",
		},
		BugComponent: "b:1208373",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.EnableFeatures("CrOSLateBootSecagentdXDRFileEvents")}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
}
