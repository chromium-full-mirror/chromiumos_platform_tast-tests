// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

// UIStopped is a ParameterizedFixture which stops the UI.
type UIStopped struct {
	// Name of the parent fixture.
	Parent string
}

var _ ParameterizedFixture = UIStopped{}

// Instance returns the instance name of the parameterized UIStopped fixture.
func (pf UIStopped) Instance() string {
	return maybeRegisterFixture(&testing.Fixture{
		Name:            maybeWithParentName("uiStopped", pf.Parent),
		Desc:            "Stops the UI",
		Contacts:        []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		Impl:            uiStoppedFixture{},
		BugComponent:    "b:776546",
		Parent:          pf.Parent,
		SetUpTimeout:    upstart.UIRestartTimeout,
		TearDownTimeout: upstart.UIRestartTimeout,
	})
}

type uiStoppedFixture struct{}

func (uiStoppedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Cannot stop ui: ", err)
	}
	return s.ParentValue()
}

func (uiStoppedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := upstart.EnsureJobRunning(ctx, "ui"); err != nil {
		s.Fatal("Cannot start ui: ", err)
	}
}

func (uiStoppedFixture) Reset(ctx context.Context) error {
	return nil
}

func (uiStoppedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (uiStoppedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

var parameterizedChromeStartedID int

// ChromeStarted is a ParameterizedFixture which starts Chrome with the given config.
type parameterizedChrome struct {
	opts []chrome.Option
}

var _ ParameterizedFixture = parameterizedChrome{}

func (pf parameterizedChrome) Instance() string {
	parameterizedChromeStartedID++
	return maybeRegisterFixture(&testing.Fixture{
		Name:         fmt.Sprintf("parameterizedChromeStarted%d", parameterizedChromeStartedID),
		Desc:         "Starts Chrome with the given config",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return pf.opts, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// Chrome returns the name of the chrome.LoggedInFixture to start Chrome with the given options.
func Chrome(opts ...chrome.Option) string {
	return parameterizedChrome{opts: opts}.Instance()
}

// CrasFeature references a feature defined in cras's features.inc.
type CrasFeature string

// CrasFeature definitions.
const (
	APNoiseCancellation CrasFeature = "CrOSLateBootAudioAPNoiseCancellation"
	StyleTransfer       CrasFeature = "CrOSLateBootAudioStyleTransfer"
)

func (f CrasFeature) shortNameForFixture() string {
	return strings.TrimPrefix(strings.TrimPrefix(string(f), "CrOSLateBoot"), "Audio")
}

// CrasFeatureOverrides is a set of features to override.
type CrasFeatureOverrides map[CrasFeature]bool

// groups return features to enable and disable.
func (cfos CrasFeatureOverrides) groups() (enable, disable []CrasFeature) {
	for feature, enabled := range cfos {
		if enabled {
			enable = append(enable, feature)
		} else {
			disable = append(disable, feature)
		}
	}
	slices.Sort(enable)
	slices.Sort(disable)
	return
}

// ChromeForCras is a ParameterizedFixture for setting up Chrome with well-known configs that
// are related to testing CRAS.
//
// ChromeForCras is preferred to using Chrome() inline because
// ChromeForCras avoids generating multiple fixtures, while Chrome() generates
// a new fixture for each call.
type ChromeForCras struct {
	CrasFeatures CrasFeatureOverrides
}

var _ ParameterizedFixture = ChromeForCras{}

// Instance implements ParameterizedFixture.
func (pf ChromeForCras) Instance() string {
	opts := []chrome.Option{
		chrome.GuestLogin(),
		chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
	}
	var name strings.Builder
	name.WriteString("chromeForCras")

	enableFeatures, disableFeatures := pf.CrasFeatures.groups()
	for _, f := range enableFeatures {
		opts = append(opts, chrome.EnableFeatures(string(f)))
		name.WriteString("Enable" + f.shortNameForFixture())
	}
	for _, f := range disableFeatures {
		opts = append(opts, chrome.DisableFeatures(string(f)))
		name.WriteString("Disable" + f.shortNameForFixture())
	}

	return maybeRegisterFixture(&testing.Fixture{
		Name:         name.String(),
		Desc:         "Starts Chrome with the given config",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return opts, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
