// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacrosfixt

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"

	"go.chromium.org/tast/core/testing"
)

func init() {
	// lacros uses rootfs lacros, which is the recommend way to use lacros
	// in Tast tests, unless you have a specific use case for using lacros from
	// another source.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacros",
		Desc:     "Lacros Chrome from a pre-built image",
		Contacts: []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig().Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosHDR is needed for playing and testing videos in HDR on lacros if the device supports it.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosHDR",
		Desc:     "Lacros Chrome from a pre-built image, for HDR tests",
		Contacts: []string{"mrfemi@google.com", "lacros-team@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(
				EnableHDR(),
				ChromeOptions(
					chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
					chrome.ExtraArgs("--suppress-message-center-popups"),           // Do not show message center notifications.
					chrome.ExtraArgs("--arc-availability=none"),                    // Make sure ARC++ is not running.
					chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosQsRevampEnabled is the same as lacros, but has feature QsRevamp enabled.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosQsRevampEnabled",
		Desc:     "Lacros Chrome from a pre-built image, with QsRevamp enabled",
		Contacts: []string{"jamescook@google.com", "lacros-team@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("QsRevamp"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosPerf is the same as lacros, but has some options specific for perf tests.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPerf",
		Desc:     "Lacros Chrome from a pre-built image, for perf tests",
		Contacts: []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			// Powerd is restarts because fwupd starts, which breaks power tests.
			// Disable the FirmwareUpdaterApp feature.
			return NewConfig(ChromeOptions(chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosForceComposition is the same as lacros but
	// forces composition for ash-chrome.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPerfForceComposition",
		Desc:     "Lacros Chrome from a pre-built image with composition forced on",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
				chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosForceNonDelegation is the same as lacros but
	// forces delegated composition off as well as hw overlays.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPerfForceNonDelegated",
		Desc:     "Lacros Chrome from a pre-built image with both delegated compositing and hw overlays forced off",
		Contacts: []string{"petermcneeley@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
				chrome.LacrosDisableFeatures("DelegatedCompositing"),
				chrome.DisableFeatures("FirmwareUpdaterApp"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosAudioQsRevampEnabled is the same as lacros but has some special flags for audio
	// tests. "QsRevamp" is enabled to access the new audio controls in quick settings.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosAudioQsRevampEnabled",
		Desc:     "Lacros Chrome from a pre-built image with camera/microphone permissions",
		Contacts: []string{"hidehiko@chromium.org", "edcourtney@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.EnableFeatures("QsRevamp"),
				chrome.ExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
				chrome.LacrosExtraArgs("--use-fake-ui-for-media-stream"),
				chrome.LacrosExtraArgs("--autoplay-policy=no-user-gesture-required"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
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
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
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
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosDisableSync is a fixture to bring up Lacros as the only browser from the rootfs partition by default, with disabled app sync.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosDisableSync",
		Desc:     "Lacros Chrome from rootfs as the only browser, with disabled app sync",
		Contacts: []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.ExtraArgs("--disable-sync"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosResourcesFileSharing is the same as lacros but has some special flags
	// for resources file sharing tests.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosResourcesFileSharing",
		Desc:     "Lacros Chrome with resources file sharing feature",
		Contacts: []string{"elkurin@chromium.org", "hidehiko@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("LacrosResourcesFileSharing"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosKeepAlive is the same as lacros but has KeepAlive enabled, i.e.
	// Lacros keeps running in the background even when the browser is closed.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosKeepAlive",
		Desc:     "Lacros Chrome with KeepAlive enabled",
		Contacts: []string{"mxcai@chromium.org", "hidehiko@chromium.org"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(KeepAlive(true)).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosVariation is similar to lacros but should be used
	// by variation smoke tests that will launch lacros with variation service enabled,
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosVariationEnabled",
		Desc:     "Lacros with variation service enabled",
		Contacts: []string{"yjt@google.com", "lacros-team@google.com"},
		Vars:     []string{"fakeVariationsChannel"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			channel := "beta"
			if val, ok := s.Var("fakeVariationsChannel"); ok {
				s.Log("Setting fake-variations-channel to ", val)
				channel = val
			}
			return NewConfig(ChromeOptions(
				chrome.LacrosExtraArgs("--fake-variations-channel="+channel),
				chrome.LacrosExtraArgs("--variations-server-url=https://clients4.google.com/chrome-variations/seed"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosGaiaLogin is used to test Lacros with a gaia user login.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosGaiaLogin",
		Desc:     "Lacros Chrome logged into a Gaia user session",
		Contacts: []string{"hyungtaekim@chromium.org", "lacros-team@google.com"},
		Vars:     []string{"ui.gaiaPoolDefault"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosEduGaiaLogin is used to test Lacros with a gaia edu user login.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosEduGaiaLogin",
		Desc:     "Lacros with Edu User Gaia Login",
		Contacts: []string{"yjt@google.com", "lacros-team@google.com"},
		// TODO(https://crbug.com/1380072): create new edu users and use that instead
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosOsFeedback is is similar to lacros but should be used
	// by tests that will launch lacros with OsFeedback enabled.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosOsFeedback",
		Desc:     "Lacros Chrome from a pre-built image with OsFeedback enabled",
		Contacts: []string{"wangdanny@google.com", "cros-feedback-app@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("OsFeedback", "SkipSendingFeedbackReportInTastTests"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosWithStackSampledMetrics",
		Desc:     "Lacros Chrome from a pre-built image; stack-sampled metrics on turned on for both ash and lacros",
		Contacts: []string{"iby@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableStackSampledMetrics())).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 1*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosJellyEnabled",
		Desc:     "Lacros Chrome with Jelly enabled",
		Contacts: []string{"conniekxu@chromium.org", "chromeos-wmp@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(chrome.EnableFeatures("Jelly"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// lacrosPrinterSetupAssistanceEnabled is the same as lacros but has flags
	// for printer setup assistance feature tests. Jelly flag is enabled to ensure
	// screenshots look correct.
	testing.AddFixture(&testing.Fixture{
		Name:     "lacrosPrinterSetupAssistanceEnabled",
		Desc:     "Lacros Chrome from a pre-built image with printer setup assistance and jelly flags enabled",
		Contacts: []string{"cros-peripherals@google.com", "ashleydp@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return NewConfig(ChromeOptions(
				chrome.EnableFeatures("PrintManagementSetupAssistance", "PrintPreviewDiscoveredPrinters", "PrintSettingsRevamp", "PrintSettingsPrinterStatus", "Jelly"),
				chrome.LacrosEnableFeatures("PrintPreviewSetupAssistance"))).Opts()
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
