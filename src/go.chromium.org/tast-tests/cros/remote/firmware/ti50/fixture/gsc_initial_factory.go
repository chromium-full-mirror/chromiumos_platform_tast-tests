// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast/core/testing"
)

const (
	// GSCInitialFactory fixture ensures the testlab is enabled at startup and that tpm is reset between
	// each tests
	GSCInitialFactory = "gscInitialFactory"

	// gsEfiLocation specifies the GS bucket location with %s placeholders for dev ids
	gsEfiLocation = "gs://chromeos-localmirror-private/distfiles/chromeos-ti50-debug/dt_shield/ti50_Unknown_NodeLocked-%s_ti50-accessory-mp.bin"
	// tmpEfiLocation specifies the format of temporary file for EFI images
	tmpEfiLocation = "ti50-efi-%s.*.bin"

	copyFromGSTimeout  = 30 * time.Second
	rescueTwiceTimeout = 2 * time.Minute
	removeFileTimeout  = 5 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            GSCInitialFactory,
		Desc:            "Ensures GSC is in the initial factory mode state with cleared INFO pages",
		Contacts:        []string{"gsc-sheriff@google.com", "jettrink@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Impl:            &initialFactoryImpl{},
		SetUpTimeout:    copyFromGSTimeout,
		TearDownTimeout: removeFileTimeout,
		PreTestTimeout:  rescueTwiceTimeout,
		Parent:          SystemDevboard,
	})
}

type initialFactoryImpl struct {
	v            *Value
	efiImagePath string
}

func (c *initialFactoryImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if s.ParentValue() != nil {
		c.v = s.ParentValue().(*Value)
	}
	// Host emulation does not need to erase anything, it always started erased
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		return c.v
	}
	if c.v.ImagePath == "" {
		s.Fatal("InitialFactory fixture must specify a image (i.e. through buildurl var)")
	}

	// Create and close temp file immediately so we can overwrite it
	file, err := os.CreateTemp("", fmt.Sprintf(tmpEfiLocation, c.v.TestbedProperties.UsbSerial))
	if err != nil {
		s.Fatal("Could not open temp file for efi: ", err)
	}
	file.Close()
	c.efiImagePath = file.Name()
	gsEfiLocation := fmt.Sprintf(gsEfiLocation, c.v.TestbedProperties.UsbSerial)

	testing.ContextLogf(ctx, "Copying EFI from %q to %q ", gsEfiLocation, c.efiImagePath)
	cmd := exec.CommandContext(ctx, "gsutil", "cp", gsEfiLocation, c.efiImagePath)
	if err := cmd.Run(); err != nil {
		s.Fatal("Could not download efi image: ", err)
	}

	return c.v
}

func (c *initialFactoryImpl) eraseInfoPage(ctx context.Context, s *testing.FixtTestState) {
	b := c.v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start EFI session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s)
	defer i.Close(ctx)

	mustSucceed(s, b.Reset(ctx), "Reset gsc console for EFI")
	mustSucceed(s, i.WaitUntilBooted(ctx), "EFI image revives after reboot")

	eraseOutput := runCommand(ctx, s, i, "erase")
	if !strings.Contains(eraseOutput, "Succeeded!") {
		s.Fatal("Erase command did not work: ", eraseOutput)
	}
	testing.ContextLog(ctx, "GSC INFO page erased")
}

func (c *initialFactoryImpl) eraseAPROVerificationSettings(ctx context.Context, s *testing.FixtTestState) {
	// Haven does not have AP RO verification settings that need to be erased
	if c.v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		return
	}
	b := c.v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start ap ro erase session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s)
	defer i.Close(ctx)

	mustSucceed(s, b.Reset(ctx), "Reset gsc console")
	mustSucceed(s, i.WaitUntilBooted(ctx), "Test image revives after reboot")

	apEraseOutput := runCommand(ctx, s, i, "ap_ro_verify erase")
	if strings.Contains(apEraseOutput, "failed") {
		s.Fatal("Could not erase AP RO settings command did not work: ", apEraseOutput)
	}
	testing.ContextLog(ctx, "AP RO verification settings erased")
}

func (c *initialFactoryImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Host emulation does not need to erase anything, it always started erased
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		return
	}

	b := c.v.devboard

	mustSucceed(s, b.EndSession(ctx), "End image under test session")

	testing.ContextLog(ctx, "Flashing EFI image")
	mustSucceed(s, b.Setup(ctx, c.efiImagePath, []string{}), "Setup EFI image")
	c.eraseInfoPage(ctx, s)

	testing.ContextLog(ctx, "Flashing image under test")
	mustSucceed(s, b.Setup(ctx, c.v.ImagePath, c.v.FwConfigJsons), "Setup for image under test")

	c.eraseAPROVerificationSettings(ctx, s)

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start testing session")

	testing.ContextLog(ctx, "Board ready for test")
}

func (c *initialFactoryImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (c *initialFactoryImpl) Reset(ctx context.Context) error {
	return nil
}

func (c *initialFactoryImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if c.efiImagePath != "" {
		if err := os.Remove(c.efiImagePath); err != nil {
			s.Errorf("Failed to delete EFI image %q: %v", c.efiImagePath, err)
		}
		c.efiImagePath = ""
	}
}
