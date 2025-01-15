// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast/core/testing"
)

const (
	// GSCInitialFactory fixture ensures the testlab is enabled at startup and that tpm is reset between
	// each tests
	GSCInitialFactory = "gscInitialFactory"

	rescueTwiceTimeout = 10 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            GSCInitialFactory,
		Desc:            "Ensures GSC is in the initial factory mode state with cleared INFO pages",
		Contacts:        []string{"cros-hwsec@google.com", "jettrink@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Impl:            &initialFactoryImpl{},
		SetUpTimeout:    rescueTwiceTimeout,
		TearDownTimeout: rescueTwiceTimeout,
		PreTestTimeout:  rescueTwiceTimeout,
		PostTestTimeout: rescueTwiceTimeout,
		Parent:          SystemDevboard,
	})
}

type initialFactoryImpl struct {
	v                  *Value
	doneFirstTimeSetup bool
}

func (c *initialFactoryImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	testing.ContextLog(ctx, "GSC Initial Factory Setup")
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
	if _, err := c.v.EfiImagePath(ctx); err != nil {
		s.Fatal(err, "failed to download the efi image")
	}
	if _, err := c.v.DebugImagePath(ctx); err != nil {
		s.Fatal(err, "failed to download the debug image")
	}
	testing.ContextLog(ctx, "End GSC Initial Factory Setup")
	return c.v
}

func eraseInfoPage(ctx context.Context, v *Value, s TestingState) {
	b := v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start EFI session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s, v.TestbedProperties.TestbedType)
	defer i.Close(ctx)

	mustSucceed(s, b.Reset(ctx), "Reset gsc console for EFI")
	mustSucceed(s, i.WaitUntilBooted(ctx), "EFI image revives after reboot")

	eraseOutput, err := i.EraseFlashInfo(ctx)
	mustSucceed(s, err, "failed to run eraseflashinfo command")
	if !strings.Contains(eraseOutput, "Succeeded!") {
		s.Fatal("Erase command did not work: ", eraseOutput)
	}
	testing.ContextLog(ctx, "GSC INFO page erased")

	isErased, err := i.WriteOnceInfoPagesAreErased(ctx)
	mustSucceed(s, err, "checking info pages")
	testing.ContextLogf(ctx, "IsErased: %t", isErased)
	if !isErased {
		s.Fatal("Failed to erase pages: ", eraseOutput)
	}

	version, err := i.VersionInfo(ctx)
	mustSucceed(s, err, "checking version")
	testing.ContextLogf(ctx, "version: %+v", version)
}

func waitUntilDBGBoots(ctx context.Context, v *Value, s TestingState) {
	b := v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start DBG session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s, v.TestbedProperties.TestbedType)
	defer i.Close(ctx)

	mustSucceed(s, b.Reset(ctx), "Reset gsc console for DBG")
	mustSucceed(s, i.WaitUntilBooted(ctx), "DBG image revives after reboot")

	version, err := i.VersionInfo(ctx)
	mustSucceed(s, err, "checking version")
	testing.ContextLogf(ctx, "DBG version: %+v", version)
}

func eraseAPROVerificationSettings(ctx context.Context, v *Value, s TestingState) {
	// Haven does not have AP RO verification settings that need to be erased
	if v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		return
	}
	b := v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start ap ro erase session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s, v.TestbedProperties.TestbedType)
	defer i.Close(ctx)

	mustSucceed(s, b.Reset(ctx), "Reset gsc console")
	mustSucceed(s, i.WaitUntilBooted(ctx), "Test image revives after reboot")

	apEraseOutput := runCommand(ctx, s, i, "ap_ro_verify erase")
	if strings.Contains(apEraseOutput, "failed") {
		s.Fatal("Could not erase AP RO settings command did not work: ", apEraseOutput)
	}
	testing.ContextLog(ctx, "AP RO verification settings erased")
}

// setupImageAndEraseInfo runs eraseflashinfo and flashes the image under test
// on the devboard using the image and json files provided in the `Value`
// parameter.
func setupImageAndEraseInfo(ctx context.Context, v *Value, s TestingState, force bool) {
	// Setup the board with the correct jsons. Use an empty string for the image
	// path so setup doesn't try to flash the image.
	if err := v.devboard.Setup(ctx, "", v.FwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	b := v.devboard
	if !force && currentImageGood(ctx, s, b, v.ImagePath, true, v.TestbedProperties.TestbedType) {
		testing.ContextLog(ctx, "Image is already running and info1 is erased")
		return
	}

	testing.ContextLog(ctx, "Setting up image and running eraseflashinfo: ", v.ImagePath)
	if v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		setupCr50Image(ctx, s, b, v.ImagePath, v.FwConfigJsons, v.TestbedProperties, true, v.TestbedProperties.TestbedType)
	} else {
		defer func() {
			// Make every effort to flash image under test in the end, even if
			// operations on DBG or EFI image results in fatal error.  In such cases,
			// this particular test case will be bound to fail, but subsequent test
			// cases may not use EFI, and the parent fixture SystemDevboard expects
			// the image under test to still be on the GSC when a testcase terminates.

			// Longer term, we should probably refactor this fixture, such that
			// GSCInitialFactory does not have any parent, and the function of
			// SystemDevboard would become helper methods used by fixture.
			testing.ContextLog(ctx, "Flashing image under test")
			mustSucceed(s, b.Setup(ctx, v.ImagePath, v.FwConfigJsons), "Setup for image under test")
		}()

		testing.ContextLog(ctx, "Flashing DBG image to erase filesystem and TPM")
		debugImagePath, _ := v.DebugImagePath(ctx)
		mustSucceed(s, b.Setup(ctx, debugImagePath, []string{}), "Setup DBG image")
		waitUntilDBGBoots(ctx, v, s)

		testing.ContextLog(ctx, "Flashing EFI image")
		efiImagePath, _ := v.EfiImagePath(ctx)
		mustSucceed(s, b.Setup(ctx, efiImagePath, []string{}), "Setup EFI image")
		eraseInfoPage(ctx, v, s)

		// Here, the image under test will be flashed by above "defer" statement.
	}
}

func (c *initialFactoryImpl) UpdateAndRunEraseFlashInfo(ctx context.Context, s TestingState, force bool) {
	// Host emulation does not need to erase anything, it always started erased
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		return
	}

	setupImageAndEraseInfo(ctx, c.v, s, force)
	eraseAPROVerificationSettings(ctx, c.v, s)
	mustSucceed(s, c.v.devboard.StartSession(ctx, ti50.StrapReset), "Start testing session")
}

func (c *initialFactoryImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Start GSC Initial Factory Fixture PreTest")

	// Inform fixture that this test may replace the firmware image in flash.
	mustSucceed(s, c.v.ImageMayBeUpdatedByTest(), "Failed to mark image as possibly updated")

	// The first update should force a complete reset of filesystem and INFO pages
	forceUpdate := !c.doneFirstTimeSetup
	c.doneFirstTimeSetup = true
	c.UpdateAndRunEraseFlashInfo(ctx, s, forceUpdate)

	testing.ContextLog(ctx, "Board ready for test")
}

func (c *initialFactoryImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (c *initialFactoryImpl) Reset(ctx context.Context) error {
	return nil
}

func (c *initialFactoryImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	testing.ContextLog(ctx, "Start GSC Initial Factory Fixture TearDown")

	c.UpdateAndRunEraseFlashInfo(ctx, s, true)
}
