// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixtures

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedIn,
		Desc:     "Fixture for a running FakeDMS with lacros",
		Contacts: []string{"mohamedaomar@google.com", "wtlee@chromium.org", "chromeos-commercial-remote-management@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig().Opts()
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	// LacrosPolicyLoggedInWithKeepAlive is similar to LacrosPolicyLoggedIn, but sets Lacros keep-alive.
	// Do not use this unless you are explicitly testing the keep-alive feature or features that depend on it.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedInWithKeepAlive,
		Desc:     "Fixture for a running FakeDMS with lacros with KeepAlive",
		Contacts: []string{"mohamedaomar@google.com", "wtlee@chromium.org", "chromeos-commercial-remote-management@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.KeepAlive(true)).Opts()
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	// TODO(b/218907052): Remove fixture after Journeys flag  is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedInFeatureJourneys,
		Desc:     "Fixture for a running FakeDMS with lacros and enabling the feature flag Journeys",
		Contacts: []string{"rodmartin@google.com", "chromeos-commercial-remote-management@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("Journeys"))).Opts()
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name: fixture.LacrosPolicyLoggedInFeatureChromeLabs,
		Desc: "Fixture for a running FakeDMS with lacros and enabling the feature flag Chrome Labs",
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("ChromeLabs"))).Opts()
			},
		},
		Contacts:        []string{"samicolon@google.com", "chromeos-commercial-remote-management@google.com"},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            fixture.LacrosPolicyLoggedInRealUser,
		Desc:            "Fixture for a running FakeDMS with lacros with a real managed user logged on",
		Contacts:        []string{"anastasiian@chromium.org", "chromeos-commercial-remote-management@google.com"},
		Impl:            &policyRealUserFixture{},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
		Vars:            []string{"policy.ManagedUser.accountPool"},
	})

	// LacrosPolicyRealUserLoggedIn is similar to LacrosPolicyLoggedInRealUser, but instead starts up a Chrome instance
	// and has an Ash equivalent: ChromePolicyRealUserLoggedIn.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyRealUserLoggedIn,
		Desc:     "Fixture for running FakeDMS with lacros with a real managed user logged on",
		Contacts: []string{"chiav@google.com", "dp-chromeos-eng@google.com"},
		Vars:     []string{"tape.service_account_key"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig().Opts()
			},
			useRealUser: true,
			// Total timeout for TAPE leased account. This needs to be higher than the total runtime
			// of all tests consuming this fixture.
			tapeTimeout: 60 * time.Minute,
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosAdminDeskTemplatesLoggedIn,
		Desc:     "Logged into a user session with admin desk templates for lacros",
		Contacts: []string{"zhumatthew@google.com", "chromeos-commercial-remote-management@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.EnableFeatures("DesksTemplates"))).Opts()
			},
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosEnrolledLoggedIn,
		Desc:     "Fixture for a running FakeDMS with lacros on enrolled device",
		Contacts: []string{"nedol@google.com", "chromeos-commercial-printing@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				opts, err := lacrosfixt.NewConfig().Opts()
				if err != nil {
					return nil, err
				}

				return append(opts, chrome.KeepEnrollment()), nil
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacrosEnrolled,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosEnrolledLoggedInShortMetricsInterval,
		Desc:     "Fixture for a running FakeDMS with lacros on enrolled device that reports metric every second",
		Contacts: []string{"sugandhagoyal@google.com", "dp-chromeos-eng@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				opts, err := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosExtraArgs("--metrics-upload-interval=1"))).Opts()
				if err != nil {
					return nil, err
				}

				return append(opts, chrome.KeepEnrollment()), nil
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacrosEnrolled,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedInAdvancedProtection,
		Desc:     "Logged into a fake user session with advanced protection enabled",
		Contacts: []string{"chiav@google.com", "dp-chromeos-eng@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
					chrome.LacrosExtraArgs("--safe-browsing-treat-user-as-advanced-protection"))).Opts()
			},
		},
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedInBruschetta,
		Desc:     "Logged into a user session with Bruschetta support and enable Lacros",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
					chrome.EnableFeatures("Bruschetta"),
					// Don't show time-of-day wallpapers. We want a solid color for screenshots.
					chrome.DisableFeatures("FeatureManagementTimeOfDayWallpaper"),
				)).Opts()
			},
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.LacrosPolicyLoggedInFilesUXEnabled,
		Desc:     "Logged into a user session with files new policy UX enabled",
		Contacts: []string{"ayaelattar@google.com", "chromeos-commercial-remote-management@google.com"},
		Impl: &policyChromeFixture{
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.EnableFeatures("NewFilesPolicyUX"))).Opts()
			},
		},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout + cleanupTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.PersistentLacros,
	})
}

type policyRealUserFixture struct {
	// fdms is the already running DMS server from the parent fixture.
	fdms *fakedms.FakeDMS
}

// PolicyRealUserFixtData is returned by the fixtures and used in tests
// by using interface HashFakeDMS to get fakeDMS.
type PolicyRealUserFixtData struct {
	// fakeDMS is an already running DMS server.
	fakeDMS *fakedms.FakeDMS
	// Chrome options to be used for starting Chrome and are set in SetUp().
	opts []chrome.Option
}

// FakeDMS implements the HasFakeDMS interface.
func (f PolicyRealUserFixtData) FakeDMS() *fakedms.FakeDMS {
	if f.fakeDMS == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return f.fakeDMS
}

// Opts returns chrome options that were created in SetUp().
func (f PolicyRealUserFixtData) Opts() []chrome.Option {
	return f.opts
}

func (p *policyRealUserFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a FakeDMS fixture")
	}
	p.fdms = fdms

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("policy.ManagedUser.accountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}
	fdms.SetPersistentPolicyUser(&gaiaCreds.User)
	if err := fdms.WritePolicyBlob(policy.NewBlob()); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),
		chrome.CustomLoginTimeout(chrome.ManagedUserLoginTimeout),
	}
	extraOpts, err := lacrosfixt.NewConfig(
		lacrosfixt.ChromeOptions(chrome.GAIALogin(gaiaCreds)),
		lacrosfixt.EnableChromeFRE()).Opts()
	if err != nil {
		return errors.Wrap(err, "failed to get extra options")
	}
	opts = append(opts, extraOpts...)

	return &PolicyRealUserFixtData{p.fdms, opts}
}

func (p *policyRealUserFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (p *policyRealUserFixture) Reset(ctx context.Context) error {
	return nil
}

func (p *policyRealUserFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (p *policyRealUserFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
