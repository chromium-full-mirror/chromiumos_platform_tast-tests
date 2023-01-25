// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/mmconst"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/hermes"
	"chromiumos/tast/local/modemfwd"
	"chromiumos/tast/local/modemmanager"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/starfish"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
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
		Impl:            &cellularFixture{disableCellularTechnology: true, restartMM: true},
	})
}

// cellularFixture implements testing.FixtureImpl.
type cellularFixture struct {
	// Fixture control flags
	disableCellularTechnology bool
	restartMM                 bool
	useFakeDMS                bool
	// Fixture variables
	helper          *cellular.Helper
	modemfwdStopped bool
	sf              *starfish.Starfish
}

// FixtData holds information made available to tests that specify this fixture.
type FixtData struct {
	fdms *fakedms.FakeDMS
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
	sfish, err := starfish.NewStarfish(ctx)
	if err != nil {
		s.Fatal("Failed to setup starfish module on supported setup: ", err)
	}
	f.sf = sfish
	if sfish != nil {
		helper, err := cellular.NewHelper(ctx)
		if err != nil {
			s.Fatal("Failed to create cellular.Helper: ", err)
		}
		// ResetModem needed to detect SIM.
		if _, err := helper.ResetModem(ctx); err != nil {
			s.Log("Failed to reset modem: ", err)
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

	// Give some time for cellular daemons to perform any modem operations. Stopping them via upstart might leave the modem in a bad state.
	if err := cellular.EnsureUptime(ctx, uptimeBeforeTest); err != nil {
		s.Fatal("Failed to wait for system uptime: ", err)
	}
	if err := cellular.SetShillVerboseLogging(ctx); err != nil {
		s.Fatal("Failed to set shill's logging config to verbose: ", err)
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

	if f.disableCellularTechnology {
		f.helper, err = cellular.NewHelper(ctx)
		if err != nil {
			s.Fatal("Failed to create cellular.Helper: ", err)
		}
		// Disabling cellular in shill, prevents shill from re-enabling cellular
		// after Modem disable called.
		if _, err := f.helper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyCellular); err != nil {
			s.Fatal("Unable to disable Cellular: ", err)
		}
	}
	if f.restartMM {
		if err := upstart.RestartJob(ctx, modemmanager.JobName); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", modemmanager.JobName, err)
		}
	}
	return &FixtData{fdms}
}

func (f *cellularFixture) Reset(ctx context.Context) error { return nil }

func (f *cellularFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// If ModemManager isn't exporting a modem, it's possible that the modem has stopped responding due to
	// b/247984538, attempt to force a restart of the modem on devices that support modemfwd-helpers.
	if _, err := modemmanager.NewModem(ctx); err != nil && cellular.ModemHelperPathExists() {
		testing.ContextLog(ctx, "No modem exported by ModemManager, attempting to restart the modem")
		if err := cellular.RestartModemWithHelper(ctx); err != nil {
			s.Fatal("Failed to restart modem (precondition): ", err)
		}
	}
	if f.disableCellularTechnology && f.restartMM {
		modem, err := modemmanager.NewModemWithSim(ctx)
		if err != nil {
			s.Fatal("Could not find MM dbus object with a valid sim (precondition): ", err)
		}
		if err := modem.Call(ctx, mmconst.ModemEnable, true).Err; err != nil {
			s.Fatal("Modem enable failed with: ", err)
		}

		if err := modemmanager.EnsureEnabled(ctx, modem); err != nil {
			s.Fatal("Modem not enabled: ", err)
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
}

func (f *cellularFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if s.HasError() {
		testing.ContextLog(ctx, "Fixture detected a test failure, restarting MM and Shill")
		// stop and start jobs instead of upstart.Restart to emulate a reboot.
		if _, err := stopJob(ctx, shill.JobName); err != nil {
			testing.ContextLogf(ctx, "Failed to stop job: %q, %s", shill.JobName, err)
		}
		if _, err := stopJob(ctx, modemmanager.JobName); err != nil {
			testing.ContextLogf(ctx, "Failed to stop job: %q, %s", modemmanager.JobName, err)
		}
		if err := upstart.StartJob(ctx, shill.JobName, cellular.GetShillUpstartArgsForVerboseLogging()...); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", shill.JobName, err)
		}
		if err := upstart.StartJob(ctx, modemmanager.JobName, cellular.GetMMUpstartArgsForVerboseLogging()...); err != nil {
			testing.ContextLogf(ctx, "Failed to restart job: %q, %s", modemmanager.JobName, err)
		}
		if _, err := modemmanager.NewModem(ctx); err != nil {
			testing.ContextLog(ctx, "Could not find MM dbus object after restarting ModemManager: ", err)
		}
		// Delay starting the next test to avoid any transients caused by restarting MM and shill.
		testing.Sleep(ctx, uptimeBeforeTest)
	}
}

func (f *cellularFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.disableCellularTechnology {
		if err := f.helper.Manager.EnableTechnology(ctx, shill.TechnologyCellular); err != nil {
			s.Fatal("Unable to enable Cellular: ", err)
		}
	}
	if f.modemfwdStopped {
		err := upstart.EnsureJobRunning(ctx, modemfwd.JobName, upstart.WithArg("DEBUG_MODE", "true"))
		if err != nil {
			s.Fatalf("Failed to start %q: %s", modemfwd.JobName, err)
		}
		s.Logf("Started %q", modemfwd.JobName)
	}
	if f.sf != nil {
		if err := f.sf.Teardown(ctx); err != nil {
			s.Fatalf("Failed to teardown starfish: %s", err)
		}
	}
	if err := cellular.SetShillDefaultLogging(ctx); err != nil {
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
