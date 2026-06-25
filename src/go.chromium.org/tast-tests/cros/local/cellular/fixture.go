// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/cellular"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/modemloggerd"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/starfish"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
)

const (
	uiJobName    = "ui"
	tearDownTime = 5 * time.Minute
	// EcResetOnResetRequestedFlagPath is set when we want the remote cellular
	// fixture to ec reset the DUT on Reset(). All contents of /run get erased on reboot.
	EcResetOnResetRequestedFlagPath = "/run/cellularEcResetOnResetRequestedFlagPath"
	// RebootOnTearDownRequestedFlagPath is set when we want the remote cellular
	// fixture to reboot the DUT on TearDown(). All contents of /run get erased on reboot.
	RebootOnTearDownRequestedFlagPath = "/run/cellularRebootOnTearDownRequestedFlagPath"
)

// The Cellular test fixture ensures that modemfwd is stopped.

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "cellular",
		Desc:            "Cellular tests are safe to run",
		Contacts:        []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularNoUI",
		Desc:            "Cellular tests without UI are safe to run",
		Contacts:        []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setStopUI(true),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularRebootSetupLocal",
		Desc:            "Cellular fixture that reboots in SetUp",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRebootSetupRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularSuspendLocal",
		Desc:            "Cellular suspend-resume fixture that reboots to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularSuspendRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularStressLocal",
		Desc:            "Cellular stress fixture that reboots to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularStressRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularE2ELocal",
		Desc:            "Cellular e2e fixture that reboots to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularE2ERemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularHotspotLocal",
		Desc:            "Cellular hotspot fixture that reboots to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularHotspotRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularDUTCheckLocal",
		Desc:            "Cellular dut-check fixture that reboots to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularDUTCheckRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularAutoconnectLocal",
		Desc:            "Cellular autoconnect fixture that reboots in SetUp",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularAutoconnectRemote",
		Impl:            newCellularFixture(),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularTestESIM",
		Desc:            "Cellular tests are safe to run with a Test SIM",
		Contacts:        []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setUseTestESIM(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolled",
		Desc:            "Cellular tests are safe to run and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jiajunz@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    3*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Impl:            newCellularFixture().setUseFakeDMS(true),
		// TODO: Create parent fixture to combine cellularRemoteRebootSupport with fixture.FakeDMSEnrolled
		Parent: fixture.FakeDMSEnrolled,
		Vars:   []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolledAndTestSIM",
		Desc:            "Cellular tests are safe to run that require a Test SIM and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jiajunz@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    3*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Impl:            newCellularFixture().setUseFakeDMS(true).setUseTestESIM(true),
		// TODO: Create parent fixture to combine cellularRemoteRebootSupport with fixture.FakeDMSEnrolled
		Parent: fixture.FakeDMSEnrolled,
		Vars:   []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolledAndSIMLockCleared",
		Desc:            "Cellular tests are safe to run that require a functioning SIM and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"cros-device-enablement@google.com", "jiajunz@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    3*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Impl:            newCellularFixture().setUseFakeDMS(true).setClearSIMLock(true),
		// TODO: Create parent fixture to combine cellularRemoteRebootSupport with fixture.FakeDMSEnrolled
		Parent: fixture.FakeDMSEnrolled,
		Vars:   []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "cellularModemManager",
		Desc: "ModemManager tests are safe to run without shill running",
		Contacts: []string{
			"andrewlassalle@google.com",
			"chromeos-cellular-team@google.com",
		},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    5*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setRestartMM(true).setRestartOnFailure([]string{modemmanager.JobName}).setDaemonUptimeBeforeTest(0 * time.Second).setDisableCellularInShill(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "cellularResetShillProfileOnPostTest",
		Desc: "Tests that require shill to be reset after each test. This includes resetting the default.profile",
		Contacts: []string{
			"andrewlassalle@google.com",
			"chromeos-cellular-team@google.com",
		},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    5*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setRestartOnFailure([]string{modemmanager.JobName}).setResetShillProfileOnPostTest(true).setDaemonUptimeBeforeTest(0 * time.Second),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "cellularNoUIResetShillProfileOnPostTest",
		Desc: "Tests that require shill to be reset after each test. This includes resetting the default.profile. Tests run without UI",
		Contacts: []string{
			"andrewlassalle@google.com",
			"chromeos-cellular-team@google.com",
		},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    5*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setRestartOnFailure([]string{modemmanager.JobName}).setResetShillProfileOnPostTest(true).setDaemonUptimeBeforeTest(0 * time.Second).setStopUI(true),
		Vars:            []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularArcBooted",
		Desc:            "Arc tests on cellular interface",
		Contacts:        []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Impl:            newCellularFixture().setHasArc(true),
		// TODO: Create parent fixture to combine cellularRemoteRebootSupport with arcBooted
		Parent: "arcBooted",
		Vars:   []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFunctioningRoamingSim",
		Desc:            "Cellular tests that require a functioning roaming SIM are safe to run",
		Contacts:        []string{"cros-device-enablement@google.com", "nikhilcn@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setUseRoaming(true).setCheckSIM(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFunctioningSim",
		Desc:            "Cellular tests that require a functioning SIM are safe to run",
		Contacts:        []string{"cros-device-enablement@google.com", "nikhilcn@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setCheckSIM(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularPower",
		Desc:            "Power tests for cellular connectivity",
		Contacts:        []string{"chromeos-cellular-team@google.com", "rmao@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Impl:            newCellularFixture().setCheckSIM(true),
		// TODO: Create parent fixture to combine cellularRemoteRebootSupport with powerMetricsNoUI
		Parent: "powerMetricsNoUI",
		Vars:   []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularSIMLockCleared",
		Desc:            "Cellular tests that may affect SIM lock",
		Contacts:        []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularRemoteRebootSupport",
		Impl:            newCellularFixture().setClearSIMLock(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithChrome",
		Desc:            "Cellular tests that require Chrome login first",
		Contacts:        []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularE2ERemote",
		Impl:            newCellularFixture().setHasChrome(true),
		Vars:            []string{"autotest_host_info_labels"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularEnforceConnectionLocal",
		Desc:            "Cellular fixture that enforces that the network is connectable as part of the reset, if not then the DUT is rebooted",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    4*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Parent:          "cellularEnforceConnectionRemote",
		// We dont need to reset on failure since we will be rebooting if we can't connect.
		Impl: newCellularFixture().setRestartOnFailure([]string{}),
		Vars: []string{"autotest_host_info_labels"},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "cellularEnforceConnectionAndResetShillProfile",
		Desc: "Cellular fixture that resets shill after each test and enforces DUT is in a connectable state",
		Contacts: []string{
			"jstanko@google.com",
			"chromeos-cellular-team@google.com",
		},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    5*time.Minute + tearDownTime,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: tearDownTime,
		Vars:            []string{"autotest_host_info_labels"},
		Impl:            newCellularFixture().setRestartOnFailure([]string{modemmanager.JobName}).setResetShillProfileOnPostTest(true).setDaemonUptimeBeforeTest(0 * time.Second),
		Parent:          "cellularEnforceConnectionRemote",
	})
}

// cellularFixture implements testing.FixtureImpl.
type cellularFixture struct {
	// Fixture control flags
	restartMM                   bool
	stopUI                      bool
	useFakeDMS                  bool
	useRoaming                  bool
	useTestESIM                 bool
	checkSIM                    bool
	clearSIMLock                bool
	hasArc                      bool
	hasChrome                   bool
	resetShillProfileOnPostTest bool
	restartOnFailure            []string
	daemonUptimeBeforeTest      time.Duration
	systemUptimeBeforeTest      time.Duration
	disableCellularInShill      bool
	// Fixture variables
	crashFilesTracker    []string
	helper               *Helper
	modemfwdStopped      bool
	modemLoggingStarted  bool
	sf                   *starfish.Starfish
	cr                   *chrome.Chrome
	netUnlock            func()
	uiStopped            bool
	fixtureFailure       bool
	dutBrokenUntilReboot error
	// Per-test logging marker
	logMarker       *logsaver.Marker
	cleanupPolicies func(ctx context.Context) error
	simFeatures     []labapi.SIMProfileInfo_Feature
}

func newCellularFixture() *cellularFixture {
	val := cellularFixture{}
	// set defaults
	val.restartOnFailure = []string{hermes.JobName, modemmanager.JobName, shill.JobName}
	val.daemonUptimeBeforeTest = 2 * time.Minute
	val.systemUptimeBeforeTest = 2 * time.Minute
	return &val
}
func (f *cellularFixture) setCheckSIM(value bool) *cellularFixture {
	f.checkSIM = value
	return f
}
func (f *cellularFixture) setClearSIMLock(value bool) *cellularFixture {
	f.clearSIMLock = value
	return f
}
func (f *cellularFixture) setHasArc(value bool) *cellularFixture {
	f.hasArc = value
	return f
}
func (f *cellularFixture) setRestartMM(value bool) *cellularFixture {
	f.restartMM = value
	return f
}
func (f *cellularFixture) setUseFakeDMS(value bool) *cellularFixture {
	f.useFakeDMS = value
	return f
}
func (f *cellularFixture) setUseRoaming(value bool) *cellularFixture {
	f.useRoaming = value
	return f
}
func (f *cellularFixture) setUseTestESIM(value bool) *cellularFixture {
	f.useTestESIM = value
	return f
}
func (f *cellularFixture) setStopUI(value bool) *cellularFixture {
	f.stopUI = value
	return f
}
func (f *cellularFixture) setResetShillProfileOnPostTest(value bool) *cellularFixture {
	f.resetShillProfileOnPostTest = value
	return f
}
func (f *cellularFixture) setRestartOnFailure(value []string) *cellularFixture {
	f.restartOnFailure = value
	return f
}
func (f *cellularFixture) setDaemonUptimeBeforeTest(value time.Duration) *cellularFixture {
	f.daemonUptimeBeforeTest = value
	return f
}
func (f *cellularFixture) setDisableCellularInShill(value bool) *cellularFixture {
	f.disableCellularInShill = value
	return f
}
func (f *cellularFixture) setHasChrome(value bool) *cellularFixture {
	f.hasChrome = value
	return f
}

// FixtData holds information made available to tests that specify this fixture.
type FixtData struct {
	Helper *Helper
	fdms   *fakedms.FakeDMS
	ARC    *arc.ARC
	Chrome *chrome.Chrome
}

// FakeDMS implements the HasFakeDMS interface.
func (fd FixtData) FakeDMS() *fakedms.FakeDMS {
	if fd.fdms == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return fd.fdms
}

func (f *cellularFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// We use the variable |FixtureFailure| to indicate that there was a failure in the fixture.
	// We check this variable later in the cleanUp/TearDown to recover the device to a good state.
	f.fixtureFailure = true
	// Add a cleanUp function to fix the fixture if it fails, as TearDown is only executed if the
	// SetUp is successful.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, tearDownTime)
	defer cancel()
	// Ensure the test restores the modemfwd state.
	defer f.fixtureCleanUp(cleanupCtx, s)

	var err error
	f.crashFilesTracker, err = crash.GetCrashes(crash.DefaultDirs()...)
	if err != nil {
		s.Fatal("Failed to get crashes: ", err)
	}
	// Ensure cellular is enabled because on models with modem power off feature implemented,
	// the modem is powered off when cellular is disabled; need to power on the modem by
	// enabling cellular before a test.
	if manager, err := shill.NewManager(ctx); err == nil {
		manager.EnableTechnology(ctx, shill.TechnologyCellular)
	}

	// Give some time for cellular daemons to perform any modem operations. Stopping them via upstart might leave the modem in a bad state.
	if err := EnsureUptime(ctx, f.systemUptimeBeforeTest); err != nil {
		s.Fatal("Failed to wait for system uptime: ", err)
	}
	if err := SetShillVerboseLogging(ctx); err != nil {
		s.Fatal("Failed to set shill's logging config to verbose: ", err)
	}
	if err := modemmanager.SetModemmanagerLogLevel(ctx, "DEBUG"); err != nil {
		s.Fatal("Failed to set Modemmanager log level to DEBUG: ", err)
	}
	// Before stopping modemfwd, check and wait for modemfwd to idle.
	if err := f.waitForModemFwdToIdle(ctx); err != nil {
		s.Fatal("Could not confirm if ModemFwd is idle: ", err)
	}
	// Check if the modem is exported by ModemManager before initializing Starfish/NewHelper().
	if _, err := waitForModemToBeExported(ctx); err != nil {
		if isL850(ctx, s) {
			err = TagKnownBug(ctx, err, "b/407604639")
		}
		s.Fatal("Could not confirm if modem was exported: ", err)
	}

	// Fetch DUT info.
	var dutInfo *cellular.DUTInfo
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		dutInfo = cellular.NewDUTInfoFromConfig(dutConfig)
		s.Log("Loaded DUT info from lab config")
	} else if dutInfo, err = cellular.NewDUTInfoFromStringArgs(ctx, s.Var, "autotest_host_info_labels"); err != nil {
		// Not fatal unless labels are required.
		s.Log("Failed to get SIM info labels: ", err)
	} else {
		s.Log("Loaded DUT info from autotest_host_info_labels")
	}
	if dutInfo != nil {
		dutInfo.LogInfo(ctx)
	}

	// Initialize Starfish.
	sfish, sfIndex, sfCarrier, err := starfish.NewStarfish(ctx, dutInfo, f.simFeatures)
	if err != nil {
		s.Fatal("Failed to setup starfish module on supported setup: ", err)
	}
	f.sf = sfish

	// Get modem after initializing starfish since SIM eject may change modem path.
	modem, err := waitForModemToBeExported(ctx)
	if err != nil {
		s.Fatal("Could not confirm if modem was exported: ", err)
	}

	// check if OS version is divisible by 10. If it is, start modem logging if available
	modemLoggingStarted, err := triggerModemLoggingConditionally(ctx)
	if err != nil {
		if f.useFakeDMS {
			s.Log("Failed to trigger modem logging: ", err)
		} else {
			s.Fatal("Failed to trigger modem logging: ", err)
		}
	}
	f.modemLoggingStarted = modemLoggingStarted
	defer func(s *testing.FixtState) {
		if f.modemLoggingStarted && s.HasError() {
			if err := stopModemLogging(ctx); err != nil {
				s.Log("Could not stop modem logging: ", err)
			} else {
				f.modemLoggingStarted = false
			}
		}
	}(s)

	checkCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// b/289540816: Ensure the APN in the modem doesn't contain a leftover value from a manual test.
	if err := CheckIfL850VerizonAndFixDefaultAPN(checkCtx); err != nil {
		s.Log("Failed to fix default APN when the device has a L850GL modem with a verizon SIM card: ", err)
	}

	// Ensure that the primary SIM slot has a valid SIM.
	if !(f.useTestESIM || f.sf != nil) {
		if modem, err = modem.EnsureValidSIM(ctx, false); err != nil {
			if isL850(ctx, s) && dutInfoSuggestsVerizon(dutInfo) {
				// b/525406848: L850 + Verizon is a known unstable combination.
				// Since EnsureValidSIM failed, we cannot query the modem for the
				// carrier, so we rely on static DUT info.
				testing.ContextLog(ctx, "L850 + Verizon detected from DUT info, marking as b/525406848")
				err = TagKnownBug(ctx, err, "b/525406848")
			}
			s.Fatal("Failed to ensure valid SIM: ", err)
		}
	}

	if f.sf != nil {
		if _, err := stopJob(ctx, modemfwd.JobName); err != nil {
			s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
		}
		// On starfish setups, PSIM being the active sim slot is required
		if _, err = modem.EnsureValidSIM(ctx, true); err != nil {
			s.Fatal("Failed to switch to PSIM on a starfish setup: ", err)
		}
		// Do a SIM insert on starfish
		if err := f.sf.SimInsert(ctx, sfIndex); err != nil {
			s.Fatalf("Failed to configure starfish at slot index - %d for %s: %s", sfIndex, sfCarrier, err)
		}
		testing.ContextLog(ctx, "starfish configured for ", sfCarrier, " at slot index: ", sfIndex)
		// if a starfish v0 (regular starfish), a modem restart is required
		if f.sf.GetModuleVersion(ctx) == starfish.ModuleVersion0 {
			if _, err = ResetModemWithHelper(ctx); err != nil {
				s.Fatal("Failed to restart modem: ", err)
			}
		}
		// On starfish setups, modemfwd needs to run to install the correct FW after the SIM is selected.
		if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
			s.Fatalf("Failed to start %q: %s", modemfwd.JobName, err)
		}
		// modemfwd might get the indication that the modem is ready with the correct firmware above
		// polling the dbus modem object, just in case.
		if _, err = waitForModemToBeExported(ctx); err != nil {
			s.Fatal("Could not find modem after starting modemfwd: ", err)
		}
	}

	// Ensure that a Helper instance can be instantiated to use where needed.
	helper, err := NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create Helper: ", err)
	}
	f.helper = helper

	if dutInfo != nil {
		helper.SetDUTInfo(dutInfo)
	}

	if f.clearSIMLock {
		// Clear the SIM lock in SetUp and TearDown to attempt to recover any
		// DUT left in a SIM locked state. Since not all tests run on a SIM that
		// supports SIM lock, only do this in fixtures that require SIM locking.
		if dutInfo == nil {
			s.Fatal("Cannot clear SIM lock, DUT configuration not available")
		}
		if err := helper.ClearSIMLockFromHostInfo(ctx); err != nil {
			s.Fatal("Failed to clear SIM lock: ", err)
		}
	}

	if !f.useTestESIM {
		service, err := helper.EnsureDefaultService(ctx)
		if err != nil {
			s.Fatal("Failed to ensure default service: ", err)
		}
		// Note: The Service path may change if Cellular is disabled or the Modem
		// is reset, so log the name instead.
		if name, err := service.GetName(ctx); err != nil {
			testing.ContextLog(ctx, "Unable to get name for default Cellular Service: ", err)
		} else {
			testing.ContextLog(ctx, "Cellular has default Service: ", name)
		}

		// Auto-connect should be enabled by default (b/352817928).
		// Setting that property requires a connectable cellular profile, but since the cellular profile
		// for test eSIM hasn't been installed yet, it can only be done when we're not using the test eSIM.
		cleanup, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, true)
		if err != nil {
			s.Fatal("Failed to initialize auto-connect to true: ", err)
		}
		// Cleanup auto-connect property.
		f.cleanupPolicies = cleanup
	}
	if f.stopUI {
		if f.uiStopped, err = stopJob(ctx, uiJobName); err != nil {
			s.Fatalf("Failed to stop job: %q, %s", uiJobName, err)
		}
	}

	var fdms *fakedms.FakeDMS
	if f.useFakeDMS {
		var ok bool
		fdms, ok = s.ParentValue().(*fakedms.FakeDMS)
		if !ok {
			s.Fatal("Parent is not a fakeDMSEnrolled fixture")
		}
	}

	var a *arc.ARC
	if f.hasArc {
		a = s.ParentValue().(*arc.PreData).ARC
	}

	var cr *chrome.Chrome
	if f.hasChrome {
		cr, err = chrome.New(ctx)
		if err != nil {
			s.Fatal("Failed to create a new instance of Chrome: ", err)
		}
	}
	f.cr = cr

	if f.modemfwdStopped, err = stopJob(ctx, modemfwd.JobName); err != nil {
		s.Fatalf("Failed to stop job: %q, %s", modemfwd.JobName, err)
	}

	if upstart.JobExists(ctx, hermes.JobName) {
		err := hermes.WaitForChromeESIMCache(ctx, 2*time.Second)
		if err != nil {
			s.Logf("Chrome may inhibit the modem at next login: %s", err)
		} else {
			if err := hermes.WaitForHermesIdle(ctx, 30*time.Second); err != nil {
				s.Logf("Could not confirm if Hermes is idle: %s", err)
			}
		}
	}

	if f.disableCellularInShill {
		// Disable cellular in shill to prevent re-enabling cellular after Modem disable called.
		// In Shill::Cellular::Stop(), we don't allow Stop(disable Cellular technology) when
		// Cellular is in Starting state. This scenario is not hard to reproduce when modemmanager
		// is restarted on a previous fixture or test. Since DisableTechnology will return an error
		// if Cellular is being started, retry a few times until Cellular has completed the Starting
		// operation.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := helper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyCellular); err != nil {
				testing.ContextLog(ctx, "Failed to disable Cellular technology: ", err)
				return err
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  8 * time.Second,
			Interval: 500 * time.Millisecond,
		}); err != nil {
			testing.ContextLog(ctx, "Unable to disable Cellular: ", err)
		}
	}
	if f.restartMM {
		if err := upstart.RestartJob(ctx, modemmanager.JobName, GetMMUpstartArgsForVerboseLogging()...); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", modemmanager.JobName, err)
		}
		// Wait for MM to export the modem after restart
		if _, err = modemmanager.NewModem(ctx); err != nil {
			return errors.Wrap(err, "failed to get modem after restart")
		}
	}
	if f.useRoaming {
		cleanup, err := SetRoamingPolicy(ctx, true, false)
		if err != nil {
			s.Fatal("Failed to set roaming property: ", err)
		}
		// Cleanup roaming policies and auto-connect property.
		f.cleanupPolicies = cleanup
	}

	if f.checkSIM {
		if _, err := helper.Connect(ctx); err != nil {
			s.Fatal("Failed to connect for checkSIM: ", err)
		}
		if _, err := helper.Disconnect(ctx); err != nil {
			s.Fatal("Failed to disconnect for checkSIM: ", err)
		}
	}
	f.fixtureFailure = false
	return &FixtData{helper, fdms, a, cr}
}

func (f *cellularFixture) Reset(ctx context.Context) error {
	if f.dutBrokenUntilReboot != nil {
		// When an error is returned on Reset, a full TearDown and SetUp is executed. This is
		// desired when rebooting the DUT using the remote fixture, since the DUT state after a
		// reboot is not the same as after SetUp.
		err := f.dutBrokenUntilReboot
		f.dutBrokenUntilReboot = nil
		testing.ContextLog(ctx, "Reset() detected a failure. Trigger a fixture TearDown and SetUp")
		return err
	}
	return nil
}

func (f *cellularFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Start tracking per-test logs.
	if f.logMarker != nil {
		testing.ContextLog(ctx, "Logging marker was leaked")
	}

	logMarker, err := logsaver.NewMarker("/var/log/net.log")
	if err != nil {
		testing.ContextLog(ctx, "Unable to create logging marker, per-log tests will be unavailable")
	} else {
		f.logMarker = logMarker
	}

	// If ModemManager isn't exporting a modem, it's possible that the modem has stopped responding due to
	// b/247984538, attempt to force a restart of the modem on devices that support modemfwd-helpers.
	modem, err := waitForModemToBeExported(ctx)
	if err != nil {
		s.Fatal("No modem exported by ModemManager: ", err)
	}

	if f.restartMM {
		if err := modem.Enable(ctx); err != nil {
			s.Fatal("Modem enable failed with: ", err)
		}
	}

	// Run modem status before starting the test
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Failed to get out dir")
		return
	}

	outFile, err := os.Create(filepath.Join(outDir, "modem-status.txt"))
	if err != nil || outFile == nil {
		return
	}

	cmd := testexec.CommandContext(ctx, "modem", "status")
	cmd.Stdout = outFile
	cmd.Stderr = outFile

	if err := cmd.Run(); err != nil {
		testing.ContextLog(ctx, "Failed to run modem status: ", err)
	}
	if err := outFile.Close(); err != nil {
		testing.ContextLog(ctx, "Failed to close modem-status.txt: ", err)
	}

	// Prevent check_ethernet.hook from interrupting test if network is temporarily
	// disabled. Automatically unlocked after 30 minutes, so unlock and lock it
	// between each test.
	if unlock, err := network.LockCheckNetworkHook(ctx); err != nil {
		f.netUnlock = nil
		// Technically possible to time out acquiring lock, log the error but
		// do not fatal since it's not a test prereq.
		testing.ContextLog(ctx, "Failed to lock the check network hook: ", err)
	} else {
		f.netUnlock = unlock
	}

	// Ensure that Cellular is Enabled and has a default Service before each test.
	if !f.useTestESIM && !f.disableCellularInShill {
		f.helper.EnsureDefaultService(ctx)
	}
}

func getUpstartArgsForVerboseLogging(job string) []upstart.Arg {
	switch job {
	case shill.JobName:
		return GetShillUpstartArgsForVerboseLogging()
	case modemmanager.JobName:
		return GetMMUpstartArgsForVerboseLogging()
	}
	return []upstart.Arg{}
}

func (f *cellularFixture) restartJobsAndWaitOnFailure(ctx context.Context) {
	var restartOnFailure []string
	for _, p := range f.restartOnFailure {
		// If shill was reset on PostTest, don't do it again.
		if f.resetShillProfileOnPostTest && p == shill.JobName {
			continue
		}
		restartOnFailure = append(restartOnFailure, p)
	}
	f.restartJobs(ctx, restartOnFailure)
}

func (f *cellularFixture) restartJobs(ctx context.Context, restartOnFailure []string) {
	// stop and start jobs instead of upstart.Restart to emulate a reboot.
	for _, p := range restartOnFailure {
		testing.ContextLogf(ctx, "Fixture detected a test failure, restarting %s", p)
		if _, err := stopJob(ctx, p); err != nil {
			testing.ContextLogf(ctx, "Failed to stop job: %q, %s", p, err)
		}
	}
	for _, p := range restartOnFailure {
		if err := upstart.StartJob(ctx, p, getUpstartArgsForVerboseLogging(p)...); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", p, err)
		}
		testing.ContextLogf(ctx, "Restarted job: %q", p)
	}
	if slices.Contains(restartOnFailure, modemmanager.JobName) {
		if _, err := modemmanager.NewModem(ctx); err != nil {
			testing.ContextLog(ctx, "Could not find MM dbus object after restarting ModemManager: ", err)
		}
	}
	if len(restartOnFailure) > 0 {
		deadline, ok := ctx.Deadline()
		if ok {
			// Don't wait exactly until deadline. Leave some time for other things to complete.
			maxSleepTime := time.Until(deadline) - 4*time.Second
			if maxSleepTime > f.daemonUptimeBeforeTest {
				maxSleepTime = f.daemonUptimeBeforeTest
			}
			if maxSleepTime > 0 {
				testing.ContextLog(ctx, "Wait for daemon uptime:", maxSleepTime)
				// GoBigSleepLint - Delay starting the next test to avoid any transients caused by restarting MM and shill.
				testing.Sleep(ctx, maxSleepTime)
			}
		}

	}
}

func (f *cellularFixture) getCrashedDaemonName(ctx context.Context) (string, []string, error) {
	crashFiles, err := crash.GetCrashes(crash.DefaultDirs()...)
	if err != nil {
		return "", crashFiles, err
	}

	for _, file := range crashFiles {
		if slices.Contains(f.crashFilesTracker, file) {
			// Skip if the crash was from a previous test
			continue
		}
		filename := filepath.Base(file)
		testing.ContextLog(ctx, "crash found: ", filename)
		for _, prefix := range []string{"ModemManager.", "shill.", "qmi_proxy.", "mbim_proxy.", "hermes.", "modemfwd.", "fastboot."} {
			if strings.HasPrefix(filename, prefix) {
				return filename, crashFiles, nil
			}
		}
	}
	return "", crashFiles, nil
}

func (f *cellularFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	crashToBlame, newCrashFiles, getCrashErr := f.getCrashedDaemonName(ctx)
	f.crashFilesTracker = newCrashFiles

	if s.HasError() {
		f.restartJobsAndWaitOnFailure(ctx)
	}

	if f.resetShillProfileOnPostTest {
		testing.ContextLog(ctx, "Resetting shill on PostTest")
		if errs := f.helper.ResetShill(ctx); errs != nil {
			testing.ContextLogf(ctx, "Failed to restart job: shill, %s", errs)
		}
	}
	modem, waitForModemErr := modemmanager.NewModem(ctx)
	if waitForModemErr != nil {
		testing.ContextLog(ctx, "Failed to create modem object: ", waitForModemErr)
		// b/407604639 :  When the L850 modem doesn't come back after reset, we need to ec reset the
		// device using the remote fixture. Since a reboot doesn't necessarily put the DUT into its
		// previous clean state, run the TearDown/SetUp by returning an error on Reset(). We use
		// dutBrokenUntilReboot to trigger a fixture TearDown/SetUp on Reset().
		if isL850(ctx, s) {
			f.dutBrokenUntilReboot = waitForModemErr
			testing.ContextLog(ctx, "PostTest: L850 Modem not detected. Requesting a dut ec reset from remote fixture")
			if err := os.WriteFile(EcResetOnResetRequestedFlagPath, []byte("1"), 0666); err != nil {
				testing.ContextLogf(ctx, "Could not write to %s: %s", EcResetOnResetRequestedFlagPath, err)
			}
		}
	} else {
		if err := RebootL850VerizonIfModemCanNoLongerConnect(ctx, &modem); err != nil {
			testing.ContextLog(ctx, "Failed to restart modem: ", err)
		}
	}

	if f.netUnlock != nil {
		f.netUnlock()
	}

	if f.clearSIMLock {
		// Helper labels will already be stored.
		if err := f.helper.ClearSIMLockFromHostInfo(ctx); err != nil {
			s.Fatal("Failed to clear SIM lock: ", err)
		}
	}

	if f.logMarker != nil {
		// Save per-test logs.
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			testing.ContextLog(ctx, "Failed to get ContextOutDir")
		} else if err := f.logMarker.Save(filepath.Join(outDir, "net.log")); err != nil {
			testing.ContextLog(ctx, "Failed to store per-test net.log")
		}
		f.logMarker = nil
	}
	if getCrashErr != nil {
		s.Fatal("Failed to get crashes: ", getCrashErr)
	}
	if crashToBlame != "" {
		s.Fatal("Failed due to previous daemon crash: ", crashToBlame)
	}

}

func (f *cellularFixture) fixtureCleanUp(ctx context.Context, s *testing.FixtState) {
	if f.fixtureFailure {
		testing.ContextLog(ctx, "Fixture failure. Calling TearDown from fixtureCleanUp")
		// Call TearDown from here so we can keep all the CleanUp and TearDown
		// logic in one place.
		f.TearDown(ctx, s)
	}
}

func (f *cellularFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	testing.ContextLog(ctx, "Executing Fixture::Teardown")
	teardownFailure := false
	if f.cleanupPolicies != nil {
		f.cleanupPolicies(ctx)
	}

	if f.modemLoggingStarted {
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := stopModemLogging(waitCtx); err != nil {
			testing.ContextLog(ctx, "Could not stop modem logging: ", err)
			teardownFailure = true
		} else {
			f.modemLoggingStarted = false
		}
	}
	if f.disableCellularInShill && f.helper != nil {
		// In Shill::Cellular::Start(), we don't allow Start(enable Cellular technology) when
		// Cellular is in Stopping state. This scenario is not hard to reproduce when modemmanager
		// is restarted by itself on the fixture teardown(for example, on fixture
		// cellularModemManager). Since EnableTechnology will return an error if Cellular is being
		// stopped, retry a few times until Cellular has completed the Stopping operation.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := f.helper.Manager.EnableTechnology(waitCtx, shill.TechnologyCellular); err != nil {
				testing.ContextLog(ctx, "Failed to enable Cellular technology: ", err)
				return err
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  5 * time.Second,
			Interval: 500 * time.Millisecond,
		}); err != nil {
			testing.ContextLog(ctx, "Unable to enable Cellular: ", err)
			teardownFailure = true
		}
	}
	if f.modemfwdStopped {
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if err := modemfwd.StartAndWaitForQuiescence(waitCtx); err != nil {
			testing.ContextLogf(ctx, "Failed to start %q: %s", modemfwd.JobName, err)
			teardownFailure = true
		}
		testing.ContextLogf(ctx, "Started %q", modemfwd.JobName)
	}
	if f.uiStopped {
		waitCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
		defer cancel()
		if err := upstart.EnsureJobRunning(waitCtx, uiJobName); err != nil {
			testing.ContextLogf(ctx, "Failed to start %q: %s", uiJobName, err)
			teardownFailure = true
		}
		testing.ContextLogf(ctx, "Started %q", uiJobName)
	}
	if f.hasChrome && f.cr != nil {
		f.cr.Close(ctx)
	}
	if f.sf != nil {
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if err := f.sf.Teardown(waitCtx); err != nil {
			testing.ContextLogf(ctx, "Failed to teardown starfish: %s", err)
			teardownFailure = true
		}
	}

	if f.fixtureFailure {
		testing.ContextLog(ctx, "Fixture failure detected. Restarting all daemons")
		f.restartJobs(ctx, []string{hermes.JobName, modemmanager.JobName, shill.JobName})
	}

	{
		waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := modemmanager.SetModemmanagerLogLevel(waitCtx, "INFO"); err != nil {
			testing.ContextLog(ctx, "Failed to set Modemmanager log level to INFO: ", err)
			teardownFailure = true
		}
	}
	{
		waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := SetShillDefaultLogging(waitCtx); err != nil {
			testing.ContextLog(ctx, "Failed to reset shill's logging config: ", err)
			teardownFailure = true
		}
	}
	if teardownFailure {
		testing.ContextLog(ctx, "Teardown failed")
		if err := os.WriteFile(RebootOnTearDownRequestedFlagPath, []byte("1"), 0666); err != nil {
			testing.ContextLogf(ctx, "Could not write to %s: %s", RebootOnTearDownRequestedFlagPath, err)
		}

	}
}

func stopJob(ctx context.Context, job string) (bool, error) {
	if !upstart.JobExists(ctx, job) {
		testing.ContextLogf(ctx, "StopJob has no action. Job %q does not exist", job)
		return false, nil
	}
	_, _, pid, err := upstart.JobStatus(ctx, job)
	if err != nil {
		return false, errors.Wrapf(err, "failed to run upstart.JobStatus for %q", job)
	}
	if pid == 0 {
		testing.ContextLogf(ctx, "StopJob has no action. Job %q has pid == 0", job)
		return false, nil
	}
	err = upstart.StopJob(ctx, job)
	if err != nil {
		return false, errors.Wrapf(err, "failed to stop %q", job)
	}
	testing.ContextLogf(ctx, "Stopped job `%q`", job)
	return true, nil

}

func (f *cellularFixture) waitForModemFwdToIdle(ctx context.Context) error {
	if err := EnsureDaemonUptime(ctx, modemfwd.JobName, f.daemonUptimeBeforeTest); err != nil {
		return errors.Wrapf(err, "failed to wait for %v uptime", modemfwd.JobName)
	}
	// Before stopping modemfwd, check and wait for flash to complete.
	if err := modemfwd.CheckAndWaitForFlashToComplete(ctx); err != nil {
		// return errors.Wrap(err, "failed to confirm if modem flash is complete")
		testing.ContextLog(ctx, "Failed to confirm if modem flash is complete after 5 minutes")
	}
	return nil
}

func waitForModemToBeExported(ctx context.Context) (*modemmanager.Modem, error) {
	// Wait for modem to be exported by ModemManager.
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		if ModemHelperPathExists() {
			testing.ContextLog(ctx, "No modem exported by ModemManager, attempting to restart the modem")
			modem, err = ResetModemWithHelper(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to restart modem")
			}
			return modem, nil
		}
		return nil, err
	}
	return modem, nil
}

func isL850(ctx context.Context, s interface{}) bool {
	// We cannot use TagKnownBugOnModem to detect the modem type and FW version because
	// TagKnownBugOnModem requires a live modem to query the modem FW version. If MR9 is ever
	// released, we need to change this logic to exclude it.
	modemType, err := GetModemType(ctx)
	if err != nil {
		// Throw fatal to simplify its usage. If a fatal is not desired by the caller, modify
		// this function to return an error.
		s.(*testing.FixtState).Fatalf("Failed to get modem type: %s", err)
	}
	return modemType == cellularconst.ModemTypeL850
}

func dutInfoSuggestsVerizon(dutInfo *cellular.DUTInfo) bool {
	if dutInfo == nil {
		return false
	}

	// Check if the static DUT info shows that the DUT has a Verizon SIM. This is used when the
	// modem is not able to report its carrier, for instance when the modem is unresponsive.
	return strings.Contains(strings.ToLower(dutInfo.CarrierName), "verizon")
}

func triggerModemLoggingConditionally(ctx context.Context) (bool, error) {
	releaseVersion := util.GetChromeOSReleaseVersion()
	// enable modem logging every 10'th version. Modem logging may change modem runtime behavior, so enable it sparsely.
	if !strings.HasSuffix(releaseVersion, "0.0.0") {
		return false, nil
	}
	modemLoggerPaths, err := modemloggerd.GetModemLoggerPaths(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get modem logger paths")
	}
	if len(modemLoggerPaths) == 0 {
		return false, nil
	}
	if err := startModemLogging(ctx); err != nil {
		return false, errors.Wrap(err, "failed to start modem logging")
	}
	return true, nil
}

func startModemLogging(ctx context.Context) error {
	modemLogger, err := modemloggerd.NewModemLogger(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get a logger for the modem")
	}
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.Wrap(err, "failed to get test output dir")
	}
	if err := modemLogger.SetOutputDir(ctx, outDir); err != nil {
		return errors.Wrap(err, "failed to set modem logs output dir")
	}
	if err := modemLogger.SetEnabled(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable modem logging")
	}
	if err := modemLogger.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start modem logging")
	}
	return nil
}

func stopModemLogging(ctx context.Context) error {
	modemLogger, err := modemloggerd.NewModemLogger(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get a logger for the modem")
	}
	if err := modemLogger.Stop(ctx); err != nil {
		return errors.Wrap(err, "failed to stop modem logging")
	}
	if err := modemLogger.SetEnabled(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable modem logging")
	}
	return nil
}
