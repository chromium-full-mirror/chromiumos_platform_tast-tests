// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides fixtures for odml tast tests.
package fixture

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// List of fixture names for Coral
const (
	CoralEnabled         = "coralEnabled"
	CoralEnabledJapanese = "coralEnabledJapanese"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "coralEnabled",
		Desc: "A fixture with ARC booted and coral feature enabled",
		Contacts: []string{
			"hcyang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent:    "b:1445284",
		Impl:            arc.NewArcBootedFixture(featureConfigWithLanguage("en")),
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PreTestTimeout:  arc.PreTestTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "coralEnabledJapanese",
		Desc: "A fixture with ARC booted and coral feature enabled with Japanese locale",
		Contacts: []string{
			"hcyang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent:    "b:1445284",
		Impl:            arc.NewArcBootedFixture(featureConfigWithLanguage("ja")),
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PreTestTimeout:  arc.PreTestTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})
}

func featureConfigWithLanguage(language string) arc.BootedFixtureConfig {
	fixtureConfig := arc.DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		loginPool, err := gaiaLoginOption(ctx)
		if err != nil {
			s.Fatal("Failed to get login pool: ", err)
		}
		opts := []chrome.Option{
			chrome.EnableFeatures("CoralFeature"),
			chrome.EnableFeatures("CoralFeatureMultiLanguage"),
			loginPool,
			// Only some specific locales have Coral feature enabled. Manually set them in the test since our accounts come from a login pool.
			chrome.ExtraArgs("--lang=" + language)}
		return opts, nil
	}
	return fixtureConfig
}

func gaiaLoginOption(ctx context.Context) (chrome.Option, error) {
	const (
		// TODO(crbug.com/404376751): Use the dedicated account pool with DMA consent enabled once available.
		pltpBaseURL = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/account"
		pltuURL     = pltpBaseURL + "/pltu_rand"
		pltpURL     = pltpBaseURL + "/pltp_rand"
	)

	usernames, err := utils.FetchFromURL(ctx, pltuURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch usernames")
	}
	password, err := utils.FetchFromURL(ctx, pltpURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch password")
	}

	names := strings.Split(usernames, "\n")
	password = strings.TrimSuffix(password, "\n")
	var loginPool strings.Builder
	// loginPool is a string containing multiple credentials separated by newlines:
	//
	// user1:pass1
	// user2:pass2
	// user3:pass3
	for _, n := range names {
		fmt.Fprintf(&loginPool, "%s:%s\n", n, password)
	}

	return chrome.GAIALoginPool(loginPool.String()), nil
}
