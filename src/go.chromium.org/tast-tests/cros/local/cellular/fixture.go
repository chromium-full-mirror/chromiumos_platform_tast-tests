// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/starfish"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// The Cellular test fixture ensures that modemfwd is stopped.

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "cellular",
		Desc:            "Cellular tests are safe to run",
		Contacts:        []string{"chromeos-cellular-team@google.com", "stevenjb@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularTestESIM",
		Desc:            "Cellular tests are safe to run with a Test SIM",
		Contacts:        []string{"chromeos-cellular-team@google.com", "stevenjb@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{useTestESIM: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolled",
		Desc:            "Cellular tests are safe to run and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jiajunzhang@google.com"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{useFakeDMS: true},
		Parent:          fixture.FakeDMSEnrolled,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolledAndTestSIM",
		Desc:            "Cellular tests are safe to run that require a Test SIM and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jiajunzhang@google.com"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{useFakeDMS: true, useTestESIM: true},
		Parent:          fixture.FakeDMSEnrolled,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFakeDMSEnrolledAndFunctioningSIM",
		Desc:            "Cellular tests are safe to run that require a functioning SIM and a fake DMS (for managed eSIM profiles) is running",
		Contacts:        []string{"cros-connectivity@google.com", "jiajunz@google.com"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{useFakeDMS: true, checkSIM: true},
		Parent:          fixture.FakeDMSEnrolled,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "cellularModemManager",
		Desc: "ModemManager tests are safe to run without shill running",
		Contacts: []string{
			"andrewlassalle@google.com",
			"chromeos-cellular-team@google.com",
		},
		SetUpTimeout:    5 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{restartMM: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularArcBooted",
		Desc:            "Arc tests on cellular interface",
		Contacts:        []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{hasArc: true},
		Parent:          "arcBooted",
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFunctioningRoamingSim",
		Desc:            "Cellular tests that require a functioning roaming SIM are safe to run",
		Contacts:        []string{"cros-connectivity@google.com", "nikhilcn@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{useRoaming: true, checkSIM: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularWithFunctioningSim",
		Desc:            "Cellular tests that require a functioning SIM are safe to run",
		Contacts:        []string{"cros-connectivity@google.com", "nikhilcn@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{checkSIM: true},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularPower",
		Desc:            "Power tests for cellular connectivity",
		Contacts:        []string{"chromeos-cellular-team@google.com", "rmao@google.com"},
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		Impl:            &cellularFixture{checkSIM: true},
		Parent:          "powerMetricsNoUI",
	})
}

// cellularFixture implements testing.FixtureImpl.
type cellularFixture struct {
	// Fixture control flags
	restartMM   bool
	useFakeDMS  bool
	useRoaming  bool
	useTestESIM bool
	checkSIM    bool
	hasArc      bool
	// Fixture variables
	helper          *Helper
	modemfwdStopped bool
	sf              *starfish.Starfish
	netUnlock       func()
}

// FixtData holds information made available to tests that specify this fixture.
type FixtData struct {
	Helper *Helper
	fdms   *fakedms.FakeDMS
	ARC    *arc.ARC
}

// FakeDMS implements the HasFakeDMS interface.
func (fd FixtData) FakeDMS() *fakedms.FakeDMS {
	if fd.fdms == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return fd.fdms
}

const uptimeBeforeTest = 2 * time.Minute

func (f *cellularFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Initialize Starfish before any Modem initialization.
	sfish, err := starfish.NewStarfish(ctx)
	if err != nil {
		s.Fatal("Failed to setup starfish module on supported setup: ", err)
	}
	f.sf = sfish

	// Check if the modem is exported by ModemManager before calling NewHelper().
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		testing.ContextLog(ctx, "No modem exported by ModemManager, attempting to restart the modem")
		if !ModemHelperPathExists() {
			s.Fatal("Failed to get modem and no ModemHelper: ", err)
		}
		modem, err = RestartModemWithHelper(ctx)
		if err != nil {
			s.Fatal("Failed to restart modem: ", err)
		}
	}

	// Ensure that the primary SIM slot has a valid SIM.
	if !(f.useTestESIM || f.restartMM) {
		if err := modem.EnsureValidSIM(ctx); err != nil {
			s.Fatal("Failed to ensure valid SIM: ", err)
		}
	}

	// Ensure that a Helper instance can be instantiated to use where needed.
	helper, err := NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create Helper: ", err)
	}
	f.helper = helper

	if sfish != nil {
		// ResetModem needed to detect SIM.
		if _, err := helper.ResetModem(ctx); err != nil {
			s.Log("Failed to reset modem for Starfish: ", err)
		}
	}

	if !f.useTestESIM {
		if err := helper.EnsureDefaultService(ctx); err != nil {
			s.Fatal("Failed to ensure default service: ", err)
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

	// Give some time for cellular daemons to perform any modem operations. Stopping them via upstart might leave the modem in a bad state.
	if err := EnsureUptime(ctx, uptimeBeforeTest); err != nil {
		s.Fatal("Failed to wait for system uptime: ", err)
	}
	if err := SetShillVerboseLogging(ctx); err != nil {
		s.Fatal("Failed to set shill's logging config to verbose: ", err)
	}
	if err := modemmanager.SetModemmanagerLogLevel(ctx, "DEBUG"); err != nil {
		s.Fatal("Failed to set Modemmanager log level to DEBUG: ", err)
	}
	// Before stopping modemfwd, check and wait for modemfwd to idle.
	if err := waitForModemFwdToIdle(ctx); err != nil {
		s.Fatal("Could not confirm if ModemFwd is idle: ", err)
	}
	if err := waitForModemToBeExported(ctx); err != nil {
		s.Fatal("Could not confirm if modem was exported: ", err)
	}
	if f.sf == nil {
		var err error
		if f.modemfwdStopped, err = stopJob(ctx, modemfwd.JobName); err != nil {
			s.Fatalf("Failed to stop job: %q, %s", modemfwd.JobName, err)
		}
		if f.modemfwdStopped {
			s.Logf("Stopped %q", modemfwd.JobName)
		} else {
			s.Logf("%q not running", modemfwd.JobName)
		}
	}
	if upstart.JobExists(ctx, hermes.JobName) {
		// Hermes is usually idle 2 minutes after boot, so go on with the test even if we cannot be sure.
		if err := hermes.WaitForHermesIdle(ctx, 30*time.Second); err != nil {
			s.Logf("Could not confirm if Hermes is idle: %s", err)
		}
	}

	if f.restartMM {
		// Disable cellular in shill to prevents re-enabling cellular after Modem
		// disable called.
		if _, err := helper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyCellular); err != nil {
			s.Fatal("Unable to disable Cellular: ", err)
		}
		if err := upstart.RestartJob(ctx, modemmanager.JobName); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", modemmanager.JobName, err)
		}
		// Wait for MM to export the modem after restart
		if _, err = modemmanager.NewModem(ctx); err != nil {
			return errors.Wrap(err, "failed to get modem after restart")
		}
	}
	if f.useRoaming {
		err := SetRoamingPolicy(ctx, true, false)
		if err != nil {
			s.Fatal("Failed to set roaming property: ", err)
		}
	}

	if f.checkSIM {
		if _, err := helper.Connect(ctx); err != nil {
			s.Fatal("Failed to connect for checkSIM: ", err)
		}
		if _, err := helper.Disconnect(ctx); err != nil {
			s.Fatal("Failed to disconnect for checkSIM: ", err)
		}
	}
	return &FixtData{helper, fdms, a}
}

func (f *cellularFixture) Reset(ctx context.Context) error { return nil }

func (f *cellularFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// If ModemManager isn't exporting a modem, it's possible that the modem has stopped responding due to
	// b/247984538, attempt to force a restart of the modem on devices that support modemfwd-helpers.
	modem, err := modemmanager.NewModem(ctx)
	if err != nil && ModemHelperPathExists() {
		testing.ContextLog(ctx, "No modem exported by ModemManager, attempting to restart the modem")
		modem, err = RestartModemWithHelper(ctx)
		if err != nil {
			if s.TestName() != "cellular.IsModemUp" {
				s.Fatal("Failed to restart modem (precondition): ", err)
			}
			s.Fatal("Failed to restart modem: ", err)
		}
	}

	if f.restartMM {
		err := modem.EnsureValidSIM(ctx)
		if err != nil {
			s.Fatal("Could not find MM dbus object with a valid sim (precondition): ", err)
		}
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
	if !f.useTestESIM {
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

func (f *cellularFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if s.HasError() {
		testing.ContextLog(ctx, "Fixture detected a test failure, restarting MM, Shill and Hermes")
		processes := []string{shill.JobName, modemmanager.JobName, hermes.JobName}
		// stop and start jobs instead of upstart.Restart to emulate a reboot.
		for _, p := range processes {
			if _, err := stopJob(ctx, p); err != nil {
				testing.ContextLogf(ctx, "Failed to stop job: %q, %s", p, err)
			}
		}
		for _, p := range processes {
			if err := upstart.StartJob(ctx, p, getUpstartArgsForVerboseLogging(p)...); err != nil {
				testing.ContextLogf(ctx, "Failed to restart job: %q, %s", p, err)
			}
		}
		if _, err := modemmanager.NewModem(ctx); err != nil {
			testing.ContextLog(ctx, "Could not find MM dbus object after restarting ModemManager: ", err)
		}
		// GoBigSleepLint - Delay starting the next test to avoid any transients caused by restarting MM and shill.
		testing.Sleep(ctx, uptimeBeforeTest)
	}

	if f.netUnlock != nil {
		f.netUnlock()
	}
}

func (f *cellularFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.restartMM {
		if err := f.helper.Manager.EnableTechnology(ctx, shill.TechnologyCellular); err != nil {
			s.Fatal("Unable to enable Cellular: ", err)
		}
	}
	if f.modemfwdStopped {
		if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
			s.Fatalf("Failed to start %q: %s", modemfwd.JobName, err)
		}
		s.Logf("Started %q", modemfwd.JobName)
	}
	if f.sf != nil {
		if err := f.sf.Teardown(ctx); err != nil {
			s.Fatalf("Failed to teardown starfish: %s", err)
		}
	}
	if err := modemmanager.SetModemmanagerLogLevel(ctx, "INFO"); err != nil {
		s.Fatal("Failed to set Modemmanager log level to INFO: ", err)
	}
	if err := SetShillDefaultLogging(ctx); err != nil {
		s.Fatal("Failed to reset shill's logging config: ", err)
	}
}

func stopJob(ctx context.Context, job string) (bool, error) {
	if !upstart.JobExists(ctx, job) {
		return false, nil
	}
	_, _, pid, err := upstart.JobStatus(ctx, job)
	if err != nil {
		return false, errors.Wrapf(err, "failed to run upstart.JobStatus for %q", job)
	}
	if pid == 0 {
		return false, nil
	}
	err = upstart.StopJob(ctx, job)
	if err != nil {
		return false, errors.Wrapf(err, "failed to stop %q", job)
	}
	return true, nil

}

func waitForModemFwdToIdle(ctx context.Context) error {
	if err := EnsureDaemonUptime(ctx, modemfwd.JobName, uptimeBeforeTest); err != nil {
		return errors.Wrapf(err, "failed to wait for %v uptime", modemfwd.JobName)
	}
	// Before stopping modemfwd, check and wait for flash to complete.
	if err := modemfwd.CheckAndWaitForFlashToComplete(ctx); err != nil {
		// return errors.Wrap(err, "failed to confirm if modem flash is complete")
		testing.ContextLog(ctx, "Failed to confirm if modem flash is complete after 5 minutes")
	}
	return nil
}

func waitForModemToBeExported(ctx context.Context) error {
	// Wait for modem to be exported by ModemManager.
	if _, err := modemmanager.NewModem(ctx); err != nil && ModemHelperPathExists() {
		testing.ContextLog(ctx, "No modem exported by ModemManager, attempting to restart the modem")
		if _, err := RestartModemWithHelper(ctx); err != nil {
			return errors.Wrap(err, "failed to restart modem")
		}
	}
	return nil
}
