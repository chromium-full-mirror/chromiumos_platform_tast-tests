// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/arc"
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
			return []chrome.Option{
				chrome.EnableFeatures("SpeakOnMuteEnabled"),
				chrome.EnableFeatures("VideoConference"),
				chrome.EnableFeatures("CrOSLateBootAudioFlexibleLoopback"),
				chrome.EnableFeatures("SystemLiveCaption"),
				chrome.EnableFeatures("FeatureManagementVideoConference"),
				chrome.ExtraArgs("--disable-sync"),
				chrome.ExtraArgs(arc.DisableSyncFlags()...),
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})
}
