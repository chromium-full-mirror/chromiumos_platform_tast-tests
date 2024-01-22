// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	remoteTi50 "go.chromium.org/tast-tests/cros/remote/firmware/ti50"
	"go.chromium.org/tast/core/testing"
)

const (
	// GSCOpenCCD fixture ensures the testlab is enabled at startup and that tpm is reset between
	// each tests
	GSCOpenCCD = "gscOpenCCD"

	testLabOpenTimeout = 30 * time.Second
)

var (
	ccdOpened = regexp.MustCompile("CCD [Oo]pened")
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:           GSCOpenCCD,
		Desc:           "Ensures that CCD is open and TPM is cleared before every test",
		Contacts:       []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:           &ccdOpenImpl{},
		PreTestTimeout: testLabOpenTimeout,
		Parent:         SystemDevboard,
	})
}

type ccdOpenImpl struct {
	v *Value
}

func (c *ccdOpenImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if s.ParentValue() != nil {
		c.v = s.ParentValue().(*Value)
	}
	return c.v
}

func (c *ccdOpenImpl) Reset(ctx context.Context) error {
	return nil
}

// EnsureFWMPDisabled ensures that FWMP is removed
func EnsureFWMPDisabled(ctx context.Context, s TestingState, b *remoteTi50.DUTControlAndreiboard, i *ti50.CrOSImage) {
	out := runCommand(ctx, s, i, "ccd")
	if !strings.Contains(out, "fwmp_lock") {
		return
	}
	s.Log("FWMP currently enabled. Removing FWMP")

	s.Log("Restarting ti50 with CCD, SPI, and Clamshell straps and AP on to remove FWMP")
	gpioApplyStrap(ctx, s, b, ti50.CcdSuzyQ, ti50.StrapForCcdOpenFixture)
	gpioSet(ctx, s, b, ti50.GpioTi50PltRstL, true)
	mustSucceed(s, b.Reset(ctx), "Reset board")
	mustSucceed(s, i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	tpm := ti50.NewTpmHandle(ctx, b, ti50.TpmBusSpi)

	// Startup, Undefined, then Commit changes
	startup := tpm2.Startup{StartupType: tpm2.TPMSUClear}
	if _, err := startup.Execute(tpm); err != nil {
		s.Fatal("TPM startup error: ", err)
	}
	if err := tpm.NvUndefineSpace(ti50.FwmpAttr()); err != nil {
		s.Fatal("Could not undefined FWMP: ", err)
	}
	if err := tpm.TpmvCommitNvmem(); err != nil {
		s.Fatal("Could not commit nvmem changes: ", err)
	}

	// Turn AP back off
	gpioSet(ctx, s, b, ti50.GpioTi50PltRstL, false)
	s.Log("FWMP space removed")
}

// EnsureTestLabEnabled ensures that testlab mode is open
func EnsureTestLabEnabled(ctx context.Context, s TestingState, b *remoteTi50.DUTControlAndreiboard, i *ti50.CrOSImage) {
	EnsureFWMPDisabled(ctx, s, b, i)

	out := runCommand(ctx, s, i, "ccd testlab")
	if strings.Contains(out, "CCD test lab mode enabled") {
		return
	}

	s.Log("Testlab disabled. Enabling now")
	gpioSet(ctx, s, b, ti50.GpioTi50ChassisOpen, true)

	out = runCommand(ctx, s, i, "ccd open")
	if !ccdOpened.MatchString(out) {
		s.Fatal("CCD did not open, but got instead: ", out)
	}

	// Reset to clear chip factory mode to allow testlab enable.
	runCommand(ctx, s, i, "ccd reset")
	mustSucceed(s, i.StartTestlabEnable(ctx), "Testlab enable")

	for powerPush := 1; powerPush <= 5; powerPush++ {
		// Wait for prompt before pushing
		mustSucceed(s, i.WaitForPowerButtonPrompt(ctx, time.Second*2), "Power button prompt %d did not happen", powerPush)
		// Ti50 requires 100ms delay between short presses.
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		gpioSet(ctx, s, b, ti50.GpioTi50PowerBtnL, false)
		testing.Sleep(ctx, 10*time.Millisecond) // GoBigSleepLint: Simulating button press
		gpioSet(ctx, s, b, ti50.GpioTi50PowerBtnL, true)
	}
	mustSucceed(s, i.WaitForTestlabEnable(ctx, time.Second*2), "Testlab was not enabled")

	s.Log("Testlab mode is now enabled")
}

// WipeTpmAndOpenCcd clears the TPM data. Testlab mode must be enabled first.
func WipeTpmAndOpenCcd(ctx context.Context, s TestingState, b *remoteTi50.DUTControlAndreiboard, i *ti50.CrOSImage) {
	testing.ContextLog(ctx, "Erasing TPM data and opening CCD")
	runCommand(ctx, s, i, "ccd testlab open")
	runCommand(ctx, s, i, "ccd reset factory")
	runCommand(ctx, s, i, "ccd set OpenNoTPMWipe ifopened")
	runCommand(ctx, s, i, "ccd lock")
	runCommand(ctx, s, i, "ccd open")
	runCommand(ctx, s, i, "ccd reset factory")
}

func (c *ccdOpenImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		// TODO(b/283151960): Enabling Testlab mode not yet supported on host emulation
		// (no SPI).
		return
	}

	b := c.v.devboard

	// We can keep the same session that the parent created, but the UART connection must be
	// closed before going into the main test code
	gscConsole := b.PhysicalUart(ti50.UartConsole, time.Second)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s)
	defer i.Close(ctx)

	// Allow GSC of reset so we can perform interact on GSC console
	mustSucceed(s, b.Reset(ctx), "Release GSC from reset")
	mustSucceed(s, i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Ensure that test lab is open before we try to open ccd
	EnsureTestLabEnabled(ctx, s, b, i)
	WipeTpmAndOpenCcd(ctx, s, b, i)

	// Hold GSC in reset until test can take is out of reset after first opening a new UART connection
	gpioApplyStrap(ctx, s, b, ti50.StrapReset)

	testing.ContextLog(ctx, "Board ready for test")
}

func (c *ccdOpenImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (c *ccdOpenImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}
