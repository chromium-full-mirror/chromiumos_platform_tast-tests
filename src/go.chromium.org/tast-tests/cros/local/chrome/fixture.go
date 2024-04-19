// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/lacros"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/screenshot/cliscreenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// screenshotTimeout is the time allocated to capture a screenshot upon fixture SetUp failure.
const screenshotTimeout = 15 * time.Second

// FixtureSetUpTimeout includes 15 seconds for screenshot capture on failure.
const FixtureSetUpTimeout = LoginTimeout + screenshotTimeout

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedIn,
		Desc:     "Logged into a user session",
		Contacts: []string{"nya@chromium.org", "seewaifu@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return nil, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInDisableSyncWithBatterySaver,
		Desc:     "Logged into a user session with --disable-sync flag and turn on battery saver",
		Contacts: []string{"darrenwu@chromium.org", "cros-vm-technology@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{ExtraArgs("--disable-sync"), EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeNotLoggedIn,
		Desc:     "Start Chrome without logging in",
		Contacts: []string{"cros-tdm-tpe-eng@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{NoLogin()}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithCalendarView,
		Desc:     "Logged into a session with Gaia user where there are calendar events",
		Contacts: []string{"jiamingc@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GAIALoginPool(s.RequiredVar("calendar.googleCalendarAccountPool")), EnableFeatures("CalendarView")}, nil
		}),
		Vars:            []string{"calendar.googleCalendarAccountPool"},
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithCalendarEvents,
		Desc:     "Logged into a session with Gaia user where there are events set up to join Hangout meetings",
		Contacts: []string{"leandre@google.com", "jiamingc@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GAIALoginPool(s.RequiredVar("calendar.googleCalendarAccountPool")), EnableFeatures("PrivacyIndicators")}, nil
		}),
		Vars:            []string{"calendar.googleCalendarAccountPool"},
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithUpcomingCalendarEvents,
		Desc:     "Logged into a session with Gaia user where there are upcoming events",
		Contacts: []string{"cros-status-area-eng@google.com", "samcackett@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GAIALoginPool(s.RequiredVar("calendar.upcomingEventsAccountPool")), EnableFeatures("CalendarJelly")}, nil
		}),
		Vars:            []string{"calendar.upcomingEventsAccountPool"},
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsFeedback,
		Desc:     "Logged into a user session with OS Feedback enabled",
		Contacts: []string{"michaelcheco@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("SkipSendingFeedbackReportInTastTests")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithOobe,
		Desc:         "Log in and proceed with the post-login OOBE flow",
		Contacts:     []string{"cros-oobe@google.com", "bohdanty@google.com"},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{DontSkipOOBEAfterLogin()}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsFeedbackSaveReportToLocalForE2ETesting,
		Desc:     "Logged into a user session with OS Feedback and OsFeedbackSaveReportToLocalForE2ETesting enabled",
		Contacts: []string{"wangdanny@google.com"},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("SkipSendingFeedbackReportInTastTests", "OsFeedbackSaveReportToLocalForE2ETesting")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithOobeAndAccessibilityButtonEnabled,
		Desc:         "Log in and proceed with the post-login OOBE flow with the accessibility button enabled on the marketing opt-in screen",
		Contacts:     []string{"bohdanty@google.com", "cros-oobe@google.com"},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{
				DontSkipOOBEAfterLogin(),
				ExtraArgs("--oobe-show-accessibility-button-on-marketing-opt-in-for-testing"),
			}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithStackSampledMetrics,
		Desc:         "Logged into a user session; stack-sampled metrics on turned on",
		Contacts:     []string{"iby@google.com", "chromeos-data-eng@google.com"},
		BugComponent: "b:1087262",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableStackSampledMetrics(), ExtraArgs("--metrics-recording-only", "--record-stack-sampling-data")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithOsSettingsSearchFeedback,
		Desc:     "Logged into a user session with searchFeedbackEnabled flag enabled",
		Contacts: []string{"cros-settings@google.com", "moteva@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("OsSettingsSearchFeedback")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInGuestWithOsSettingsSearchFeedback,
		Desc:     "Logged into a guest user session with searchFeedbackEnabled flag enabled",
		Contacts: []string{"cros-settings@google.com", "moteva@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{GuestLogin(), EnableFeatures("OsSettingsSearchFeedback")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithPrinterSetupAssistance,
		Desc:     "Logged into a user session with all printer setup assistance flags enabled",
		Contacts: []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("PrintManagementSetupAssistance", "PrintPreviewDiscoveredPrinters", "PrintPreviewSetupAssistance", "PrintSettingsRevamp", "PrintSettingsPrinterStatus")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInWithInputDeviceSettingsSplit",
		Desc:     "Logged into a user session with InputDeviceSettingsSplit enabled",
		Contacts: []string{"wangdanny@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("InputDeviceSettingsSplit", "AllowScrollSettings", "PeripheralCustomization")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
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
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithFieldTrialConfigEnable,
		Desc:         "Logged into a user session with FieldTrialConfigEnable",
		Contacts:     []string{"iby@chromium.org", "cros-exp-wg@google.com"},
		BugComponent: "b:1096648",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{FieldTrialConfig(FieldTrialConfigEnable)}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithFieldTrialConfigDisable,
		Desc:         "Logged into a user session with FieldTrialConfigDisable",
		Contacts:     []string{"iby@chromium.org", "cros-exp-wg@google.com"},
		BugComponent: "b:1096648",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{FieldTrialConfig(FieldTrialConfigDisable)}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithFieldTrialConfigEnableAndVerboseConsent,
		Desc:         "Logged into a user session with FieldTrialConfigEnable and verbose consent flags enabled",
		Contacts:     []string{"iby@chromium.org", "cros-exp-wg@google.com"},
		BugComponent: "b:1096648",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{FieldTrialConfig(FieldTrialConfigEnable),
				ExtraArgs("--vmodule=*stats_reporting_controller*=1,*autotest_private_api*=1,*owner_pending_setting_controller*=1")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ChromeLoggedInWithFieldTrialConfigDisableAndVerboseConsent,
		Desc:         "Logged into a user session with FieldTrialConfigDisable",
		Contacts:     []string{"iby@chromium.org", "cros-exp-wg@google.com"},
		BugComponent: "b:1096648",
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{FieldTrialConfig(FieldTrialConfigDisable),
				ExtraArgs("--vmodule=*stats_reporting_controller*=1,*autotest_private_api*=1,*owner_pending_setting_controller*=1")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
		ResetTimeout:    ResetTimeout,
		TearDownTimeout: ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInWithSchedQoS,
		Desc:     "Logged into a user session with SchedQoSOnResourcedForChrome feature enabled",
		Contacts: []string{"kawasin@chromium.org", "cros-core-systems-perf@google.com"},
		Impl: NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]Option, error) {
			return []Option{EnableFeatures("SchedQoSOnResourcedForChrome")}, nil
		}),
		SetUpTimeout:    FixtureSetUpTimeout,
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
	parentState func(s *testing.FixtState) interface{}
	cr          *Chrome
	fOpt        OptionsCallback  // Function to generate Chrome Options
	logMarker   *logsaver.Marker // Marker for per-test log.

	// The last time PreTest was called. This is used by PostTest to collect per-test Lacros logs.
	// Note that cr contains its own logsStartTime, which remains untouched and will be used for
	// per-fixture collection of Lacros logs.
	logsStartTime time.Time
}

type loggedInFixtureState struct {
	parentState interface{}
	cr          *Chrome
}

// HasParentState allows for the retrieval of some state provided by parent
// fixture.
type HasParentState interface {
	// ParentState returns the fixture state of the parent fixture.
	ParentState() interface{}
}

// Chrome function, part of HasChrome, returns the Chrome object.
func (f *loggedInFixtureState) Chrome() *Chrome {
	return f.cr
}

// ParentState, implementation of HasParentState, returns the parent fixture
// state.
func (f *loggedInFixtureState) ParentState() interface{} {
	return f.parentState
}

// NewLoggedInFixture returns a FixtureImpl with a OptionsCallback function to
// provide Chrome options.
func NewLoggedInFixture(fOpt OptionsCallback) testing.FixtureImpl {
	return &loggedInFixture{fOpt: fOpt}
}

// NewLoggedInFixtureWithParentState returns a FixtureImpl with a OptionsCallback function to provide Chrome options, as well as some state provided by the parent fixture.
func NewLoggedInFixtureWithParentState(ps func(s *testing.FixtState) interface{}, fOpt OptionsCallback) testing.FixtureImpl {
	return &loggedInFixture{
		parentState: ps,
		fOpt:        fOpt,
	}
}

func (f *loggedInFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Reserve 15 seconds for screenshot capture,
	// so it has time even if the New() function times out.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	var opts []Option
	// If there's a parent fixture and the fixture supplies extra options, use them.
	if extraOpts, ok := s.ParentValue().([]Option); ok {
		opts = append(opts, extraOpts...)
	}

	var parentState interface{}
	if f.parentState != nil {
		parentState = f.parentState(s)
	}

	crOpts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain Chrome options: ", err)
	}
	opts = append(opts, crOpts...)

	defer screenshotOnError(cleanupCtx, s.HasError)

	cr, err := New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	Lock()
	f.cr = cr
	return &loggedInFixtureState{
		parentState: parentState,
		cr:          cr,
	}
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
	f.logsStartTime = time.Now().UTC()
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

	if err := lacros.SaveLogsAfter(ctx, s.OutDir(), f.logsStartTime); err != nil {
		s.Log("Failed to store per-test Lacros log data: ", err)
	}
}

func screenshotOnError(ctx context.Context, hasError func() bool) {
	if !hasError() {
		return
	}

	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		testing.ContextLog(ctx, "Output directory is unavailable")
		return
	}

	path := filepath.Join(dir, "fixture_failure.png")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Ensure that we can try multiple times even if the call gets stuck.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		return cliscreenshot.Capture(ctx, path)
	}, &testing.PollOptions{Timeout: screenshotTimeout, Interval: time.Second}); err != nil {
		testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
	}
}
