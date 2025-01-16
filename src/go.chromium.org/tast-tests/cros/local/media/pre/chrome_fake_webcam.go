// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func initChromeFakeWebCamFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcam",
		Desc:         "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoINPVDWithFakeWebcam",
		Desc:         "Like chromeVideoWithFakeWebcam but with out-of-process video decoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndV4L2FlatStatefulDecoder",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but using the V4L2 Flat stateful VD",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("V4L2FlatStatefulVideoDecoder"),
				chrome.EnableFeatures("UseChromeOSDirectVideoDecoder"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once out-of-process video encoding is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndOOPVE",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but using the out-of-process video encoder",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/248528896): Remove once both out-of-process video decoding and encoding are enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndINPVDAndOOPVE",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but using the out-of-process video encoder and out-of-process video decoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.DisableFeatures("UseOutOfProcessVideoDecoding"),
				chrome.EnableFeatures("UseOutOfProcessVideoEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndNoHwAcceleration",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but with both hardware decoding and encoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-decode"),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndSWEncoding",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but hardware encoding disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/351090228): Remove the test cases after the experiment is complete.
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeVideoWithFakeWebcamAndAllowHwEncoderForLowResolutions",
		Desc:     "Similar to chromeVideoWithFakeWebcam fixture and allowing hw encoders for low resolutions",
		Contacts: []string{"chromeos-gfx-video@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				// Disable the 360p resolution guards.
				chrome.DisableFeatures("ForceSoftwareForLowResolutions"),
				// Disable VA-API small resolution guards.
				chrome.DisableFeatures("VaapiEnforceVideoMinMaxResolution"),
				chrome.DisableFeatures("VaapiVideoMinResolutionForPerformance"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndGlobalVaapiLockDisabled",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture but the global VA-API lock is disabled if applicable",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs("--disable-features=GlobalVaapiLock"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndZeroLatencyRtc",
		Desc:         "Similar to chromeVideo fixture but supplementing it with the use of a fake video/audio capture device (a.k.a. 'fake webcam'), see https://webrtc.org/testing/, and the webrtc rendering smoothness algorithm disabled",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcam60fpsArgs...),
				chrome.ExtraArgs("--disable-rtc-smoothness-algorithm"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/378401081): Remove once vaapi av1 temporal SVC encoding enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeVideoWithFakeWebcamAndVaapiAv1TemporalSVC",
		Desc:         "Similar to chromeVideoWithFakeWebcam fixture and allowing hw encoders for av1 temporal SVC encoding",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(chromeFakeWebcamArgs...),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.EnableFeatures("VaapiAv1TemporalLayerEncoding"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
