// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

// List of ARC++ fixture names for video conferencing testing.
const (
	gaiaLoggedInARC = "gaiaLoggedInARCForVideoConferencing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedInARC,
		Desc: "A fixture with GAIA user logged in and ARC booted",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: arc.NewArcBootedWithPlayStoreFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := []chrome.Option{
				chrome.EnableFeatures("SpeakOnMuteEnabled"),
				chrome.ExtraArgs(arc.DisableSyncFlags()...),
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))}
			opts = append(opts, vcOpts...)
			return opts, nil
		}),
		Parent:          fixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})
}
