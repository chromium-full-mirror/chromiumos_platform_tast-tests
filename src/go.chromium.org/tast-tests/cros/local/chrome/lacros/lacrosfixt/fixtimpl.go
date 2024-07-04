// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacrosfixt

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/calendar"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"

	"go.chromium.org/tast/core/testing"
)

func init() {
	// lacros uses rootfs lacros, which is the recommend way to use lacros
	// in Tast tests, unless you have a specific use case for using lacros from
	// another source.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacros",
		Desc:         "Lacros Chrome from a pre-built image",
		Contacts:     []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		BugComponent: "b:1088267",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig().Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosHDR is needed for playing and testing videos in HDR on lacros if the device supports it.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosHDR",
		Desc:         "Lacros Chrome from a pre-built image, for HDR tests",
		Contacts:     []string{"mrfemi@google.com", "lacros-team@google.com"},
		BugComponent: "b:1457249",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(
				EnableHDR(),
				ChromeOptions(
					chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
					chrome.ExtraArgs("--suppress-message-center-popups"),           // Do not show message center notifications.
					chrome.ExtraArgs("--arc-availability=none"),                    // Make sure ARC++ is not running.
					chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosPerf is the same as lacros, but has some options specific for perf tests.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosPerf",
		Desc:         "Lacros Chrome from a pre-built image, for perf tests",
		Contacts:     []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		BugComponent: "b:1088267",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			// Powerd is restarts because fwupd starts, which breaks power tests.
			// Disable the FirmwareUpdaterApp feature.
			return NewConfig(ChromeOptions(chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosForceComposition is the same as lacros but
	// forces composition for ash-chrome.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPerfForceComposition",
		Desc:     "Lacros Chrome from a pre-built image with composition forced on",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org", "cros-sw-perf@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
				chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosForceNonDelegation is the same as lacros but
	// forces delegated composition off as well as hw overlays.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPerfForceNonDelegated",
		Desc:     "Lacros Chrome from a pre-built image with both delegated compositing and hw overlays forced off",
		Contacts: []string{"petermcneeley@chromium.org", "edcourtney@chromium.org", "cros-sw-perf@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
				chrome.LacrosDisableFeatures("DelegatedCompositing"),
				chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosAudioFieldTrialConfigDisable is the same as lacros but has some special flags for audio tests.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosAudioFieldTrialConfigDisable",
		Desc:     "Lacros Chrome from a pre-built image with camera/microphone permissions",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.ExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
				chrome.LacrosExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.LacrosExtraArgs("--autoplay-policy=no-user-gesture-required"),
				chrome.FieldTrialConfig(chrome.FieldTrialConfigDisable))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosAudioFieldTrialConfigEnable is the same as lacros but has some special flags for audio tests
	// and enables the field-trial config.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosAudioFieldTrialConfigEnable",
		Desc:     "Lacros Chrome from a pre-built image with camera/microphone permissions",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.ExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
				chrome.LacrosExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.LacrosExtraArgs("--autoplay-policy=no-user-gesture-required"),
				chrome.FieldTrialConfig(chrome.FieldTrialConfigEnable))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosWith100FakeApps is the same as the "lacros" fixture but
	// creates 100 fake apps for lacros that are shown in the OS launcher.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosWith100FakeApps",
		Desc:     "Lacros Chrome from a pre-built image with 100 fake apps installed for lacros",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig().Opts()
		}),
		Parent:          "install100LacrosApps",
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosOmaha is a fixture to enable Lacros by feature flag in Chrome.
	// This does not require downloading a binary from Google Storage before the test.
	// It will use the currently available fishfood release of Lacros from Omaha.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosOmaha",
		Desc:     "Lacros Chrome from omaha",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(Selection(lacros.Omaha)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosDisableSync is a fixture to bring up Lacros as the only browser from the rootfs partition by default, with disabled app sync.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosDisableSync",
		Desc:         "Lacros Chrome from rootfs as the only browser, with disabled app sync",
		Contacts:     []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		BugComponent: "b:1088267",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--disable-sync"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosResourcesFileSharing is the same as lacros but has some special flags
	// for resources file sharing tests.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosResourcesFileSharing",
		Desc:     "Lacros Chrome with resources file sharing feature",
		Contacts: []string{"elkurin@chromium.org", "hidehiko@chromium.org"},
		// Public Trackers > Chromium Public Trackers > Chromium > OS > LaCrOS > Core
		BugComponent: "b:1456870",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("LacrosResourcesFileSharing"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosKeepAlive is the same as lacros but has KeepAlive enabled, i.e.
	// Lacros keeps running in the background even when the browser is closed.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosKeepAlive",
		Desc:     "Lacros Chrome with KeepAlive enabled",
		Contacts: []string{"hidehiko@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(KeepAlive(true)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosGaiaLogin is used to test Lacros with a gaia user login.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosGaiaLogin",
		Desc:         "Lacros Chrome logged into a Gaia user session",
		Contacts:     []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		BugComponent: "b:1088267",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosEduGaiaLogin is used to test Lacros with a gaia edu user login.
	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosEduGaiaLogin",
		Desc:         "Lacros with Edu User Gaia Login",
		Contacts:     []string{"kuanhuang@google.com", "lacros-team@google.com"},
		BugComponent: "b:1088267",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				// TODO(https://crbug.com/1380072): create new edu users and use that instead
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosOsFeedback is is similar to lacros but should be used
	// by tests that will launch lacros with OsFeedback enabled.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosOsFeedback",
		Desc:     "Lacros Chrome from a pre-built image with OsFeedback enabled",
		Contacts: []string{"wangdanny@google.com", "cros-feedback-app@google.com"},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("SkipSendingFeedbackReportInTastTests"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "lacrosWithStackSampledMetrics",
		Desc:         "Lacros Chrome from a pre-built image; stack-sampled metrics turned on for lacros",
		Contacts:     []string{"iby@google.com", "chromeos-data-eng@google.com"},
		BugComponent: "b:1087262",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.EnableLacrosStackSampledMetrics(),
				chrome.LacrosExtraArgs("--metrics-recording-only", "--record-stack-sampling-data"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosPrinterSetupAssistanceEnabled is the same as lacros but has flags
	// for printer setup assistance feature tests.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPrinterSetupAssistanceEnabled",
		Desc:     "Lacros Chrome from a pre-built image with printer setup assistance and jelly flags enabled",
		Contacts: []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		// ChromeOS > Software > Fundamentals > Peripherals > Printing
		BugComponent: "b:1131981",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.EnableFeatures("PrintManagementSetupAssistance", "PrintPreviewDiscoveredPrinters", "PrintSettingsRevamp", "PrintSettingsPrinterStatus"),
				chrome.LacrosEnableFeatures("PrintPreviewSetupAssistance"))).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosLoggedInWithCalendarEvents is similar chromeLoggedInWithCalendarEvents but with Lacros enabled.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosLoggedInWithCalendarEvents",
		Desc:     "Lacros Chrome with Gaia user where there are events set up to join Hangout meetings",
		Contacts: []string{"cros-status-area-eng@google.com", "leandre@google.com", "jiamingc@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.GAIALoginPool(dma.CredsFromPool(calendar.GoogleCalendarAccountPoolVarName)),
				chrome.EnableFeatures("PrivacyIndicators"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
