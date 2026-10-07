// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/disk"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/mlbenchmark"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	pUtil "go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/scx"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// CPUStabilizationTimeout is the time to wait for cpu stabilization, which
	// is the sum of cpu cool down time, cpu idle time, and cpu pkg state activity drop time.
	CPUStabilizationTimeout = cujrecorder.CooldownTimeout
	// BatteryChargingTimeout is the battery charging duration if capacity is
	//  below 25%
	BatteryChargingTimeout = 50 * time.Minute
	// webRTCLogsGatherTimeout is the time allowed for gathering the WebRTC
	// event log files into a gzip archive in the test output directory.
	webRTCLogsGatherTimeout = 15 * time.Second

	// arcLogsGatherTimeout is the time allowed for gathering ARC logs.
	arcLogsGatherTimeout = 15 * time.Second

	// postTestTimeout is the time allowed for gather various logs if needed.
	postTestTimeout = webRTCLogsGatherTimeout + arcLogsGatherTimeout

	// setUpTimeout is the time to set up chrome and arc.
	setUpTimeout          = chrome.MinLoginTimeout + arc.BootTimeout + 2*time.Minute
	setUpWithOptinTimeout = setUpTimeout + optin.OptinTimeout
	resetTimeout          = 30 * time.Second

	// batterySaverTimeout is the time to enable or disable battery saver.
	batterySaverTimeout = 10 * time.Second

	webRTCEventLogCommandFlag = "--webrtc-event-logging=/tmp"
	webRTCEventLogFilePattern = "/tmp/event_log_*.log"

	highResFakeCameraFileName = "1080p_camera_video.mjpeg"
	lowResFakeCameraFileName  = "720p_camera_video.mjpeg"
)

// webRTCOpts configures WebRTC logging behavior.
// Increase the stats polling interval to reduce dump file size.
var webRTCOpts = []chrome.Option{
	chrome.EnableFeatures("WebRtcInternalsStatsPollingInterval:interval/20s"),
	chrome.ExtraArgs(webRTCEventLogCommandFlag),
}

// Benchmark flags to mimic CrossBench setup.
var benchmarkFlags = []string{
	"--no-default-browser-check",
	"--disable-component-update",
	"--no-first-run",
	"--disable-search-engine-choice-screen",
	"--disable-background-timer-throttling",
	"--disable-renderer-backgrounding",
	"--no-experiments",
	"--enable-benchmarking",
	"--disable-field-trial-config",
}

// webUIOmniboxOptimizationFeatures are the optimizations for the WebUI
// Omnibox popup.
var webUIOmniboxOptimizationFeatures = []string{
	"OmniboxWebUIPopupHideOnCreation",
	"OmniboxWebUIPopupMarkAsHidden",
}

// isLocalVar is a runtime variable that specifies whether to skip
// waiting for the CPU to cooldown and idle, which speeds up overall
// test runtime. This variable is explicitly made for local testing,
// when performance data does not matter.
var isLocalVar = testing.RegisterVarString(
	"cuj.isLocal",
	"",
	"A boolean string (true/false) signifying whether or not to skip certain startup procedures for local testing",
)

// disableChargeBatteryBeforeTest is a runtime variable that specifies
// whether to disable battery charging when battery capacity is not
// higher than minimumBatteryCapacity+lowBatteryShutdownPercent.
var disableChargeBatteryBeforeTest = testing.RegisterVarString(
	"cuj.disableChargeBatteryBeforeTest",
	"",
	"A boolean string (true/false) signifying whether to charge battery before running a test",
)

// minimumBatteryCapacity is the minimum battery capacity on top of the
// low battery shutdown percentage.
var minimumBatteryCapacity = 25.0
var chargeBatteryTestPollOpt = &testing.PollOptions{Interval: 60 * time.Second, Timeout: BatteryChargingTimeout}

// minimumBatteryRequirementForCUJs is the lowest value that CUJ tests can start
// with for battery capacity - any lower and the test should fail.
var minimumBatteryRequirementForCUJs = 50.0

var extraArgsVar = testing.RegisterVarString(
	"cuj.extraArgs",
	"",
	"A comma separated list of extra args to be passed into Chrome",
)

var extraFeaturesVar = testing.RegisterVarString(
	"cuj.extraFeatures",
	"",
	"A comma separated list of extra features to be passed into Chrome",
)

// DocsBlocker extension files.
var docsBlockerFiles = []string{
	"docs_blocker/background.js",
	"docs_blocker/manifest.json",
}

// DocsBlocker extension ID.
var docsBlockerExtensionID = "lanldddoamfhbgpgmmlckdklilaggblp"

// EnableRealCameraVar is a runtime variable that specifies
// whether to enable real camera if the fixture uses a fake camera.
var EnableRealCameraVar = testing.RegisterVarString(
	"cuj.enableRealCamera",
	"",
	"A boolean string (true/false) signifying whether to enable real camera if the fixture uses a fake camera",
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "prepareForCUJ",
		Desc: "The fixture to prepare DUT for CUJ tests",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &prepareCUJFixture{
			minBatteryRequirement: minimumBatteryRequirementForCUJs,
		},
		PreTestTimeout:  CPUStabilizationTimeout + BatteryChargingTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          "gpuWatchHangs",
	})
	testing.AddFixture(&testing.Fixture{
		Name: "prepareForCUJWithoutCooldown",
		Desc: "The fixture to prepare DUT for CUJ tests without CPU cooldown",
		Contacts: []string{
			"vincentchiang@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &prepareCUJFixture{
			minBatteryRequirement: minimumBatteryRequirementForCUJs,
			skipCPUCooldown:       true,
		},
		PreTestTimeout:  BatteryChargingTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          "gpuWatchHangs",
	})
	testing.AddFixture(&testing.Fixture{
		Name: "prepareForCUJWithCharge",
		Desc: "The fixture to wait DUT cpu to stabilize for CUJ tests",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &prepareCUJFixture{chargeBattery: true},
		PreTestTimeout:  CPUStabilizationTimeout + BatteryChargingTimeout + 5*time.Second,
		PostTestTimeout: postTestTimeout,
		Parent:          "gpuWatchHangs",
	})
	testing.AddFixture(&testing.Fixture{
		Name: "prepareForCUJEnrolledWithCharge",
		Desc: "The fixture to wait DUT cpu to stabilize for logged in with gaia user on an enrolled device",
		Contacts: []string{
			"alston.huang@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &prepareCUJFixture{chargeBattery: true},
		SetUpTimeout:    chrome.EnrollmentAndLoginTimeout + chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout + BatteryChargingTimeout + 5*time.Second,
		PostTestTimeout: postTestTimeout,
		Parent:          "gpuWatchHangsEnrolled",
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUser",
		Desc: "The main fixture used for UI CUJ tests",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &loggedInToCUJUserFixture{},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserDisableARC",
		Desc: "The main fixture used for UI CUJ tests with ARC disabled",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			disableARC: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	// loggedInToCUJUserARCSupported fixture is similar to loggedInToCUJUser
	// but uses "chrome.ARCSupported" flag instead of "chrome.ARCEnabled". When
	// a test needs to open any ARC windows or use the Play Store, this fixture
	// should be used so that ARC Play Store optin procedure can be performed.
	// This is the same case for all other fixtures with "ARCSupported" in
	// their names.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupported",
		Desc: "The main fixture used for UI CUJ tests with ARC supported",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			arcSupported: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
		Desc: "The main fixture used for UI CUJ tests using an enterprise account, with WebRTC event logging",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			useEnterprisePool: true,
			docsBlocker:       true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujEnterpriseAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepState",
		Desc: "The CUJ test fixture which keeps login state",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			keepState: true,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateARCSupported",
		Desc: "The CUJ test fixture which keeps login state with ARC supported",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			keepState:    true,
			arcSupported: true,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithBatterySaverParent",
		Desc: "The CUJ test fixture which keeps login state and turns on battery saver without Android battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			keepState: true,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--allow-insecure-localhost"),
			},
			enableBatterySaver: true,
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithBatterySaver",
		Desc: "The CUJ test fixture which keeps login state and turns on battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInAndKeepStateWithBatterySaverParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithFakeCamera",
		Desc: "The CUJ test fixture which keeps login state and uses fake camera",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         []string{highResFakeCameraFileName},
		Impl: &loggedInToCUJUserFixture{
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: highResFakeCameraFileName,
			chromeExtraOpts:    []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithLowResFakeCamera",
		Desc: "The CUJ test fixture which keeps login state and uses low resolution fake camera",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         append(docsBlockerFiles, lowResFakeCameraFileName),
		Impl: &loggedInToCUJUserFixture{
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: lowResFakeCameraFileName},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "enrolledLoggedInToCUJUser",
		Desc: "Logged in with gaia user on an enrolled device",
		Contacts: []string{
			"alston.huang@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &loggedInToCUJUserFixture{},
		Parent:          "prepareForCUJEnrolledWithCharge",
		SetUpTimeout:    chrome.EnrollmentAndLoginTimeout + chrome.GAIALoginTimeout + optin.OptinTimeout + 2*time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLogging",
		Desc: "CUJ test fixture with WebRTC event logging",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	// TODO(b/331565548): Remove if VsyncDecoding is launched.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithVsyncDecoding",
		Desc: "CUJ test fixture with WebRTC event logging",
		Contacts: []string{
			"hiroh@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate", "VsyncDecoding"),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingDisableARC",
		Desc: "CUJ test fixture with WebRTC event logging with ARC disabled",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			disableARC:  true,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
		Desc: "CUJ test fixture with WebRTC event logging and VC platform effects enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"sammc@chromium.org",
			"cros-sw-perf@google.com",
			"cros-pe-pnp@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures(
					"PreferConstantFrameRate",
					"CrOSLateBootAudioAPNoiseCancellation",
					"ShowLiveCaptionInVideoConferenceTray",
					"SystemLiveCaption",
					"VideoConference",
					"FeatureManagementVideoConference",
				),
				chrome.DisableFeatures("CrOSLateBootAudioStyleTransfer"),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsAndStudioMic",
		Desc: "CUJ test fixture with WebRTC event logging and VC platform effects, including studio mic, enabled",
		Contacts: []string{
			"sammc@chromium.org",
			"cranelw@google.com",
			"cros-sw-perf@google.com",
			"cros-pe-pnp@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures(
					"PreferConstantFrameRate",
					"CrOSLateBootAudioStyleTransfer",
					"ShowLiveCaptionInVideoConferenceTray",
					"SystemLiveCaption",
					"VideoConference",
					"FeatureManagementVideoConference",
				),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithBatterySaverParent",
		Desc: "CUJ test fixture with WebRTC event logging before set ARC battery saver mode",
		Contacts: []string{
			"darrenwu@google.com",
			"chromeos-bsm@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			docsBlocker:        true,
			enableBatterySaver: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithBatterySaver",
		Desc: "CUJ test fixture with WebRTC event logging and battery saver",
		Contacts: []string{
			"darrenwu@google.com",
			"chromeos-bsm@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserWithWebRTCEventLoggingWithBatterySaverParent",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithScxCentral",
		Desc: "CUJ test fixture with WebRTC event logging with scx_central scheduler",
		Contacts: []string{
			"darrenwu@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			docsBlocker: true,
			scxType:     scx.TypeScxCentral,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFieldTrials",
		Desc: "CUJ fixture with all field trials enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-field-trial-config"),
				chrome.EnableFeatures("DisablePrivacySandboxPrompts"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedWithFieldTrials",
		Desc: "CUJ fixture with ARC supported and all field trials",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-field-trial-config"),
				// TODO: (b/359374171) Remove this once the "ArcVmGki" feature
				// is removed from fieldtrial_testing_config.json.
				chrome.DisableFeatures("ArcVmGki"),
			},
			arcSupported: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFieldTrialsAndWebRTCEventLogging",
		Desc: "Variant of loggedInToCUJUserWithFieldTrials with WebRTCEventLogging enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.ExtraArgs("--enable-field-trial-config"),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFieldTrialsWithoutCooldown",
		Desc: "CUJ fixture with all field trials enabled without CPU cooldown",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-field-trial-config"),
				chrome.EnableFeatures("DisablePrivacySandboxPrompts"),
			},
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverParent",
		Desc: "CUJ test fixture with battery saver without Android",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			enableBatterySaver: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaver",
		Desc: "CUJ test fixture with battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserWithBatterySaverParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBounceKeys",
		Desc: "CUJ fixture with Bounce Keys feature enabled",
		Contacts: []string{
			"aluh@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1686419", // ChromeOS > Software > Experiences > Accessibility > Features > Slow And Bounce Keys
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("AccessibilityBounceKeys"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithChromeVox",
		Desc: "CUJ fixture with ChromeVox enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			enableChromeVox: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown",
		Contacts: []string{
			"vincentchiang@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &loggedInToCUJUserFixture{},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithoutCooldownBenchmark",
		Desc: "CUJ fixture that skips CPU cooldown and has Benchmark Flags",
		Contacts: []string{
			"vincentchiang@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{chrome.ExtraArgs(benchmarkFlags...)},
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithVulkanWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown and runs with Vulkan composite/raster",
		Contacts: []string{
			"hob@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("Vulkan", "DefaultANGLEVulkan", "VulkanFromANGLE"),
				chrome.ExtraArgs(benchmarkFlags...),
			},
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverWithoutCooldownParent",
		Desc: "CUJ fixture that skips CPU cooldown and has battery saver active without Android battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			enableBatterySaver: true,
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown and has battery saver active",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserWithBatterySaverWithoutCooldownParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	// TODO(b/292249282): Remove when Vulkan is launched on brya and volteer.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserVulkan",
		Desc: "CUJ fixture that enables Vulkan for raster/composite",
		Contacts: []string{
			"hob@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("Vulkan"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithMlbenchmarkDataDirectory",
		Desc: "CUJ fixture used for UI CUJ tests with mlbenchmark data directory",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			mlbenchmarkDataDirectory: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithMlbenchmarkDataDirectory",
		Desc: "CUJ fixture with WebRTC event logging with mlbenchmark data directory",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
			),
			disableARC:               true,
			docsBlocker:              true,
			mlbenchmarkDataDirectory: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithMlbenchmarkDataDirectoryWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown with mlbenchmark data directory",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			mlbenchmarkDataDirectory: true,
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCAndMlbenchmarkAndVCEffects",
		Desc: "CUJ fixture with WebRTC event logging, mlbenchmark data directory and VC platform effects enabled",
		Contacts: []string{
			"vivian.chen@cienet.com",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures(
					"PreferConstantFrameRate",
					"CrOSLateBootAudioAPNoiseCancellation",
					"ShowLiveCaptionInVideoConferenceTray",
					"SystemLiveCaption",
					"VideoConference",
					"FeatureManagementVideoConference",
				),
				chrome.DisableFeatures("CrOSLateBootAudioStyleTransfer"),
			),
			disableARC:               true,
			mlbenchmarkDataDirectory: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithForceComposition",
		Desc: "Chrome from a pre-built image with composition forced on",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithForceNonDelegated",
		Desc: "Chrome from a pre-built image with both delegated compositing and hw overlays forced off",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-hardware-overlays=\"\""),
				chrome.DisableFeatures("DelegatedCompositing"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFocusMode",
		Desc: "Chrome from a pre-built image with FocusMode feature enabled",
		Contacts: []string{
			"richui@google.com",
			"chromeos-wms@google.com",
		},
		BugComponent: "b:1238195", // ChromeOS > Software > Focus Mode
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("FocusMode"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithCoralEnabled",
		Desc: "CUJ test fixture with Coral feature enabled",
		Contacts: []string{
			"hcyang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent: "b:1445284",
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures(
					"CoralFeature",
				),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithCoralEnabledAndWebRTCEventLogging",
		Desc: "CUJ test fixture with Coral feature enabled with WebRTCEventLogging enabled",
		Contacts: []string{
			"hcyang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent: "b:1445284",
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("CoralFeature"),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	// The following fixtures are variants of loggedInToCUJUser and
	// loggedInToCUJUserWithWebRTCEventLogging, used to compare CUJ performance
	// across these WebUI Omnibox configs:
	//  1. WebUIOmniboxDisabled: all WebUI Omnibox features disabled.
	//  2. WebUIOmniboxPopupEnabledOnly: WebUIOmniboxPopup and
	//     WebUIOmniboxAimPopup enabled.
	//  3. WebUIOmniboxFullPopupEnabledOnly: WebUIOmniboxFullPopup and
	//     WebUIOmniboxAimPopup enabled.
	//  4. WebUIOmniboxPopupEnabledAndOptimized: config 2 with
	//     webUIOmniboxOptimizationFeatures enabled.
	//  5. WebUIOmniboxFullPopupEnabledAndOptimized: config 3 with
	//     webUIOmniboxOptimizationFeatures enabled.
	// Every feature that a config doesn't enable is explicitly disabled.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWebUIOmniboxDisabled",
		Desc: "Variant of loggedInToCUJUser with the WebUI Omnibox popup and its optimizations disabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.DisableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup", "WebUIOmniboxFullPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWebUIOmniboxPopupEnabledOnly",
		Desc: "Variant of loggedInToCUJUser with WebUIOmniboxPopup and WebUIOmniboxAimPopup enabled, without the popup optimizations",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxFullPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWebUIOmniboxFullPopupEnabledOnly",
		Desc: "Variant of loggedInToCUJUser with WebUIOmniboxFullPopup and WebUIOmniboxAimPopup enabled, without the popup optimizations",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("WebUIOmniboxFullPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWebUIOmniboxPopupEnabledAndOptimized",
		Desc: "Variant of loggedInToCUJUser with WebUIOmniboxPopup, WebUIOmniboxAimPopup and the popup optimizations enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxFullPopup"),
				chrome.EnableFeatures(webUIOmniboxOptimizationFeatures...),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWebUIOmniboxFullPopupEnabledAndOptimized",
		Desc: "Variant of loggedInToCUJUser with WebUIOmniboxFullPopup, WebUIOmniboxAimPopup and the popup optimizations enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("WebUIOmniboxFullPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxPopup"),
				chrome.EnableFeatures(webUIOmniboxOptimizationFeatures...),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWebUIOmniboxDisabled",
		Desc: "Variant of loggedInToCUJUserWithWebRTCEventLogging with the WebUI Omnibox popup and its optimizations disabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.DisableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup", "WebUIOmniboxFullPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWebUIOmniboxPopupEnabledOnly",
		Desc: "Variant of loggedInToCUJUserWithWebRTCEventLogging with WebUIOmniboxPopup and WebUIOmniboxAimPopup enabled, without the popup optimizations",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.EnableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxFullPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWebUIOmniboxFullPopupEnabledOnly",
		Desc: "Variant of loggedInToCUJUserWithWebRTCEventLogging with WebUIOmniboxFullPopup and WebUIOmniboxAimPopup enabled, without the popup optimizations",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.EnableFeatures("WebUIOmniboxFullPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxPopup"),
				chrome.DisableFeatures(webUIOmniboxOptimizationFeatures...),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWebUIOmniboxPopupEnabledAndOptimized",
		Desc: "Variant of loggedInToCUJUserWithWebRTCEventLogging with WebUIOmniboxPopup, WebUIOmniboxAimPopup and the popup optimizations enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.EnableFeatures("WebUIOmniboxPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxFullPopup"),
				chrome.EnableFeatures(webUIOmniboxOptimizationFeatures...),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWebUIOmniboxFullPopupEnabledAndOptimized",
		Desc: "Variant of loggedInToCUJUserWithWebRTCEventLogging with WebUIOmniboxFullPopup, WebUIOmniboxAimPopup and the popup optimizations enabled",
		Contacts: []string{
			"vincentchiang@chromium.org",
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:         docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: append(
				webRTCOpts,
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.EnableFeatures("WebUIOmniboxFullPopup", "WebUIOmniboxAimPopup"),
				chrome.DisableFeatures("WebUIOmniboxPopup"),
				chrome.EnableFeatures(webUIOmniboxOptimizationFeatures...),
			),
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStabilizationTimeout,
		PostTestTimeout: postTestTimeout,
	})
}

func prepareDocsBlockerExtension(s *testing.FixtState) (string, error) {
	extDir, err := os.MkdirTemp("", "docs_blocker_extension")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temporary directory for DocsBlockerExtension")
	}

	if err := os.Chown(extDir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return "", errors.Wrap(err, "failed to chown DocsBlockerExtension dir")
	}

	for _, file := range docsBlockerFiles {
		dst := filepath.Join(extDir, filepath.Base(file))
		if err := fsutil.CopyFile(s.DataPath(file), dst); err != nil {
			return "", errors.Wrapf(err, "failed to copy %q file to %q", file, extDir)
		}

		if err := os.Chown(dst, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			return "", errors.Wrapf(err, "failed to chown %q", file)
		}
	}

	return extDir, nil
}

// GetDocsBlockerConn returns a connection to the DocsBlocker background page
// and waits for the background page to finish loading before it returns.
func GetDocsBlockerConn(ctx context.Context, cr *chrome.Chrome) (*chrome.Conn, error) {
	bgURL := "chrome-extension://" + docsBlockerExtensionID + "/background.js"

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		return nil, errors.Wrap(err, "DocsBlocker extension not found")
	}

	if err = conn.WaitForExpr(ctx, `typeof keepAliveTimerId === "number" && keepAliveTimerId !== 0`); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to wait for DocsBlocker extension")
	}
	return conn, nil
}

func loginCreds(s *testing.FixtState, useEnterprisePool bool) (credconfig.Creds, error) {
	if useEnterprisePool {
		return credconfig.PickRandomCreds(s.RequiredVar("ui.cujEnterpriseAccountPool"))
	}

	return credconfig.PickRandomCreds(dma.CredsFromPool(ui.CUJAccountPoolVarName))
}

func startFakeDMSWithARCEnabled(ctx context.Context, outdir, user string) (fdms *fakedms.FakeDMS, retErr error) {
	blob := policy.NewBlob()
	blob.PolicyUser = user
	// Set UniversalSigningKeys flag to disable the user domain
	// verification, so that test accounts associated with non
	// "managedchrome.com" domains can be used for the test.
	blob.UseUniversalSigningKeys = true
	// Provision the policy with ARCEnabled.
	if err := blob.AddPolicies([]policy.Policy{&policy.ArcEnabled{Val: true}}); err != nil {
		return nil, errors.Wrap(err, "failed to add policy to policy blob")
	}

	fdms, err := fakedms.New(ctx, outdir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start fake policy server")
	}
	defer func() {
		if retErr != nil {
			fdms.Stop(ctx)
		}
	}()
	if err := fdms.WritePolicyBlob(blob); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob to fdms")
	}
	return fdms, nil
}

func runningPackages(ctx context.Context, a *arc.ARC) (map[string]struct{}, error) {
	tasks, err := a.TaskInfosFromDumpsys(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "listing activities failed")
	}
	acts := make(map[string]struct{})
	for _, t := range tasks {
		for _, a := range t.ActivityInfos {
			acts[a.PackageName] = struct{}{}
		}
	}
	return acts, nil
}

// chargeBatteryCapacity allows charging of the battery for 8 minutes if battery capacity
// is not higher than a pre-defined level (minimumBatteryCapacity+lowBatteryShutdownPercent).
func chargeBatteryCapacity(ctx context.Context, minimumBatteryCapacity float64, chargeBatteryTestPollOpt *testing.PollOptions) error {
	// Check if there is a battery first.
	devPath, err := pm.SysfsBatteryPath(ctx)
	if err != nil {
		return err
	}
	if err := setup.AllowBatteryCharging(ctx); err != nil {
		return err
	}
	lowBatteryShutdownPercent, err := pm.LowBatteryShutdownPercent(ctx)
	if err != nil {
		return err
	}
	initCapacity := -1.0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		capacity, err := pm.ReadBatteryCapacity(ctx, devPath)
		if err != nil {

			return errors.Wrap(err, "failed to get battery capacity")
		}
		// Set the initial battery capacity.
		if initCapacity < 0.0 {
			initCapacity = capacity
		}

		testing.ContextLogf(ctx, "Current battery capacity: %.1f%%", capacity)
		if capacity <= minimumBatteryCapacity+lowBatteryShutdownPercent {
			if capacity <= initCapacity {
				// Report that the battery was not charging.
				return errors.Errorf("battery is not charging; initial=%.1f%%, current=%.1f%%, minimum=%.1f%%",
					initCapacity, capacity, (minimumBatteryCapacity + lowBatteryShutdownPercent))
			}
			return errors.Errorf("current battery capacity (%.1f%%) does not meet the minimum %.1f%%",
				capacity, (minimumBatteryCapacity + lowBatteryShutdownPercent))
		}

		return nil
	}, chargeBatteryTestPollOpt); err != nil {
		return errors.Wrap(err, "failed to get battery status")
	}
	return nil
}

// chargeBatteryCapacityBeforePowerTest allows charging of the battery for 8 minutes if battery capacity
// is lower than a pre-defined level when the disableChargeBatteryBeforeTest variable is not true.
// This is usually added before the case execution.
func chargeBatteryCapacityBeforePowerTest(ctx context.Context, minBatteryCapacity float64) error {
	if !pUtil.SupportChromeEC() {
		return nil
	}

	if strings.ToLower(disableChargeBatteryBeforeTest.Value()) != "true" {
		// Wait for battery to be charged.
		err := chargeBatteryCapacity(ctx, minBatteryCapacity, chargeBatteryTestPollOpt)
		if err != nil {
			if errors.Is(err, pm.ErrNoBattery) {
				testing.ContextLog(ctx, "Battery not found")
				return nil
			}
			return errors.Wrap(err, "battery failed to be charged to minimum level")
		}
	}
	return nil
}

func simulateARCBatterySaver(ctx context.Context, arc *arc.ARC) error {
	// Android's battery saver won't enable when charging, so simulate being unplugged.
	if err := arc.Command(ctx, "dumpsys", "battery", "unplug").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to unplug battery in Android")
	}

	powerd, err := power.NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Power Manager")
	}

	// Wait for battery saver to be enabled.
	if err := testing.Poll(ctx, func(context context.Context) error {
		state, err := powerd.GetBatterySaverModeState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get battery saver state")
		}
		if state.Enabled != nil && *state.Enabled {
			return nil
		}
		// Enable battery saver mode if it is not initially enabled.
		if err := powerd.SetBatterySaverModeState(ctx, true); err != nil {
			return errors.Wrap(err, "failed to toggle battery saver")
		}
		return errors.New("battery saver is not enabled")
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for battery saver to reenable")
	}
	return nil
}

func setARCLowBattery(ctx context.Context, arc *arc.ARC) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := arc.Command(ctx, "settings", "get", "global", "low_power").Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to get Android battery saver state")
		}
		if string(out) == "1\n" {
			return nil
		}
		// Enable the Android battery saver if it's not automatically enabled.
		if err := arc.Command(ctx, "settings", "put", "global", "low_power", "1").Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to enable Android battery saver")
		}
		return errors.New("Android battery saver is not on")
	}, &testing.PollOptions{Timeout: 8 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to ensure Android battery saver is on")
	}
	return nil
}

func disableARCBatterySaver(ctx context.Context, arc *arc.ARC) error {
	if err := arc.Command(ctx, "dumpsys", "battery", "reset").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to reset battery unplug in Android")
	}
	return nil
}

type prepareCUJFixture struct {
	skipCPUCooldown       bool
	chargeBattery         bool
	minBatteryRequirement float64
}

func (f *prepareCUJFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return nil
}

func (f *prepareCUJFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *prepareCUJFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *prepareCUJFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if strings.ToLower(isLocalVar.Value()) == "true" {
		s.Log("Skipping CPU cooldown because local testing variable is set")
		return
	}

	if f.minBatteryRequirement > 0 {
		if err := chargeBatteryCapacityBeforePowerTest(ctx, f.minBatteryRequirement); err != nil {
			s.Fatal("Failed to meet minimum battery capacity before test: ", err)
		}
	} else if f.chargeBattery {
		if err := chargeBatteryCapacityBeforePowerTest(ctx, minimumBatteryCapacity); err != nil {
			testing.ContextLog(ctx, "Failed to charge battery capacity before power test: ", err)
		}
	}

	if f.skipCPUCooldown {
		s.Log("Skipping CPU cooldown because of fixture")
		return
	}

	// Drop host caches for predictable results.
	if err := disk.DropCaches(ctx); err != nil {
		s.Fatal("Failed to drop caches: ", err)
	}

	// Wait for CPU stabilization and package idling.
	cujrecorder.WaitForCPUStabilization(ctx)

	// Ensure display on to record UI performance correctly. Keep trying for 2 min
	// since it could take 2 min for `powerd` dbus service to be accessible via
	// dbus from tast. See b/244752048. Also, ensure the display is on after
	// waiting for the CPU to idle, because idling could take up to 10 minutes,
	// and the display will turn off in 7.5 minutes.
	if err := testing.Poll(ctx, power.TurnOnDisplay, &testing.PollOptions{
		Interval: 10 * time.Second,
		Timeout:  2 * time.Minute,
	}); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}
}

func (f *prepareCUJFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// FixtureData is the struct returned by the preconditions.
type FixtureData struct {
	chrome *chrome.Chrome
	ARC    *arc.ARC
}

// Chrome gets the CrOS-chrome instance.
func (f FixtureData) Chrome() *chrome.Chrome { return f.chrome }

type loggedInToCUJUserFixture struct {
	cr                 *chrome.Chrome
	arc                *arc.ARC
	wr                 *wpr.WPR
	origRunningPkgs    map[string]struct{}
	logMarker          *logsaver.Marker
	keepState          bool
	chromeExtraOpts    []chrome.Option
	useEnterprisePool  bool
	fdms               *fakedms.FakeDMS
	fakeCamera         bool
	fakeCameraFileName string
	docsBlocker        bool
	disableARC         bool
	arcSupported       bool // Use ARCSupported flag instead of ARCEnabled.
	enableChromeVox    bool
	cleanupTheme       func(ctx context.Context) error
	// mlbenchmarkDataDirectory describes whether to create data directory for mlbenchmark.
	mlbenchmarkDataDirectory bool
	// If other than -1, indicates a WPR mode to work in using wprArchive.
	wprMode    wpr.Mode
	wprArchive string
	// Scx scheduler type to be loaded.
	scxType scx.Type
	// Enable "CrosBatterySaver" and "CrosBatterySaverAlwaysOn" features.
	enableBatterySaver bool
}

// NewWPRLoggedInToCUJUserWithoutCooldownFixture returns a newly created fixture object with WPR parameters
// set. This is a workaround for customizing the WPR behavior prior to b/285970864 implementation.
// Note that if mode is not wpr.Record, fixture will assume that the WPR archive is an external Data,
// in the s.DataPath(), otherwise the recorded archive will be put in the /tmp directory.
func NewWPRLoggedInToCUJUserWithoutCooldownFixture(name, desc string, contacts []string, mode wpr.Mode, archive string) *testing.Fixture {
	var data []string
	if mode != wpr.Record {
		data = append(data, archive)
	}
	return &testing.Fixture{
		Name:     name,
		Desc:     desc,
		Contacts: contacts,
		Impl: &loggedInToCUJUserFixture{
			wprMode:    mode,
			wprArchive: archive,
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Data:            data,
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
	}
}

func (f *loggedInToCUJUserFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var cr *chrome.Chrome
	var setupCompleted bool // Whether the SetUp function is successfully completed.
	disableARC := f.disableARC || !arc.Supported()

	screenshotCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if s.HasError() {
			path := filepath.Join(s.OutDir(), "fixture_failure.png")
			if err := screenshot.Capture(ctx, path); err != nil {
				s.Log("Failed to capture screenshot: ", err)
			}
		}
	}(screenshotCtx)

	func() {
		var docsBlockerExtDir string
		var err error
		var funcCompleted bool

		if f.docsBlocker {
			docsBlockerExtDir, err = prepareDocsBlockerExtension(s)
			if err != nil {
				s.Fatal("Failed to prepare DocsBlockerExtension: ", err)
			}
		}

		creds, err := loginCreds(s, f.useEnterprisePool)
		if err != nil {
			s.Fatal("Failed to obtain login credentials: ", err)
		}
		opts := []chrome.Option{
			chrome.ExtraArgs("--disable-sync", "--disable-drive-fs-for-testing"),
			chrome.DisableFeatures("PeripheralNotification"),
			chrome.DisableFeatures("CrosSodaConchLanguages"), // b:370423420 use small SODA models to avoid downloading large models on cbx duts.
			// TODO(b/472157982): Remove if the metric issue for TPS.Display.Smoothness is resolved.
			chrome.DisableFeatures("PauseMutedBackgroundAudio"),
		}
		// Enable WPR mode. Do not use GAIA login as replay won't connect to real servers.
		if f.wprArchive != "" {
			wprCtx := s.FixtContext()
			archive := filepath.Join("/tmp", f.wprArchive)
			if f.wprMode != wpr.Record {
				archive = s.DataPath(f.wprArchive)
			}
			f.wr, err = wpr.New(wprCtx, f.wprMode, archive, []string{""})
			if err != nil {
				s.Fatal("Failed to initialize WPR: ", err)
			}
			s.Log("Using FakeLogin due to WPR")
			opts = append(opts, f.wr.ChromeOptions...)
		} else {
			opts = append(opts, chrome.GAIALogin(creds))
		}
		if f.keepState {
			opts = append(opts, chrome.KeepState())
		}
		if !disableARC {
			// When arcSupported is set, use the chrome.ARCSupported flag to
			// enable the real Play Store optin procedure, so that the tests
			// can install ARC Apps and open ARC windows to do the test.
			// On the other hand, ARCEnabled flag will just bring up ARC
			// environment but ignore the PlayStore optin procedure and
			// avoid the ARC login overhead. It is used for tests involving
			// no ARC Apps.
			//
			// In tests that use ARC Supported, also use
			// ArcLmkPerceptibleMinStateUpdate, to prevent apps from being
			// killed under perceptible memory pressure.
			// TODO(b/279498529) Remove this flag when it is fully rolled out.
			if f.arcSupported {
				opts = append(opts, chrome.ARCSupported(), chrome.EnableFeatures("ArcLmkPerceptibleMinStateUpdate"))
			} else {
				opts = append(opts, chrome.ARCEnabled())
			}
			opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
			opts = append(opts, chrome.DisableFeatures("ArcExternalStorageAccess"))
			if f.useEnterprisePool {
				fdms, err := startFakeDMSWithARCEnabled(ctx, s.OutDir(), creds.User)
				if err != nil {
					s.Fatal("Failed to start fake DMS to enable ARC: ", err)
				}
				f.fdms = fdms
				defer func() {
					if !funcCompleted {
						fdms.Stop(ctx)
						f.fdms = nil
					}
				}()
				opts = append(opts,
					chrome.DMSPolicy(fdms.URL),
					// Allow accounts with any domains to be used for the test.
					chrome.DisablePolicyKeyVerification(),
				)
			}
		}
		if f.scxType != scx.TypeScxOff {
			if !scx.IsLoaded(f.scxType) {
				if err := scx.Load(ctx, s.OutDir(), f.scxType); err != nil {
					s.Fatalf("Failed to load scx %s: %v", string(f.scxType), err)
				}
			} else {
				s.Logf("scx scheduler %s has been loaded", string(f.scxType))
			}
		}
		if f.enableBatterySaver {
			opts = append(opts, chrome.EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"))
		} else {
			opts = append(opts, chrome.DisableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"))
		}
		opts = append(opts, f.chromeExtraOpts...)
		// Delay for logging memory metrics is set to 6 minutes. Considering most of CUJ tests
		// are longer than 10 minutes, this could guarantee at least one histogram for each
		// memory metric.
		const MemLogDelayArg = "--test-memory-log-delay-in-minutes=6"
		opts = append(opts,
			chrome.ExtraArgs(MemLogDelayArg))

		extraArgs := extraArgsVar.Value()
		if extraArgs != "" {
			testing.ContextLog(ctx, "Adding extra args to Chrome: ", extraArgs)
			opts = append(opts, chrome.ExtraArgs(strings.Split(extraArgs, ",")...))
		}

		extraFeatures := extraFeaturesVar.Value()
		if extraFeatures != "" {
			testing.ContextLog(ctx, "Enabling additional features: ", extraFeatures)
			opts = append(opts, chrome.EnableFeatures(strings.Split(extraFeatures, ",")...))
		}

		if f.docsBlocker {
			opts = append(opts, chrome.UnpackedExtension(docsBlockerExtDir))
		}

		if f.fakeCamera && strings.ToLower(EnableRealCameraVar.Value()) != "true" {
			fakeCameraOpts := []string{
				// See https://webrtc.github.io/webrtc-org/testing/.
				// Feed a test pattern to getUserMedia() instead of live camera input.
				// The default fps of fake device is 20.
				"--use-fake-device-for-media-stream",
				// Feed a Y4M/MJPEG test file to getUserMedia() instead of live camera input.
				"--use-file-for-fake-video-capture=" + s.DataPath(f.fakeCameraFileName),
			}
			opts = append(opts, chrome.ExtraArgs(fakeCameraOpts...))
		}

		cr, err = chrome.New(ctx, opts...)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		chrome.Lock()
		funcCompleted = true
	}()
	defer func() {
		if !setupCompleted && f.fdms != nil {
			f.fdms.Stop(ctx)
			f.fdms = nil
		}
		if !setupCompleted && f.wr != nil {
			if err := f.wr.Close(ctx); err != nil {
				s.Error("Failed to close WPR: ", err)
			}
		}
		if cr != nil {
			chrome.Unlock()
			if err := cr.Close(ctx); err != nil {
				s.Error("Failed to close Chrome: ", err)
			}
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get the test conn: ", err)
	}

	// Set shelf behavior explicitly to prevent unpredictability in
	// the shelf state at the start of each test.
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the primary display info: ", err)
	}
	if err := ash.SetShelfBehavior(ctx, tconn, info.ID, ash.ShelfBehaviorNeverAutoHide); err != nil {
		s.Fatal("Failed to set the shelf behavior to 'never auto-hide' for display ID ", info.ID)
	}

	// Create a NewScopedAutoRelease to ensure that loading the ui tree in
	// setup.TurnOnLightTheme does not interfere wih the rest of the test.
	automationAutoRelease, err := uiauto.NewScopedAutoRelease(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create automation ScopedAutoRelease: ", err)
	}
	defer automationAutoRelease.Reset(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Set the theme to light mode to ensure power usage consistency
	// between each test.
	f.cleanupTheme, err = setup.TurnOnLightTheme(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to turn on light theme: ", err)
	}

	enablePlayStore := true
	if f.keepState {
		// Check whether the play store has been enabled.
		st, err := arc.GetState(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get ARC state: ", err)
		}
		enablePlayStore = !st.Provisioned
	}

	// Do Play Store optin if needed.
	// ARC policy for Enterprise accounts is controlled by managed policies and
	// optin procedure should be skipped.
	if enablePlayStore && !disableARC && f.arcSupported && !f.useEnterprisePool {
		func() {
			const playStorePackageName = "com.android.vending"
			ctx, cancel := context.WithTimeout(ctx, optin.OptinTimeout+time.Minute)
			defer cancel()

			// Optin to Play Store.
			s.Log("Opting into Play Store")
			maxAttempts := 2
			if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
				s.Fatal("Failed to optin to Play Store: ", err)
			}

			s.Log("Waiting for Play Store shown")
			if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
				return w.ARCPackageName == playStorePackageName
			}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
				// Playstore app window might not be shown, but optin should be successful
				// at this time. Log the error message but continue.
				s.Log("Failed to wait for the Play Store window to be visible: ", err)
			} else if err := apps.Close(ctx, tconn, apps.PlayStore.ID); err != nil {
				s.Fatal("Failed to close Play Store: ", err)
			} else if err := ash.WaitForAppClosed(ctx, tconn, apps.PlayStore.ID); err != nil {
				s.Fatal("Failed to wait for Play Store to be closed: ", err)
			}
			histogram, err := metrics.WaitForHistogram(
				ctx,
				tconn,
				"Ash.ArcAppInitialAppsInstallDuration",
				10*time.Second,
			)
			if err != nil {
				s.Fatal("Failed to wait until ARC initial "+
					"apps installed: ", err)
			}
			s.Log("loggedInToCUJUserFixture: "+
				"Ash.ArcAppInitialAppsInstallDuration histogram=",
				histogram)
		}()
	}

	var a *arc.ARC
	if !disableARC {
		func() {
			ctx, cancel := context.WithTimeout(ctx, arc.BootTimeout)
			defer cancel()

			var err error
			if a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser()); err != nil {
				s.Fatal("Failed to start ARC: ", err)
			}

			if f.origRunningPkgs, err = runningPackages(ctx, a); err != nil {
				if err := a.Close(ctx); err != nil {
					s.Error("Failed to close ARC connection: ", err)
				}
				s.Fatal("Failed to list running packages: ", err)
			}
		}()
	}

	if f.enableChromeVox {
		kw, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to create a keyboard: ", err)
		}
		defer kw.Close(ctx)

		if err := crastests.Mute(ctx); err != nil {
			s.Log("Failed to mute audio: ", err)
		}

		ui := uiauto.New(tconn)
		if err := uiauto.Combine(
			"enable ChromeVox",
			kw.AccelAction("Ctrl+Alt+z"),
			ui.WaitUntilExists(nodewith.HasClass("AccessibilityBubbleContainer")),
		)(ctx); err != nil {
			s.Fatal("Failed to press Ctrl+Alt+z to enable ChromeVox: ", err)
		}
	}

	var windows []*ash.Window
	// Wait for up to 2 minutes for all windows to close.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		windows, err = ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get all windows"))
		}
		if len(windows) != 0 {
			return errors.Errorf("unexpected number of windows; got %d, expected 0", len(windows))
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 10 * time.Second,
	}); err != nil {
		var windowNames []string
		for _, w := range windows {
			windowNames = append(windowNames, w.Name)
		}
		s.Logf("Failed to wait for window(s) to close: %s", strings.Join(windowNames, ", "))
	}

	if f.mlbenchmarkDataDirectory {
		go func() {
			<-s.FixtContext().Done()
			// Make sure mlbenchmark data directory is removed when fixture is done.
			if _, err := os.Stat(mlbenchmark.DataDirectory); !os.IsNotExist(err) {
				if err := os.RemoveAll(mlbenchmark.DataDirectory); err != nil {
					s.Logf("Failed to clear data directory %s: %v", mlbenchmark.DataDirectory, err)
				}
			}
		}()
	}

	f.cr = cr
	f.arc = a
	cr = nil
	setupCompleted = true

	return FixtureData{chrome: f.cr, ARC: f.arc}
}

func (f *loggedInToCUJUserFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()

	if f.cleanupTheme != nil {
		if err := f.cleanupTheme(ctx); err != nil {
			s.Log("Failed to cleanup theme: ", err)
		}
	}

	if f.enableChromeVox {
		tconn, err := f.cr.TestAPIConn(ctx)
		if err != nil {
			s.Log("Failed to get the test conn: ", err)
		}

		kw, err := input.Keyboard(ctx)
		if err != nil {
			s.Log("Failed to create keyboard: ", err)
		}
		defer kw.Close(ctx)

		ui := uiauto.New(tconn)
		if err := uiauto.Combine(
			"disable ChromeVox",
			kw.AccelAction("Ctrl+Alt+z"),
			ui.WaitUntilGone(nodewith.HasClass("AccessibilityBubbleContainer")),
		)(ctx); err != nil {
			s.Log("Failed to disable ChromeVox: ", err)
		}

		if err := crastests.Unmute(ctx); err != nil {
			s.Log("Failed to unmute audio: ", err)
		}
	}

	if f.arc != nil {
		if err := f.arc.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close ARC connection: ", err)
		}
		if err := disableARCBatterySaver(ctx, f.arc); err != nil {
			testing.ContextLog(ctx, "Failed to disable ARC battery saver: ", err)
		}
	}

	if err := f.cr.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome connection: ", err)
	}

	if f.wr != nil {
		if err := f.wr.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close WPR: ", err)
		}
	}

	if f.fdms != nil {
		f.fdms.Stop(ctx)
		f.fdms = nil
	}

	if f.scxType != scx.TypeScxOff {
		if err := scx.Unload(ctx, f.scxType); err != nil {
			testing.ContextLogf(ctx, "Failed to unload scx %s: %v", string(f.scxType), err)
		}
	}
}

func (f *loggedInToCUJUserFixture) Reset(ctx context.Context) error {
	// Check oauth2 token is still valid. If not, return an error to restart
	// chrome and re-login.
	tconn, err := f.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the test conn")
	}
	if st, err := lockscreen.GetState(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to get login status")
	} else if !st.HasValidOauth2Token {
		return errors.New("invalid oauth2 token")
	}

	if f.arc != nil {
		// Stopping the running apps.
		running, err := runningPackages(ctx, f.arc)
		if err != nil {
			return errors.Wrap(err, "failed to get running packages")
		}
		for pkg := range running {
			if _, ok := f.origRunningPkgs[pkg]; ok {
				continue
			}
			testing.ContextLogf(ctx, "Stopping package %q", pkg)
			if err := f.arc.Command(ctx, "am", "force-stop", pkg).Run(testexec.DumpLogOnError); err != nil {
				return errors.Wrapf(err, "failed to stop %q", pkg)
			}
		}
	}

	// Unlike ARC.preImpl, this does not uninstall apps. This is because we
	// typically want to reuse the same list of applications, and additional
	// installed apps wouldn't affect the test scenarios.
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed to reset chrome")
	}

	// Ensures that there are no toplevel windows left open.
	if all, err := ash.GetAllWindows(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to call ash.GetAllWindows")
	} else if len(all) != 0 {
		return errors.Wrapf(err, "toplevel window (%q) stayed open, total %d left", all[0].Name, len(all))
	}

	return nil
}

func (f *loggedInToCUJUserFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Some tests that utilizes on-device ML will load models in odmld. We should restart odmld
	// to make sure its model/cache states are clean.
	var odmlDaemon = &hwsec.DaemonInfo{
		Name:       "odml",
		DaemonName: "odmld",
		HasDBus:    false,
	}
	cmdRunner := hwseclocal.NewCmdRunner()
	daemonController := hwsec.NewDaemonController(cmdRunner)
	if err := daemonController.Restart(ctx, odmlDaemon); err != nil {
		s.Log("Failed to restart odmld: ", err)
	} else {
		s.Log("Successfully restarted odmld")
	}

	if f.mlbenchmarkDataDirectory {
		if _, err := os.Stat(mlbenchmark.DataDirectory); !os.IsNotExist(err) {
			if err := os.RemoveAll(mlbenchmark.DataDirectory); err != nil {
				s.Logf("Failed to clear data directory %s: %v", mlbenchmark.DataDirectory, err)
			}
		}
		if err := os.MkdirAll(mlbenchmark.DataDirectory, 0755); err != nil {
			s.Logf("Failed to create data directory %s: %v", mlbenchmark.DataDirectory, err)
		}
	}

	if f.arc != nil {
		arcLogOutDir := filepath.Join(s.OutDir(), "arc_logs")
		if err := os.MkdirAll(arcLogOutDir, 0755); err != nil {
			s.Log("Error creating arc_logs directory: ", err)
			arcLogOutDir = s.OutDir()
		} else {
			s.Log("Created arc_logs directory successfully")
		}
		if err := f.arc.ResetOutDir(ctx, arcLogOutDir); err != nil {
			s.Log("Failed to reset outDir field of ARC object: ", err)
		}
	}

	if f.logMarker != nil {
		s.Log("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(f.cr.LogFilename())
	if err == nil {
		f.logMarker = logMarker
	} else {
		s.Log("Failed to start the log saver: ", err)
	}

	webRTCLogs, err := filepath.Glob(webRTCEventLogFilePattern)
	if err != nil {
		s.Log("Failed to check for WebRTC event log files before test: ", err)
	}
	if len(webRTCLogs) == 0 {
		return
	}
	s.Log("Deleting WebRTC event log files found in /tmp before test: ", webRTCLogs)
	for _, filename := range webRTCLogs {
		if err := os.Remove(filename); err != nil {
			s.Logf("Failed to delete %q: %s", filename, err)
		}
	}
}

func (f *loggedInToCUJUserFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.mlbenchmarkDataDirectory {
		if _, err := os.Stat(mlbenchmark.DataDirectory); !os.IsNotExist(err) {
			if err := os.RemoveAll(mlbenchmark.DataDirectory); err != nil {
				s.Logf("Failed to clear data directory %s: %v", mlbenchmark.DataDirectory, err)
			}
		}
	}

	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "chrome.log")); err != nil {
			s.Log("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}

	if f.arc != nil {
		if err := f.arc.SaveLogFiles(ctx); err != nil {
			s.Log("Failed to save ARC-related log files: ", err)
		} else {
			s.Log("ARC-related log files saved successfully")
		}
	}

	webRTCLogs, err := filepath.Glob(webRTCEventLogFilePattern)
	if err != nil {
		s.Log("Failed to check for WebRTC event log files after test: ", err)
	}
	if len(webRTCLogs) == 0 {
		return
	}
	s.Log("Gathering WebRTC event log files: ", webRTCLogs)
	if err := testexec.CommandContext(ctx, "tar",
		append(
			[]string{"-cvzf", path.Join(s.OutDir(), "webrtc-logs.tar.gz")},
			webRTCLogs...,
		)...,
	).Run(testexec.DumpLogOnError); err != nil {
		s.Log("Failed to gather WebRTC event log files: ", err)
	}
	s.Log("Deleting WebRTC event log files in /tmp")
	for _, filename := range webRTCLogs {
		if err := os.Remove(filename); err != nil {
			s.Logf("Failed to delete %q: %s", filename, err)
		}
	}
}

// androidBatterySaverFixture does the extra work needed to enable battery
// saver on Android.
// Android won't turn on battery saver unless the device is disconnected from
// power. So we run `dumpsys battery unplug` to simulate it, and then toggle
// battery saver to properly propagate the state into Android.
type androidBatterySaverFixture struct {
	arc *arc.ARC
}

func (f *androidBatterySaverFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	value := s.ParentValue().(FixtureData)
	if !arc.Supported() {
		return value
	}

	f.arc = value.ARC
	if err := simulateARCBatterySaver(ctx, f.arc); err != nil {
		s.Fatal("Failed to simulate Android battery saver: ", err)
	}

	return value
}

func (f *androidBatterySaverFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if !arc.Supported() {
		return
	}

	if err := disableARCBatterySaver(ctx, f.arc); err != nil {
		s.Fatal("Failed to disable Android battery saver: ", err)
	}
	f.arc = nil
}

func (f *androidBatterySaverFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *androidBatterySaverFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if !arc.Supported() {
		return
	}

	if err := setARCLowBattery(ctx, f.arc); err != nil {
		s.Fatal("Failed to set ARC low battery: ", err)
	}
}

func (f *androidBatterySaverFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
