// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

const (
	// VideoFeatureFakeMediaStreamUI avoids the need to grant camera/microphone permissions.
	VideoFeatureFakeMediaStreamUI featureType = featureType(uint32(1) << (iota + 1))

	// VideoFeatureSWDecoding disables HW accelerated video decoding.
	VideoFeatureSWDecoding

	// VideoFeatureGuestLogin ensures the test runs while logged in as the guest user.
	VideoFeatureGuestLogin

	// VideoFeatureAshComposited disables HW overlays in ash-chrome entirely in order to force video to be composited by ash-chrome.
	VideoFeatureAshComposited

	// VideoFeatureDistinctiveIdentifier allows for a distinctive identifier with DRM playback.
	VideoFeatureDistinctiveIdentifier
)

func initChromeVideoFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideo",
		Desc:         "Logged into a user session with logging enabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoINPVD",
		Desc:         "Logged into a user session with logging and out-of-process video decoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once out-of-process video encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithOOPVE",
		Desc:         "Similar to chromeVideo fixture but enabling out-of-process video encoding",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithGuestLogin",
		Desc:         "Similar to chromeVideo fixture but forcing login as a guest",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.GuestLogin(),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoINPVDWithGuestLogin",
		Desc:         "Like chromeVideoWithGuestLogin but with out-of-process video decoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.GuestLogin(),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	// TODO(crbug.com/958166): Use simply ChromeVideo() when HDR is launched.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithHDRScreen",
		Desc:         "Similar to chromeVideo fixture but enabling the HDR screen if present",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.EnableFeatures("UseHDRTransferFunction"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeCompositedVideo",
		Desc:         "Similar to chromeVideo fixture but disabling hardware overlays entirely to force video to be composited",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithSWDecoding",
		Desc:         "Similar to chromeVideo fixture but making sure Chrome does not use any potential hardware accelerated decoding",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(crbug.com/958166): Use simply ChromeVideoWithSWDecoding() when HDR is launched.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithSWDecodingAndHDRScreen",
		Desc:         "Similar to chromeVideoWithSWDecoding but also enalbing the HDR screen if present",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.EnableFeatures("UseHDRTransferFunction"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithDistinctiveIdentifier",
		Desc:         "Similar to chromeVideo fixture but also allows a distinctive identifier which is needed for HWDRM",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoINPVDWithDistinctiveIdentifier",
		Desc:         "Like chromeVideoWithDistinctiveIdentifier but with out-of-process video decoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeAllowDistinctiveIdentifierArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithBatchDecodingInRenderer",
		Desc:         "Similar to chromeVideo fixture but enabling batch decoding for non-MF renderer path",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("VideoDecodeBatching"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithVCDInUtilityProcess",
		Desc:         "Similar to chromeVideo fixture but running VCD in the utility process",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.DisableFeatures("RunVideoCaptureServiceInBrowserProcess"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithReducedHardwareVideoDecoderBuffers",
		Desc:         "Similar to chromeVideo fixture but reduce the number of required renderer pipeline buffers to fill video frame pool",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("ReduceHardwareVideoDecoderBuffers"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoHardwareTemporalEncoding",
		Desc:         "Pre release testing of temporal hardware encoding functionality",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.EnableFeatures("V4L2H264TemporalLayerHWEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
