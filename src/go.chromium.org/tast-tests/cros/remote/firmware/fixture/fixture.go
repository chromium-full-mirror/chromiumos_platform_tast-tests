// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture implements fixtures for firmware tests.
package fixture

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Fixture names for the tests to use.
const (
	NormalMode              = "bootModeNormal"
	DevMode                 = "bootModeDev"
	DevModeGBB              = "bootModeDevGBB"
	DevModeWPEnabledZeroGBB = "boolModeDevWPEnabledZeroGBB"
	USBDevModeNoServices    = "bootModeUSBDevNoServices"
	USBDevModeGBBNoServices = "bootModeUSBDevGBBNoServices"
	USBDevModeGBB           = "bootModeUSBDevGBB"
	DevRecModeNoServices    = "bootModeDevRecNoServices"
	RecModeNoServices       = "bootModeRecNoServices"
	RecModeCopyServices     = "bootModeRecModeCopyServices"
	USBDevModeWithReinstall = "bootModeUSBDevGBBAndReinstall"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            NormalMode,
		Desc:            "Reboot into normal mode before test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeNormal, false, true),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            DevMode,
		Desc:            "Reboot into dev mode before test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeDev, false, true),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            DevModeGBB,
		Desc:            "Reboot into dev mode using GBB flags before test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeDev, true, true),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            DevModeWPEnabledZeroGBB,
		Desc:            "Reboot while ensuring HW and SW write protect are enabled and that GBB is set to zero. Device must already be in developer mode",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newWPEnabledZeroGBBFixture(),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            USBDevModeNoServices,
		Desc:            "Reboot into usb-dev mode before test, ServiceDeps are not supported",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeUSBDev, false, false),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            USBDevModeGBBNoServices,
		Desc:            "Reboot into usb-dev mode using GBB flags before test, ServiceDeps are not supported",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeUSBDev, true, false),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            DevRecModeNoServices,
		Desc:            "Reboot into dev recovery mode before test, ServiceDeps are not supported",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "digehlot@google.com"},
		Impl:            newFixture(common.BootModeRecovery, true, false),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            RecModeNoServices,
		Desc:            "Reboot into recovery mode before test, ServiceDeps are not supported",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeRecovery, false, false),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            RecModeCopyServices,
		Desc:            "Reboot into recovery mode before test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeRecovery, false, true),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            USBDevModeGBB,
		Desc:            "Reboot into usb-dev mode using GBB flags before test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            newFixture(common.BootModeUSBDev, true, true),
		Vars:            []string{"servo", "dutHostname", "powerunitHostname", "powerunitOutlet", "hydraHostname", "firmware.no_ec_sync", "firmware.skipFlashUSB", "noSSH"},
		SetUpTimeout:    60 * time.Minute, // Setting up USB key is slow
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  15 * time.Minute,
		PostTestTimeout: 10 * time.Minute,
		TearDownTimeout: 10 * time.Minute,
		Data:            []string{firmware.ConfigFile},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            USBDevModeWithReinstall,
		Desc:            "Reboot into usb dev mode before test, and reinstall ChromeOS after last test",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            &reinstall{},
		Parent:          USBDevModeGBB,
		TearDownTimeout: 30 * time.Minute,
	})
}

// Value contains fields that are useful for tests.
type Value struct {
	BootMode      common.BootMode
	GBBFlags      *pb.GBBFlagsState
	Helper        *firmware.Helper
	ForcesDevMode bool
	ForceZeroGBB  bool
	ForceWPEnable bool
}

// impl contains fields that are useful for Fixture methods.
type impl struct {
	value         *Value
	origBootMode  *common.BootMode
	origGBBFlags  *pb.GBBFlagsState
	copyTastFiles bool
	disallowSSH   bool
}

// newFixture creates an instance of firmware Fixture.
func newFixture(mode common.BootMode, forceDev, copyTastFiles bool) testing.FixtureImpl {
	return &impl{
		value: &Value{
			BootMode:      mode,
			ForcesDevMode: forceDev,
			ForceZeroGBB:  false,
			ForceWPEnable: false,
		},
		copyTastFiles: copyTastFiles,
	}
}

// newWPEnabledZeroGBBFixture creates an instance of firmware Fixture with WP enabled and zero GBB.
func newWPEnabledZeroGBBFixture() testing.FixtureImpl {
	return &impl{
		value: &Value{
			BootMode:      common.BootModeDev,
			ForcesDevMode: false,
			ForceZeroGBB:  true,
			ForceWPEnable: true,
		},
		copyTastFiles: true,
	}
}

func (i *impl) varToBool(s *testing.FixtState, varName string) (bool, error) {
	value := false
	valueStr, ok := s.Var(varName)
	if ok {
		var err error
		value, err = strconv.ParseBool(valueStr)
		if err != nil {
			return false, errors.Errorf("invalid value for var %v: got %q, want true/false", varName, valueStr)
		}
	}
	return value, nil
}

// SetUp is called by the framework to set up the environment with possibly heavy-weight
// operations.
func (i *impl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var err error
	i.disallowSSH, err = i.varToBool(s, "noSSH")
	if err != nil {
		s.Fatal("noSSH: ", err)
	}

	s.Log("Creating a new firmware Helper instance for fixture: ", i.String())
	i.initHelper(ctx, s)

	if i.disallowSSH {
		s.Log("Skipping GBB and reboot because noSSH var was set")
		return i.value
	}

	if i.value.ForceZeroGBB {
		i.value.GBBFlags = &pb.GBBFlagsState{Clear: common.AllGBBFlags()}
		return i.value
	}

	flags := pb.GBBFlagsState{Clear: common.NonpreciousGBBFlags(), Set: common.FAFTGBBFlags()}
	if i.value.ForcesDevMode {
		if i.value.BootMode == common.BootModeUSBDev {
			common.GBBAddFlag(&flags, pb.GBBFlag_FORCE_DEV_SWITCH_ON, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY, pb.GBBFlag_FORCE_DEV_BOOT_USB)
		} else {
			common.GBBAddFlag(&flags, pb.GBBFlag_FORCE_DEV_SWITCH_ON, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)
		}
	}
	noECSync, err := i.varToBool(s, "firmware.no_ec_sync")
	if err != nil {
		s.Fatal("ECSync: ", err)
	}
	if noECSync {
		common.GBBAddFlag(&flags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC)
		s.Log("User selected to disable EC software sync")
	}
	i.value.GBBFlags = &flags
	// If rebooting to recovery mode, verify the usb key.
	if i.value.BootMode == common.BootModeRecovery || i.value.BootMode == common.BootModeUSBDev {
		if err := i.value.Helper.RequireServo(ctx); err != nil {
			s.Error("Test did not run")
			s.Fatal("Failed to connect to servod: ", err)
		}
		connectTimeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
		defer cancel()
		if err := i.value.Helper.WaitConnect(connectTimeout); err != nil {
			s.Error("Test did not run")
			s.Fatal("Failed to connect to DUT: ", err)
		}
		skipFlashUSB := false
		skipFlashUSB, err = i.varToBool(s, "firmware.skipFlashUSB")
		if err != nil {
			s.Fatal("SkipFlashUSB: ", err)
		}
		cs := s.CloudStorage()
		if skipFlashUSB {
			cs = nil
		}
		if err := i.value.Helper.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("Failed to setup USB key: ", err)
		}
	}
	return i.value
}

// Reset is called by the framework after each test (except for the last one) to do a
// light-weight reset of the environment to the original state.
func (i *impl) Reset(ctx context.Context) error {
	// Close the servo to reset pd role, watchdogs, etc.
	i.value.Helper.CloseServo(ctx)
	// Close the RPC client in case the DUT rebooted at some point, and it doesn't recover well.
	i.value.Helper.CloseRPCConnection(ctx)
	return nil
}

// PreTest is called by the framework before each test to do a light-weight set up for the test.
func (i *impl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if err := i.value.Helper.RequireServo(ctx); err != nil {
		s.Error("Test did not run")
		s.Fatal("Failed to connect to servo: ", err)
	}
	// Write an echo to servod, just to make the test name appear in the logs.
	if _, err := i.value.Helper.Servo.Echo(ctx, fmt.Sprintf("Test start: %s", s.TestName())); err != nil {
		s.Error("Test did not run")
		s.Fatal("Servo echo failed: ", err)
	}
	// Check whether CCD connection exists, if it does, attempt to open
	// CCD with all capabilities set to factory settings. If CCD is locked,
	// transitioning the dut from one mode to another might fail, returning
	// one error that says "EC: No data was sent from the pty".
	hasCCD, err := i.value.Helper.Servo.HasCCD(ctx)
	if err != nil {
		s.Fatal("Failed to check for CCD connection: ", err)
	}
	hasC2D2, err := i.value.Helper.Servo.HasC2D2(ctx)
	if err != nil {
		s.Fatal("Failed to check for CCD connection: ", err)
	}
	if hasCCD || hasC2D2 {
		s.Log("Ensuring CCD open, testlab enabled, and capabilities set to factory settings")
		if err := i.value.Helper.OpenCCD(ctx, true, true); err != nil {
			s.Fatal("Failed to set CCD open: ", err)
		}
	}
	if hasC2D2 {
		if err := i.value.Helper.Servo.SetCCDCapability(ctx, map[servo.CCDCap]servo.CCDCapState{
			servo.OverrideWP:     servo.CapAlways,
			servo.GscFullConsole: servo.CapAlways,
			servo.RebootECAP:     servo.CapAlways,
		}); err != nil {
			s.Log("Failed to set CCD capability: ", err)
		}
	}

	// Make sure the lid is open
	if ok, err := i.value.Helper.Servo.HasControl(ctx, string(servo.LidOpen)); err != nil {
		s.Fatalf("Failed to check control %s: %v", servo.LidOpen, err)
	} else if ok {
		if err := i.value.Helper.Servo.SetString(ctx, servo.LidOpen, string(servo.LidOpenYes)); err != nil {
			s.Fatal("Failed to open lid: ", err)
		}
	}

	// Check whether the DUT has a CrOS EC that can talk through servo UART.
	// If CrOS EC exists, check for ec responsiveness. Fail the test as
	// "test did not run" if ec is not responsive because a non-responsive EC
	// would cause failure in communicating information from/to the EC, and
	// there are a good number of such commands used in FAFT tests.
	supportCrosEC, err := i.value.Helper.Servo.GetString(ctx, servo.SupportCrosECComm)
	if err != nil {
		s.Logf("Failed to get value for %s: %v", servo.SupportCrosECComm, err)
	}
	if supportCrosEC == "yes" {
		// If CrOS ec exists, verify that ec is responsive.
		s.Log("Sending an ec command to check if ec is responsive")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := i.value.Helper.Servo.RunECCommandGetOutput(ctx, "version", []string{`[^\x00]+`}); err != nil {
				return errors.Wrap(err, "ec not responsive")
			}
			return nil
		}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 2 * time.Minute}); err != nil {
			s.Error("Test did not run")
			s.Fatal("Sending ec command failed: ", err)
		}
		s.Log("EC is active")
	}

	if i.disallowSSH {
		return
	}

	// The GBB flags might prevent booting into the correct mode, so check the boot mode,
	// then save the GBB flags, then set the GBB flags, and finally reboot into the right mode.
	mode, err := i.value.Helper.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Failed to get current boot mode: ", err)
	}

	// If this is the first PreTest invocation, save the starting boot mode.
	// This isn't in SetUp to avoid reading CurrentBootMode twice.
	if i.origBootMode == nil {
		s.Logf("Saving boot mode %q for restoration upon completion of all tests under this fixture", mode)
		i.origBootMode = &mode
	}

	curr, err := common.GetGBBFlags(ctx, i.value.Helper.DUT)
	if err != nil {
		s.Fatal("Failed to read GBB flags: ", err)
	}

	// If this is the first PreTest invocation, save the starting GBB flags.
	// This isn't in SetUp to avoid reading GetGBBFlags twice. (It's very slow)
	if i.origGBBFlags == nil {
		i.origGBBFlags = common.CopyGBBFlags(curr)
		i.value.GBBFlags = common.GBBFlagsStateClearSet(curr, i.value.GBBFlags)
		// For backwards compatibility with Tauto FAFT tests, firmware.no_ec_sync=true will leave DISABLE_EC_SOFTWARE_SYNC set after the test is over. See b/194807451
		// TODO(jbettis): Consider revisiting this flag with something better.
		if common.GBBFlagsContains(i.value.GBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
			common.GBBAddFlag(i.origGBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC)
		}
		testing.ContextLogf(ctx, "Saving GBB flags %+v for restoration upon completion of all tests under this fixture", i.origGBBFlags.Set)
	}

	rebootRequired := false
	var opts []firmware.ModeSwitchOption
	if common.GBBFlagsStatesEqual(i.value.GBBFlags, curr) {
		s.Log("GBBFlags are already proper")
	} else {
		s.Log("Pretest setting GBB flags to ", i.value.GBBFlags.Set)
		if err := common.SetGBBFlags(ctx, i.value.Helper.DUT, i.value.GBBFlags.Set); err != nil {
			s.Log("Disabling write protect to allow GBB flags to be set")
			// Read the hardware WP state, and disable if necessary
			if val, err := i.value.Helper.Servo.GetString(ctx, servo.FWWPState); err != nil {
				s.Fatal("Failed to query write protect: ", err)
			} else if val == "on" || val == string(servo.FWWPStateOn) {
				if err := i.value.Helper.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
					s.Fatal("Failed to disable write protect: ", err)
				}
				// A reboot is required after changing the wp state.
				ms, err := firmware.NewModeSwitcher(ctx, i.value.Helper)
				if err != nil {
					s.Fatal("Failed to create mode switcher: ", err)
				}
				if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
					s.Fatal("Failed to warm reboot: ", err)
				}
			}

			var flashromConfig flashrom.Config
			flashromInstance, ctx, shutdown, _, err := flashromConfig.
				FlashromInit(flashrom.VerbosityInfo).
				ProgrammerInit(flashrom.ProgrammerHost, "").
				SetDut(i.value.Helper.DUT).
				Probe(ctx)

			defer func() {
				if err := shutdown(); err != nil {
					s.Error("Failed to shutdown flashromInstance: ", err)
				}
			}()

			if err != nil {
				s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
			}

			if out, err := flashromInstance.SoftwareWriteProtectDisable(ctx); err != nil {
				s.Logf("Software WP disable failed with output: %s", string(out))
				s.Fatal("Failed to disable software WP: ", err)
			}

			if err := common.SetGBBFlags(ctx, i.value.Helper.DUT, i.value.GBBFlags.Set); err != nil {
				s.Fatal("SetGBBFlags failed: ", err)
			}
		}
		if common.GBBFlagsChanged(curr, i.value.GBBFlags, common.RebootRequiredGBBFlags()) {
			opts = append(opts, firmware.RebootForGBBFlagsChanged)
			rebootRequired = true
		}
	}

	if mode != i.value.BootMode {
		testing.ContextLogf(ctx, "Current boot mode is %q, rebooting to %q to satisfy fixture", mode, i.value.BootMode)
		rebootRequired = true
	}

	if rebootRequired {
		opts = append(opts, firmware.AssumeGBBFlagsCorrect)
		if i.value.ForcesDevMode {
			opts = append(opts, firmware.AllowGBBForce)
		}
		// Only copy the tast files if it is allowed, and also if we are in the boot mode we started in.
		if i.copyTastFiles && mode == *i.origBootMode {
			opts = append(opts, firmware.CopyTastFiles)
		}

		if i.value.BootMode == common.BootModeRecovery {
			// Read the hardware WP state, and disable if necessary
			if val, err := i.value.Helper.Servo.GetString(ctx, servo.FWWPState); err != nil {
				s.Fatal("Failed to query write protect: ", err)
			} else if val == "on" || val == string(servo.FWWPStateOn) {
				s.Log("Disabling write protect to allow PD negotiation in EC-RO")
				if err := i.value.Helper.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
					s.Fatal("Failed to disable write protect: ", err)
				}
			}
		}

		if err := rebootToMode(ctx, i.value.Helper, i.value.BootMode, opts...); err != nil {
			s.Fatalf("Failed to reboot to mode %q: %s", i.value.BootMode, err)
		}
	}

	if i.value.ForceWPEnable {
		s.Log("Enabling software and hardware write protect")
		if err := i.value.Helper.DUT.Conn().CommandContext(ctx, "futility", "flash", "--wp-enable").Run(); err != nil {
			s.Fatal("Failed to enable software write protect: ", err)
		}
		if err := i.value.Helper.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
			s.Fatal("Failed to enable write protect: ", err)
		}
	}
}

// PostTest is called by the framework after each test to tear down changes PreTest made.
func (i *impl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := i.value.Helper.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod: ", err)
	}
	if err := i.value.Helper.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("DUT is offline after test end: ", err)
	}
	// Restarting UI logs out any potential chrome sessions logged in during a test.
	if err := i.value.Helper.RestartUI(ctx); err != nil {
		s.Fatal("Failed to restart ui after test end: ", err)
	}
}

// TearDown is called by the framework to tear down the environment SetUp set up.
func (i *impl) TearDown(ctx context.Context, s *testing.FixtState) {
	defer func(ctx context.Context) {
		i.closeHelper(ctx, s)
		i.origBootMode = nil
		i.origGBBFlags = nil
	}(ctx)

	// If we enabled WP during setup, then disable here so we can update GBB flags if needed
	if i.value.ForceWPEnable {
		s.Log("Disabling software and hardware write protect")
		if err := i.value.Helper.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
			s.Fatal("Failed to disable write protect: ", err)
		}
		if err := i.value.Helper.DUT.Conn().CommandContext(ctx, "futility", "flash", "--wp-disable").Run(); err != nil {
			s.Fatal("Failed to disable software write protect: ", err)
		}
	}

	// Close the servo to reset pd role, watchdogs, etc. unless we are booted from USB as resetting the pd role will make the dut reboot.
	if i.value.BootMode != common.BootModeRecovery && i.value.BootMode != common.BootModeUSBDev {
		i.value.Helper.CloseServo(ctx)
	}
	// Close the RPC client in case the DUT rebooted at some point, and it doesn't recover well.
	i.value.Helper.CloseRPCConnection(ctx)

	if err := i.value.Helper.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("DUT is offline after test end: ", err)
	}

	if i.disallowSSH {
		return
	}

	rebootRequired := false
	toMode := common.BootModeUnspecified
	setGBBFlagsAfterReboot := false
	if i.origBootMode != nil {
		mode, err := i.value.Helper.Reporter.CurrentBootMode(ctx)
		if err != nil {
			s.Fatal("Failed to get boot mode: ", err)
		}
		if mode != *i.origBootMode {
			s.Logf("Restoring boot mode from %q to %q", mode, *i.origBootMode)
			rebootRequired = true
			toMode = *i.origBootMode
		}
	}

	if i.origGBBFlags != nil {
		curr, err := common.GetGBBFlags(ctx, i.value.Helper.DUT)
		if err != nil {
			s.Fatal("Getting current GBB Flags failed: ", err)
		}

		if !common.GBBFlagsStatesEqual(i.origGBBFlags, curr) {
			tempGBBFlags := common.CopyGBBFlags(i.origGBBFlags)

			// If we need to reboot the boot mode, we must have common.FAFTGBBFlags() set, but then we might have to restore the GBB flags again.
			if rebootRequired {
				common.GBBAddFlag(tempGBBFlags, common.FAFTGBBFlags()...)
				setGBBFlagsAfterReboot = !common.GBBFlagsStatesEqual(tempGBBFlags, i.origGBBFlags)
			}

			s.Log("Setting GBB flags to ", tempGBBFlags.Set)
			if _, err := common.ClearAndSetGBBFlags(ctx, i.value.Helper.DUT, tempGBBFlags); err != nil {
				s.Fatal("Restore GBB flags failed: ", err)
			}
			if common.GBBFlagsChanged(curr, tempGBBFlags, common.RebootRequiredGBBFlags()) {
				s.Log("Resetting DUT due to GBB flag change in fixture")
				rebootRequired = true
			}
		}
	}

	if rebootRequired {
		opts := []firmware.ModeSwitchOption{firmware.AssumeGBBFlagsCorrect}
		if i.origGBBFlags != nil {
			if common.GBBFlagsContains(i.origGBBFlags, pb.GBBFlag_FORCE_DEV_SWITCH_ON) {
				opts = append(opts, firmware.AllowGBBForce)
			}
		}
		if err := rebootToMode(ctx, i.value.Helper, toMode, opts...); err != nil {
			s.Errorf("Failed to reboot to mode %q: %s", toMode, err)
		}
		// Make sure the DUT is booted, just in case the rebootToMode failed.
		if err := i.value.Helper.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("DUT is offline after test end: ", err)
		}
		if setGBBFlagsAfterReboot {
			s.Log("Setting GBB flags to ", i.origGBBFlags.Set)
			if _, err := common.ClearAndSetGBBFlags(ctx, i.value.Helper.DUT, i.origGBBFlags); err != nil {
				s.Fatal("Restore GBB flags failed: ", err)
			}
		}
	}
}

// String identifies this fixture.
func (i *impl) String() string {
	name := string(i.value.BootMode)
	if i.value.ForcesDevMode {
		name += "-gbb"
	}
	return name
}

// initHelper ensures that the impl has a working Helper instance.
func (i *impl) initHelper(ctx context.Context, s *testing.FixtState) {
	if i.value.Helper == nil {
		servoSpec, _ := s.Var("servo")

		if i.disallowSSH {
			i.value.Helper = firmware.NewHelperWithoutDUT(s.DataPath(firmware.ConfigFile), servoSpec, s.DUT().KeyFile(), s.DUT().KeyDir())
			i.value.Helper.DisallowServices()
		} else {
			dutHostname, _ := s.Var("dutHostname")
			if dutHostname == "" {
				host, _, err := net.SplitHostPort(s.DUT().HostName())
				if err != nil {
					testing.ContextLogf(ctx, "Failed to extract DUT hostname from %q, use --var=dutHostname to set", s.DUT().HostName())
				}
				dutHostname = host
			}
			powerunitHostname, _ := s.Var("powerunitHostname")
			powerunitOutlet, _ := s.Var("powerunitOutlet")
			hydraHostname, _ := s.Var("hydraHostname")
			i.value.Helper = firmware.NewHelper(s.DUT(), s.RPCHint(), s.DataPath(firmware.ConfigFile), servoSpec, dutHostname, powerunitHostname, powerunitOutlet, hydraHostname)
			if !i.copyTastFiles {
				i.value.Helper.DisallowServices()
			}
		}
	}
}

// closeHelper closes and nils any existing Helper instance.
func (i *impl) closeHelper(ctx context.Context, s *testing.FixtState) {
	if i.value.Helper == nil {
		return
	}
	if err := i.value.Helper.Close(ctx); err != nil {
		s.Fatal("Failed to close helper: ", err)
	}
	i.value.Helper = nil
}

// rebootToMode reboots to the specified mode using the ModeSwitcher, it assumes the helper is present.
func rebootToMode(ctx context.Context, h *firmware.Helper, mode common.BootMode, opts ...firmware.ModeSwitchOption) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	checkPowerState := func() string {
		powerState := "unknown"
		testing.ContextLog(ctx, "Checking for the DUT's power state")
		if hasEC, err := h.Servo.HasControl(ctx, string(servo.ECSystemPowerState)); err != nil {
			testing.ContextLog(ctx, "Failed to check for chrome ec: ", err)
			return powerState
		} else if hasEC {
			out, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				testing.ContextLog(ctx, "Failed to check for power state: ", err)
				return powerState
			}
			powerState = out
		}
		return powerState
	}
	if mode == common.BootModeUnspecified {
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			powerState := checkPowerState()
			return errors.Wrapf(err, "failed to warm reboot, got power state %s", powerState)
		}
		return nil
	}
	if err := ms.RebootToMode(ctx, mode, opts...); err != nil {
		powerState := checkPowerState()
		return errors.Wrapf(err, "failed to reboot to mode %q, got power state %s", mode, powerState)
	}

	return nil
}

type reinstall struct {
	parentValue *Value
}

func (r *reinstall) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	r.parentValue = s.ParentValue().(*Value)
	return r.parentValue
}

func (r *reinstall) Reset(ctx context.Context) error {
	return nil
}

func (r *reinstall) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (r *reinstall) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (r *reinstall) TearDown(ctx context.Context, s *testing.FixtState) {
	mode, err := r.parentValue.Helper.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Failed to get current boot mode: ", err)
	}

	if mode != r.parentValue.BootMode {
		testing.ContextLogf(ctx, "Current boot mode is %q, rebooting to %q to satisfy fixture", mode, r.parentValue.BootMode)
		var opts []firmware.ModeSwitchOption
		if r.parentValue.ForcesDevMode {
			opts = append(opts, firmware.AllowGBBForce)
		}
		if err := rebootToMode(ctx, r.parentValue.Helper, r.parentValue.BootMode, opts...); err != nil {
			s.Fatalf("Failed to reboot to mode %q: %s", r.parentValue.BootMode, err)
		}
	}
	s.Log("Reinstalling ChromeOS")
	cmd := r.parentValue.Helper.DUT.Conn().CommandContext(ctx, "/usr/sbin/chromeos-install", "--yes")
	if err := cmd.Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to reinstall ChromeOS: ", err)
	}
	r.parentValue.Helper.DUTHasNoTastFilesInternalDisk()
}
