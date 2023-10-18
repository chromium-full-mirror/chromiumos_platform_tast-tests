// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// CPUStablizationTimeout is the time to wait for cpu stablization, which
	// is the sum of cpu cool down time, cpu idle time, and cpu pkg state activity drop time.
	CPUStablizationTimeout = cujrecorder.CooldownTimeout
	// BatteryChargingTimeout is the battery charging duration if capacity is
	//  below 25%
	BatteryChargingTimeout = 3 * time.Minute
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

	hrtimerPath = "/proc/sys/kernel/timer_highres"
)

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
var chargeBatteryTestPollOpt = &testing.PollOptions{Interval: 60 * time.Second, Timeout: 3 * time.Minute}

// EnableWaylandLoggingVar is a runtime variable that specifies
// whether to enable Wayland logging into Lacros logs.
var EnableWaylandLoggingVar = testing.RegisterVarString(
	"cuj.enableWaylandLogging",
	"",
	"A boolean string (true/false) signifying whether to enable Wayland logging into Lacros logs",
)

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
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl:            &prepareCUJFixture{},
		PreTestTimeout:  CPUStablizationTimeout,
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
		Impl:            &prepareCUJFixture{skipCPUCooldown: true},
		PreTestTimeout:  CPUStablizationTimeout,
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
		Impl:            &prepareCUJFixture{chargeBattery: true},
		PreTestTimeout:  CPUStablizationTimeout + BatteryChargingTimeout + 5*time.Second,
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
		Impl:            &prepareCUJFixture{chargeBattery: true},
		SetUpTimeout:    chrome.EnrollmentAndLoginTimeout + chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PreTestTimeout:  CPUStablizationTimeout + BatteryChargingTimeout + 5*time.Second,
		PostTestTimeout: postTestTimeout,
		Parent:          "gpuWatchHangsEnrolled",
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUser",
		Desc: "The main fixture used for UI CUJ tests",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{bt: browser.TypeAsh},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
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
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:           browser.TypeAsh,
			arcSupported: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
		Desc: "The main fixture used for UI CUJ tests using an enterprise account, with WebRTC event logging",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:                browser.TypeAsh,
			useEnterprisePool: true,
			docsBlocker:       true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujEnterpriseAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepState",
		Desc: "The CUJ test fixture which keeps login state",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			keepState: true,
			bt:        browser.TypeAsh,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateARCSupported",
		Desc: "The CUJ test fixture which keeps login state with ARC supported",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			keepState:    true,
			bt:           browser.TypeAsh,
			arcSupported: true,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithBatterySaverParent",
		Desc: "The CUJ test fixture which keeps login state and turns on battery saver without Android battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"chromeos-perfmetrics-eng@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			keepState: true,
			bt:        browser.TypeAsh,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--allow-insecure-localhost"),
				chrome.EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"),
			},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithBatterySaver",
		Desc: "The CUJ test fixture which keeps login state and turns on battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInAndKeepStateWithBatterySaverParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithHighResTimerOff",
		Desc: "The CUJ test fixture which keeps login state and high res timer off",
		Contacts: []string{
			"hsinyi@google.com",
			"joelaf@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:         browser.TypeAsh,
			keepState:  true,
			hrtimerOff: true,
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--allow-insecure-localhost"),
			},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithFakeCamera",
		Desc: "The CUJ test fixture which keeps login state and uses fake camera",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Data: []string{highResFakeCameraFileName},
		Impl: &loggedInToCUJUserFixture{
			bt:                 browser.TypeAsh,
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: highResFakeCameraFileName,
			chromeExtraOpts:    []chrome.Option{chrome.ExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateWithLowResFakeCamera",
		Desc: "The CUJ test fixture which keeps login state and uses low resolution fake camera",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Data: append(docsBlockerFiles, lowResFakeCameraFileName),
		Impl: &loggedInToCUJUserFixture{
			bt:                 browser.TypeAsh,
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: lowResFakeCameraFileName},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserLacros",
		Desc: "Fixture used for lacros variation of UI CUJ tests",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{bt: browser.TypeLacros},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedLacros",
		Desc: "Fixture used for lacros variation of UI CUJ tests with ARC supported",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:           browser.TypeLacros,
			arcSupported: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateLacros",
		Desc: "Fixture keeping login status and used for lacros variation of CUJ tests",
		Contacts: []string{
			"xliu@cienet.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			keepState: true,
			bt:        browser.TypeLacros,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.LacrosExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateARCSupportedLacros",
		Desc: "Fixture keeping login status and used for lacros variation of CUJ tests with ARC supported",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			keepState:    true,
			arcSupported: true,
			bt:           browser.TypeLacros,
			// Some tests will connect to websites hosted locally with HTTPs and this flag allows invalid certificates for resources loaded from localhost.
			chromeExtraOpts: []chrome.Option{chrome.LacrosExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateLacrosWithFakeCamera",
		Desc: "Fixture keeping login status, used fake camera for lacros variation of CUJ tests",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Data: []string{highResFakeCameraFileName},
		Impl: &loggedInToCUJUserFixture{
			bt:                 browser.TypeLacros,
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: highResFakeCameraFileName,
			chromeExtraOpts:    []chrome.Option{chrome.LacrosExtraArgs("--allow-insecure-localhost")},
		},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInAndKeepStateLacrosWithLowResFakeCamera",
		Desc: "Fixture keeping login status, used low resolution fake camera for lacros variation of CUJ tests",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Data: append(docsBlockerFiles, lowResFakeCameraFileName),
		Impl: &loggedInToCUJUserFixture{
			bt:                 browser.TypeLacros,
			keepState:          true,
			fakeCamera:         true,
			fakeCameraFileName: lowResFakeCameraFileName},
		Parent:          "prepareForCUJWithCharge",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "enrolledLoggedInToCUJUser",
		Desc: "Logged in with gaia user on an enrolled device",
		Contacts: []string{
			"alston.huang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{},
		Parent:          "prepareForCUJEnrolledWithCharge",
		SetUpTimeout:    chrome.EnrollmentAndLoginTimeout + chrome.GAIALoginTimeout + optin.OptinTimeout + 2*time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars: []string{
			"ui.cujAccountPool",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "enrolledLoggedInToCUJUserLacros",
		Desc: "Logged in with gaia user on an enrolled device and used for lacros variation of CUJ tests",
		Contacts: []string{
			"jane.yang@cienet.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{bt: browser.TypeLacros},
		Parent:          "prepareForCUJEnrolledWithCharge",
		SetUpTimeout:    chrome.EnrollmentAndLoginTimeout + chrome.GAIALoginTimeout + optin.OptinTimeout + 2*time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars: []string{
			"ui.cujAccountPool",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLogging",
		Desc: "CUJ test fixture with WebRTC event logging",
		Contacts: []string{
			"ramsaroop@chromium.org",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingLacros",
		Desc: "Lacros variation of loggedInToCUJUserWithWebRTCEventLogging",
		Contacts: []string{
			"ramsaroop@chromium.org",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeLacros,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithOneGroupPerRenderer",
		Desc: "CUJ test fixture with the OneGroupPerRenderer feature enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("OneGroupPerRenderer", "PreferConstantFrameRate"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	// TODO(crbug/1410581): Remove this variant when done with testing.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithMainThreadCompositingPriority",
		Desc: "CUJ test fixture with the MainThreadCompositingPriority feature enabled",
		Contacts: []string{
			"youssefesmat@google.com",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("MainThreadCompositingPriority", "PreferConstantFrameRate"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
		Desc: "CUJ test fixture with WebRTC event logging and VC platform effects enabled",
		Contacts: []string{
			"ramsaroop@chromium.org",
			"sammc@chromium.org",
			"cros-sw-perf@google.com",
			"cros-pe-pnp@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
				chrome.EnableFeatures(
					"PreferConstantFrameRate",
					"CrOSLateBootAudioAPNoiseCancellation",
					"CrOSLateBootAudioFlexibleLoopback",
					"ShowLiveCaptionInVideoConferenceTray",
					"SystemLiveCaption",
					"VideoConference",
				),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsLacros",
		Desc: "CUJ test fixture with WebRTC event logging, VC platform effects enabled and lacros",
		Contacts: []string{
			"ramsaroop@chromium.org",
			"sammc@chromium.org",
			"cros-sw-perf@google.com",
			"cros-pe-pnp@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
				chrome.EnableFeatures(
					"PreferConstantFrameRate",
					"CrOSLateBootAudioAPNoiseCancellation",
					"CrOSLateBootAudioFlexibleLoopback",
					"ShowLiveCaptionInVideoConferenceTray",
					"SystemLiveCaption",
					"VideoConference",
				),
			},
			bt:          browser.TypeLacros,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingAndHighResTimerOff",
		Desc: "CUJ test fixture with WebRTC event logging and turn off hrtimer",
		Contacts: []string{
			"hsinyi@google.com",
			"joelaf@google.com",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
			hrtimerOff:  true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFieldTrials",
		Desc: "CUJ fixture with all field trials enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-field-trial-config"),
			},
			bt: browser.TypeAsh,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedWithFieldTrials",
		Desc: "CUJ fixture with ARC supported and all field trials",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs("--enable-field-trial-config"),
			},
			bt:           browser.TypeAsh,
			arcSupported: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithFieldTrialsAndWebRTCEventLogging",
		Desc: "Variant of loggedInToCUJUserWithFieldTrials with WebRTCEventLogging enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("PreferConstantFrameRate"),
				chrome.ExtraArgs("--enable-field-trial-config"),
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserLacrosWithoutARC",
		Desc: "Variant of loggedInToCUJUser with ARC disabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:         browser.TypeLacros,
			disableARC: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    chrome.GAIALoginTimeout + 2*time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithAppRescue",
		Desc: "CUJ fixture with AppRescue enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("ArcMglruReclaim:interval/30000/swappiness/60,ArcLmkPerceptibleMinStateUpdate"),
			},
			bt: browser.TypeAsh,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverParent",
		Desc: "CUJ test fixture with battery saver without Android",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt: browser.TypeAsh,
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithHighResTimerOff",
		Desc: "CUJ test fixture with high res timer off",
		Contacts: []string{
			"hsinyi@google.com",
			"joelaf@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:         browser.TypeAsh,
			hrtimerOff: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedWithBatterySaverParent",
		Desc: "CUJ test fixture with ARC supported and battery saver without Android",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:           browser.TypeAsh,
			arcSupported: true,
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"),
			},
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedWithHighResTimerOff",
		Desc: "CUJ test fixture with ARC supported and high res timer off",
		Contacts: []string{
			"hsinyi@google.com",
			"joelaf@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:           browser.TypeAsh,
			arcSupported: true,
			hrtimerOff:   true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpWithOptinTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaver",
		Desc: "CUJ test fixture with battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserWithBatterySaverParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserARCSupportedWithBatterySaver",
		Desc: "CUJ test fixture with ARC supported and battery saver",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserARCSupportedWithBatterySaverParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithWebRTCEventLoggingAndBatterySaver",
		Desc: "CUJ test fixture with WebRTC event logging and battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Data: docsBlockerFiles,
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.ExtraArgs(webRTCEventLogCommandFlag),
				chrome.EnableFeatures(
					"CrosBatterySaver",
					"CrosBatterySaverAlwaysOn",
					"PreferConstantFrameRate",
				),
			},
			bt:          browser.TypeAsh,
			docsBlocker: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithChromeVox",
		Desc: "CUJ fixture with ChromeVox enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt:              browser.TypeAsh,
			enableChromeVox: true,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown",
		Contacts: []string{
			"vincentchiang@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{bt: browser.TypeAsh},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverWithoutCooldownParent",
		Desc: "CUJ fixture that skips CPU cooldown and has battery saver active without Android battery saver",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"chromeos-perfmetrics-eng@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			bt: browser.TypeAsh,
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("CrosBatterySaver", "CrosBatterySaverAlwaysOn"),
			},
		},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserWithBatterySaverWithoutCooldown",
		Desc: "CUJ fixture that skips CPU cooldown and has battery saver active",
		Contacts: []string{
			"cwd@google.com",
			"cros-vm-technology@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &androidBatterySaverFixture{},
		Parent:          "loggedInToCUJUserWithBatterySaverWithoutCooldownParent",
		SetUpTimeout:    batterySaverTimeout,
		TearDownTimeout: batterySaverTimeout,
		PreTestTimeout:  batterySaverTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserLacrosWithoutCooldown",
		Desc: "Lacros CUJ fixture that skips CPU cooldown",
		Contacts: []string{
			"vincentchiang@google.com",
			"cros-sw-perf@google.com",
		},
		Impl:            &loggedInToCUJUserFixture{bt: browser.TypeLacros},
		Parent:          "prepareForCUJWithoutCooldown",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
	// TODO(b/292249282): Remove when Vulkan is launched on brya and volteer.
	testing.AddFixture(&testing.Fixture{
		Name: "loggedInToCUJUserVulkan",
		Desc: "CUJ fixture that enables Vulkan for raster/composite",
		Contacts: []string{
			"hob@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl: &loggedInToCUJUserFixture{
			chromeExtraOpts: []chrome.Option{
				chrome.EnableFeatures("Vulkan"),
			},
			bt: browser.TypeAsh,
		},
		Parent:          "prepareForCUJ",
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  CPUStablizationTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ui.cujAccountPool"},
	})
}

func prepareDocsBlockerExtension(s *testing.FixtState) (string, error) {
	extDir, err := ioutil.TempDir("", "docs_blocker_extension")
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
func GetDocsBlockerConn(ctx context.Context, br *browser.Browser) (*browser.Conn, error) {
	bgURL := "chrome-extension://" + docsBlockerExtensionID + "/_generated_background_page.html"

	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		return nil, errors.Wrap(err, "DocsBlocker extension not found")
	}

	if err = conn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "failed to wait for DocsBlocker extension")
	}
	return conn, nil
}

func loginCreds(s *testing.FixtState, useEnterprisePool bool) (credconfig.Creds, error) {
	var variableName string

	if useEnterprisePool {
		variableName = "ui.cujEnterpriseAccountPool"
	} else {
		variableName = "ui.cujAccountPool"
	}

	return credconfig.PickRandomCreds(s.RequiredVar(variableName))
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

// chargeBatteryCapacity allows charging of the battery for 3 minutes if battery capacity
// is not higher than a pre-defined level (minimumBatteryCapacity+lowBatteryShutdownPercent).
func chargeBatteryCapacity(ctx context.Context, minimumBatteryCapacity float64, chargeBatteryTestPollOpt *testing.PollOptions) error {
	if err := setup.AllowBatteryCharging(ctx); err != nil {
		return err
	}
	devPath, err := power.SysfsBatteryPath(ctx)
	if err != nil {
		return err
	}
	lowBatteryShutdownPercent, err := power.LowBatteryShutdownPercent(ctx)
	if err != nil {
		return err
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		capacity, err := power.ReadBatteryCapacity(ctx, devPath)
		if err != nil {
			return errors.Wrap(err, "failed to get battery capacity")
		}
		testing.ContextLogf(ctx, "Current battery capacity: %.1f%%", capacity)
		if capacity <= minimumBatteryCapacity+lowBatteryShutdownPercent {
			return errors.New("current battery capacity is not higher than minimum")
		}
		return nil
	}, chargeBatteryTestPollOpt); err != nil {
		return errors.Wrap(err, "failed to get battery status")
	}
	return nil
}

// ChargeBatteryCapacityBeforePowerTest allows charging of the battery for 3 minutes if battery capacity
// is lower than a pre-defined level when the disableChargeBatteryBeforeTest variable is not true.
// This is usually added before the case execution.
func ChargeBatteryCapacityBeforePowerTest(ctx context.Context) error {
	if strings.ToLower(disableChargeBatteryBeforeTest.Value()) != "true" {
		// Wait for battery to be charged.
		err := chargeBatteryCapacity(ctx, minimumBatteryCapacity, chargeBatteryTestPollOpt)
		if err != nil {
			if errors.Is(err, power.ErrNoBattery) {
				return errors.Wrap(err, "battery not found")
			}
			return errors.Wrap(err, "battery failed to be charged to minimum level")
		}
	}
	return nil
}

type prepareCUJFixture struct {
	skipCPUCooldown bool
	chargeBattery   bool
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

	if f.skipCPUCooldown {
		s.Log("Skipping CPU cooldown because of fixture")
		return
	}

	if f.chargeBattery {
		if err := ChargeBatteryCapacityBeforePowerTest(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to charge battery capacity before power test: ", err)
		}
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
	cr              *chrome.Chrome
	arc             *arc.ARC
	origRunningPkgs map[string]struct{}
	logMarker       *logsaver.Marker
	keepState       bool
	chromeExtraOpts []chrome.Option
	// bt describes what type of browser this fixture should use
	bt                 browser.Type
	useEnterprisePool  bool
	fdms               *fakedms.FakeDMS
	fakeCamera         bool
	fakeCameraFileName string
	docsBlocker        bool
	disableARC         bool
	arcSupported       bool // Use ARCSupported flag instead of ARCEnabled.
	enableChromeVox    bool
	hrtimerOff         bool // Turn off highres timer. TODO(b/298151007): Remove when sufficient data is collected.
	cleanupTheme       func(ctx context.Context) error
}

func (f *loggedInToCUJUserFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var cr *chrome.Chrome
	var setupCompleted bool // Whether the SetUp function is successfully completed.

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
			chrome.GAIALogin(creds),
			chrome.ExtraArgs("--disable-sync"),
		}
		if f.keepState {
			opts = append(opts, chrome.KeepState())
		}
		if f.hrtimerOff {
			if err := os.WriteFile(hrtimerPath, []byte("0"), 0); err != nil {
				s.Fatal("Failed to turn off hrtimer: ", err)
			}
		}
		if !f.disableARC {
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
		opts = append(opts, f.chromeExtraOpts...)
		// Delay for logging memory metrics is set to 6 minutes. Considering most of CUJ tests
		// are longer than 10 minutes, this could guarantee at least one histogram for each
		// memory metric.
		const MemLogDelayArg = "--test-memory-log-delay-in-minutes=6"
		opts = append(opts,
			chrome.ExtraArgs(MemLogDelayArg),
			chrome.LacrosExtraArgs(MemLogDelayArg))

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

		if f.bt == browser.TypeLacros {
			if strings.ToLower(EnableWaylandLoggingVar.Value()) == "true" {
				opts = append(opts, chrome.ExtraArgs("--lacros-chrome-additional-env=WAYLAND_DEBUG=1"))
			}

			opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
			if err != nil {
				s.Fatal("Failed to get lacros options: ", err)
			}

			if f.docsBlocker {
				opts = append(opts, chrome.LacrosUnpackedExtension(docsBlockerExtDir))
			}

			opts = append(opts, chrome.EnableFeatures("LacrosProfileMigrationForceOff"))
		} else {
			if f.docsBlocker {
				opts = append(opts, chrome.UnpackedExtension(docsBlockerExtDir))
			}
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
	if enablePlayStore && !f.disableARC && f.arcSupported && !f.useEnterprisePool {
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
	if !f.disableARC {
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

		if err := crastestclient.Mute(ctx); err != nil {
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

	if f.hrtimerOff {
		if err := os.WriteFile(hrtimerPath, []byte("1"), 0); err != nil {
			s.Fatal("Failed to reset hrtimer: ", err)
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

		if err := crastestclient.Unmute(ctx); err != nil {
			s.Log("Failed to unmute audio: ", err)
		}
	}

	if f.arc != nil {
		if err := f.arc.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close ARC connection: ", err)
		}
	}

	if err := f.cr.Close(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome connection: ", err)
	}

	if f.fdms != nil {
		f.fdms.Stop(ctx)
		f.fdms = nil
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
// battery saver to properly propegate the state into Android.
type androidBatterySaverFixture struct {
	arc *arc.ARC
}

func (f *androidBatterySaverFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	value := s.ParentValue().(FixtureData)
	f.arc = value.ARC

	// Android's battery saver won't enable when charging, so simulate being unplugged.
	if err := f.arc.Command(ctx, "dumpsys", "battery", "unplug").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to unplug battery in Android: ", err)
	}

	powerd, err := power.NewPowerManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Power Manager: ", err)
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
		s.Fatal("Failed to wait for battery saver to reenable: ", err)
	}

	return value
}

func (f *androidBatterySaverFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.arc.Command(ctx, "dumpsys", "battery", "reset").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to reset battery unplug in Android: ", err)
	}
	f.arc = nil
}

func (f *androidBatterySaverFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *androidBatterySaverFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := f.arc.Command(ctx, "settings", "get", "global", "low_power").Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to get Android battery saver state")
		}
		if string(out) == "1\n" {
			return nil
		}
		// Enable the Android battery saver if it's not automatically enabled.
		s.Log("Enable Android battery saver")
		if err := f.arc.Command(ctx, "settings", "put", "global", "low_power", "1").Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to enable Android battery saver")
		}
		return errors.New("Android battery saver is not on")
	}, &testing.PollOptions{Timeout: 8 * time.Second}); err != nil {
		s.Fatal("Failed to ensure Android battery saver is on: ", err)
	}
}

func (f *androidBatterySaverFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
