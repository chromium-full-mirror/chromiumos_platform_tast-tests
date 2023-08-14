// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/logsaver"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedIn,
		Desc:     "Logged into a user session",
		Contacts: []string{"nya@chromium.org", "oka@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return nil, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInDisableSync,
		Desc:     "Logged into a user session with --disable-sync flag",
		Contacts: []string{"dhaddock@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{ExtraArgs("--disable-sync")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInDisableSyncNoFwUpdate,
		Desc:     "Logged into a user session with --disable-sync flag and firmware updates disabled",
		Contacts: []string{"cwd@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{ExtraArgs("--disable-sync"), ExtraArgs("--disable-features=FirmwareUpdaterApp")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInGuest,
		Desc:     "Logged into a guest user session",
		Contacts: []string{"benreich@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GuestLogin()}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWith100FakeApps,
		Desc:     "Logged into a user session with 100 fake apps",
		Contacts: []string{"mukai@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return nil, nil
		}),
		Parent:          "install100Apps",
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: fixture.ChromeLoggedInWith100FakeAppsWithBatterySaver,
		Desc: "Logged into a user session with 100 fake apps and battery saver enabled",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{
				EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"),
			}, nil
		}),
		Parent:          "install100Apps",
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// TOOD(b/233238923): Remove when passthrough is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWith100FakeAppsPassthroughCmdDecoder,
		Desc:     "Logged into a user session with 100 fake apps and the passthrough command decoder enabled",
		Contacts: []string{"hob@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("DefaultPassthroughCommandDecoder")}, nil
		}),
		Parent:          "install100Apps",
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithCalendarView,
		Desc:     "Logged into a session with Gaia user where there are calendar events",
		Contacts: []string{"jiamingc@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			// TODO(b/252870625): Remove QsRevamp when it is enabled by default in tast tests.
			return []Option{GAIALoginPool(s.RequiredVar("calendar.googleCalendarAccountPool")), EnableFeatures("CalendarView", "QsRevamp")}, nil
		}),
		Vars:            []string{"calendar.googleCalendarAccountPool"},
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithCalendarEvents,
		Desc:     "Logged into a session with Gaia user where there are events set up to join Hangout meetings",
		Contacts: []string{"leandre@google.com", "jiamingc@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			// TODO(b/252870625): Remove QsRevamp when it is enabled by default in tast tests.
			return []Option{GAIALoginPool(s.RequiredVar("calendar.googleCalendarAccountPool")), EnableFeatures("PrivacyIndicators", "QsRevamp")}, nil
		}),
		Vars:            []string{"calendar.googleCalendarAccountPool"},
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithUpcomingCalendarEvents,
		Desc:     "Logged into a session with Gaia user where there are upcoming events",
		Contacts: []string{"cros-status-area-eng@google.com", "samcackett@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			// TODO(b/252870625): Remove QsRevamp when it is enabled by default in tast tests.
			return []Option{GAIALoginPool(s.RequiredVar("calendar.upcomingEventsAccountPool")), EnableFeatures("CalendarJelly", "QsRevamp")}, nil
		}),
		Vars:            []string{"calendar.upcomingEventsAccountPool"},
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// TODO(b/252870625): Delete this after M-117 branch when QsRevamp is fully
	// launched and won't be rolled back in chrome.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInQsRevampEnabled,
		Desc:     "Logged into a user session that has updated quick settings enabled (QsRevamp)",
		Contacts: []string{"cros-status-area-eng@google.com", "jamescook@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("QsRevamp")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// TODO(b/252870625): Delete this after all tests are ported to work with
	// QsRevamp.
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInQsRevampDisabled,
		Desc:     "Logged into a user session that has updated quick settings disabled (QsRevamp)",
		Contacts: []string{"cros-status-area-eng@google.com", "jamescook@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{DisableFeatures("QsRevamp")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithGaia,
		Desc:     "Logged into a session with Gaia user",
		Contacts: []string{"jinrongwu@google.com"},
		Vars:     []string{"ui.gaiaPoolDefault"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInThunderbolt,
		Desc:     "Logged into a user session to support thunderbolt devices",
		Contacts: []string{"pathan.jilani@intel.com", "intel-chrome-system-automation-team@intel.com"},
		Vars:     []string{"ui.signinProfileTestExtensionManifestKey"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{DeferLogin(), LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey"))}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsFeedback,
		Desc:     "Logged into a user session with OS Feedback enabled",
		Contacts: []string{"michaelcheco@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("OsFeedback", "SkipSendingFeedbackReportInTastTests")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithLauncherContinueSection,
		Desc:     "Logged into a user session that has continue section in the launcher enabled",
		Contacts: []string{"tbarzic@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("ProductivityLauncher:enable_continue/true")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOobe,
		Desc:     "Log in and proceed with the post-login OOBE flow",
		Contacts: []string{"cros-oobe@google.com", "bohdanty@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{DontSkipOOBEAfterLogin()}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsFeedbackSaveReportToLocalForE2ETesting,
		Desc:     "Logged into a user session with OS Feedback and OsFeedbackSaveReportToLocalForE2ETesting enabled",
		Contacts: []string{"wangdanny@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("OsFeedback", "SkipSendingFeedbackReportInTastTests", "OsFeedbackSaveReportToLocalForE2ETesting")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOobeAndAccessibilityButtonEnabled,
		Desc:     "Log in and proceed with the post-login OOBE flow with the accessibility button enabled on the marketing opt-in screen",
		Contacts: []string{"bohdanty@google.com", "cros-oobe@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{
				DontSkipOOBEAfterLogin(),
				ExtraArgs("--oobe-show-accessibility-button-on-marketing-opt-in-for-testing"),
			}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithStackSampledMetrics,
		Desc:     "Logged into a user session; stack-sampled metrics on turned on",
		Contacts: []string{"iby@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableStackSampledMetrics(), ExtraArgs("--metrics-recording-only")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInExtendedAutocomplete,
		Desc:     "Logged into a user session with FirmwareUpdaterApp disabled",
		Contacts: []string{"yulunwu@chromium.org", "tbarzic@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("AutocompleteExtendedSuggestions")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsSettingsSearchFeedback,
		Desc:     "Logged into a user session with searchFeedbackEnabled flag enabled",
		Contacts: []string{"cros-settings@google.com", "moteva@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("OsFeedback, OsSettingsSearchFeedback")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInGuestWithOsSettingsSearchFeedback,
		Desc:     "Logged into a guest user session with searchFeedbackEnabled flag enabled",
		Contacts: []string{"cros-settings@google.com", "moteva@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GuestLogin(), EnableFeatures("OsFeedback, OsSettingsSearchFeedback")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithPrinterSetupAssistance,
		Desc:     "Logged into a user session with all printer setup assistance and jelly flags enabled",
		Contacts: []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("PrintManagementSetupAssistance", "PrintPreviewDiscoveredPrinters", "PrintPreviewSetupAssistance", "PrintSettingsRevamp", "PrintSettingsPrinterStatus", "Jelly")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInVerboseConsentLogs,
		Desc:     "Logged into a user session with flags to enable verbose logging about consent",
		Contacts: []string{"cwd@chromium.org"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{ExtraArgs("--vmodule=*stats_reporting_controller*=1,*autotest_private_api*=1,*owner_pending_setting_controller*=1")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithPasspoint,
		Desc:     "Logged into a user session with PasspointARCSupport flag enabled",
		Contacts: []string{"jasongustaman@google.com", "cros-networking@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("PasspointARCEnabled")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInWithInputDeviceSettingsSplit",
		Desc:     "Logged into a user session with InputDeviceSettingsSplit enabled",
		Contacts: []string{"wangdanny@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("InputDeviceSettingsSplit", "AllowScrollSettings")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInWithShortcutCustomizationApp",
		Desc:     "Logged into a user session with ShortcutCustomizationApp, OnlyShowNewShortcutsApp and SearchInShortcutsApp enabled",
		Contacts: []string{"jimmyxgong@google.com", "cros-peripherals@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("ShortcutCustomizationApp", "SearchInShortcutsApp", "OnlyShowNewShortcutsApp")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithForceVMDisplayExternal,
		Desc:     "Logged into a user session with VM virtual display marked as external, allowing display mode change",
		Contacts: []string{"chromeos-velocity@google.com", "yixie@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{ExtraArgs("--drm-virtual-connector-is-external")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithJelly,
		Desc:     "Logged into a user session with Jelly enabled",
		Contacts: []string{"conniekxu@chromium.org", "chromeos-sw-engprod@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("Jelly")}, nil
		}),
		SetUpTimeout:    LoginTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})
}

// OptionsCallback is the function used to set up the fixture by returning Chrome options.
type OptionsCallback func(ctx context.Context, s *testing.FixtState) ([]Option, error)

// loggedInFixture is a fixture to start Chrome with the given options.
// If the parent is specified, and the parent returns a value of []Option, it
// will also add those options when starting Chrome.
type loggedInFixture struct {
	cr        *Chrome
	fOpt      OptionsCallback  // Function to generate Chrome Options
	logMarker *logsaver.Marker // Marker for per-test log.
}

// NewLoggedInFixture returns a FixtureImpl with a OptionsCallback function to provide Chrome options.
func NewLoggedInFixture(fOpt OptionsCallback) testing.FixtureImpl {
	return &loggedInFixture{fOpt: fOpt}
}

func (f *loggedInFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var opts []Option
	// If there's a parent fixture and the fixture supplies extra options, use them.
	if extraOpts, ok := s.ParentValue().([]Option); ok {
		opts = append(opts, extraOpts...)
	}

	crOpts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain Chrome options: ", err)
	}
	opts = append(opts, crOpts...)

	cr, err := New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	Lock()
	f.cr = cr
	return cr
}

func (f *loggedInFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

func (f *loggedInFixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *loggedInFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		s.Log("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(f.cr.LogFilename())
	if err == nil {
		f.logMarker = logMarker
	} else {
		s.Log("Failed to start the log saver: ", err)
	}
}

func (f *loggedInFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Log("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}
}
