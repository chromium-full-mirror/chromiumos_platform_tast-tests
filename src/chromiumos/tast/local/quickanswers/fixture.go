// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/testing"
)

const (
	// LacrosFixture is a fixture of a Lacros Chrome session with a GAIA.
	LacrosFixture = "quickAnswersLoggedInFixtureLacros"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LacrosFixture,
		Desc: "Lacros Chrome session logged in with OTA for Quick answers testing",
		Contacts: []string{
			"assistive-eng@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			}
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
