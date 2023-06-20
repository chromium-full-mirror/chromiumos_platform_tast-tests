// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"

	"go.chromium.org/tast/core/testing"
)

const (
	// Ti50CcdOpen fixture ensures the testlab is enabled at startup and that tpm is reset between
	// each tests
	Ti50CcdOpen = "ti50CcdOpen"

	testLabOpenTimeout = 30 * time.Second
)

var (
	pushButton     = regexp.MustCompile("Press the physical button now")
	testLabEnabled = regexp.MustCompile("Updating testlab to true")
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:           Ti50CcdOpen,
		Desc:           "Ensures that CCD is open and TPM is cleared before every test",
		Contacts:       []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:           &ccdOpenImpl{},
		PreTestTimeout: testLabOpenTimeout,
		Parent:         Ti50Devboard,
	})
}

// Copied from tpm_helper

func gpioSet(ctx context.Context, s *testing.FixtTestState, b ti50.DevBoard, g ti50.GpioName, val bool) {
	if _, err := b.PlainCommand(ctx, "gpio", "write", string(g), strconv.FormatBool(val)); err != nil {
		s.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

func gpioApplyStrap(ctx context.Context, s *testing.FixtTestState, b ti50.DevBoard, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := b.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
			s.Fatalf("Failed to apply gpio strap %s: %s", strap, err)
		}
	}
}

func mustSucceed(s *testing.FixtTestState, err error, format string, args ...interface{}) {
	if err != nil {
		s.Fatalf(format+": %s", append(args, err)...)
	}
}

func runCommand(ctx context.Context, s *testing.FixtTestState, image *ti50.CrOSImage, command string) string {
	out, err := image.Command(ctx, command)
	if err != nil {
		s.Fatalf("Running Command `%s` failed", command)
	}
	return out
}

type ccdOpenImpl struct {
	v *Value
}

func (c *ccdOpenImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if s.ParentValue() != nil {
		c.v = s.ParentValue().(*Value)
	}
	// Note that devboard services does not start until PreTest, so we cannot enable testlab mode
	// in setup
	return c.v
}

func (c *ccdOpenImpl) Reset(ctx context.Context) error {
	return nil
}

func (c *ccdOpenImpl) ensureTestLabOpen(ctx context.Context, s *testing.FixtTestState) {
	b := c.v.devboard
	i := ti50.NewCrOSImage(b)

	mustSucceed(s, i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	out := runCommand(ctx, s, i, "ccd testlab")

	if strings.Contains(out, "CCD test lab mode enabled") {
		s.Log("Testlab mode already enabled")
		return
	}

	s.Log("Testlab mode not enabled")
	s.Log("Restarting ti50 with CCD, SPI, and Clamshell straps and AP on to remove FWMP")
	gpioApplyStrap(ctx, s, b, ti50.CcdSuzyQ, ti50.TpmSpi, ti50.FfClamshell)
	gpioSet(ctx, s, b, ti50.GpioTi50PltRstL, true)
	mustSucceed(s, b.Reset(ctx), "Reset board")
	mustSucceed(s, i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Removing FWMP space if present (which can block ccd open)")
	tpm := ti50.NewTpmHandle(ctx, b, ti50.TpmBusSpi)
	tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.FwmpFileID)

	s.Logf("Setting %s to high", ti50.GpioTi50ChassisOpen)
	gpioSet(ctx, s, b, ti50.GpioTi50ChassisOpen, true)

	s.Log("Opening CCD with chassis open and no FWMP space. Should be instant")
	out = runCommand(ctx, s, i, "ccd open")
	if !strings.Contains(out, "CCD Opened") {
		s.Fatal("CCD did not open, but got instead: ", out)
	}

	s.Log("Setting testlab to enabled")
	// Reset to clear chip factory mode to allow testlab enable.
	runCommand(ctx, s, i, "ccd reset")
	// Use WriteSerial here so we can WaitUntilMatch(pushButton) below. runCommand doesn't work
	// because the pushButton message comes before the console prompt.
	mustSucceed(s, b.WriteSerial(ctx, []byte("ccd testlab enable\r")), "Testlab enable")

	for powerPush := 1; powerPush <= 5; powerPush++ {
		// Wait for prompt before pushing
		err := i.WaitUntilMatch(ctx, pushButton, time.Second*2)
		mustSucceed(s, err, "Power button prompt %d did not happen", powerPush)
		// Ti50 requires 100ms delay between short presses.
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		gpioSet(ctx, s, b, ti50.GpioTi50PowerBtnL, false)
		gpioSet(ctx, s, b, ti50.GpioTi50PowerBtnL, true)
	}
	err := i.WaitUntilMatch(ctx, testLabEnabled, time.Second*2)
	mustSucceed(s, err, "Testlab was not enabled")

	s.Log("Testlab mode is now enabled")
}

func (c *ccdOpenImpl) wipeTpmAndOpenCcd(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Erasing TPM data and opening CCD")
	i := ti50.NewCrOSImage(c.v.devboard)
	// If we don't close the UART connections here, then tests don't get uart data correctly
	defer c.v.devboard.Close(ctx)
	mustSucceed(s, i.WaitUntilBooted(ctx), "GSC did not boot")
	runCommand(ctx, s, i, "ccd testlab open")
	runCommand(ctx, s, i, "ccd reset factory")
	runCommand(ctx, s, i, "ccd set OpenNoTPMWipe ifopened")
	runCommand(ctx, s, i, "ccd lock")
	runCommand(ctx, s, i, "ccd open")
	runCommand(ctx, s, i, "ccd reset factory")
}

func (c *ccdOpenImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	props, err := c.v.devboard.Query(ctx)
	mustSucceed(s, err, "Testbed query failed")
	if props.TestbedType == ti50.GscHostEmulation {
		// TODO(b/283151960): Enabling Testlab mode not yet supported on host emulation
		// (no SPI).
		return
	}
	if props.TestbedType == ti50.GscOpentitanCw310Fpga {
		// TODO(jbk): Once OpenTitan port has SPI TPM capability, and other feature
		// parity, this should be re-enabled.
		return
	}

	// Ensure that test lab is open before we try to open ccd
	c.ensureTestLabOpen(ctx, s)
	c.wipeTpmAndOpenCcd(ctx, s)
	testing.ContextLog(ctx, "Board ready for test")
}

func (c *ccdOpenImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (c *ccdOpenImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}
