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

	// gsEfiLocation specifies the GS bucket location with %s placeholders for dev ids
	gsEfiLocation = "gs://chromeos-localmirror-private/distfiles/chromeos-ti50-debug/dt_shield/ti50_Unknown_NodeLocked-%s_ti50-accessory-mp.bin"
	// tmpEfiLocation specifies the format of temporary file for EFI images
	tmpEfiLocation = "ti50-efi-%s.*.bin"

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
	v *Value
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

	testing.ContextLog(ctx, "Saving images for GSC Initial Factory Fixture")
	efiImage, err := DownloadEfiImage(ctx, c.v.TestbedProperties)
	if err != nil {
		s.Fatal(err, "failed to download the efi image")
	}
	debugImage, err := DownloadDebugImage(ctx, c.v.TestbedProperties)
	if err != nil {
		if c.v.TestbedProperties.TestbedType == ti50.GscH1Shield {
			s.Fatal(err, "failed to download the debug image")
		}
		debugImage = ""
	}
	c.v.DebugImagePath = debugImage
	c.v.EfiImagePath = efiImage
	testing.ContextLog(ctx, "End GSC Initial Factory Setup")
	return c.v
}

func eraseInfoPage(ctx context.Context, v *Value, s TestingState) {
	b := v.devboard

	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start EFI session")
	defer b.EndSession(ctx)

	gscConsole := b.PhysicalUart(ti50.UartConsole)
	i := ti50.MustOpenCrOSImage(ctx, gscConsole, s)
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

func eraseAPROVerificationSettings(ctx context.Context, v *Value, s TestingState) {
	// Haven does not have AP RO verification settings that need to be erased
	if v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		return
	}
	b := v.devboard

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

// setupImageAndEraseInfo runs eraseflashinfo and flashes the image under test
// on the devboard using the image and json files provided in the `Value`
// parameter.
func setupImageAndEraseInfo(ctx context.Context, v *Value, s TestingState) {
	// Setup the board with the correct jsons. Use an empty string for the image
	// path so setup doesn't try to flash the image.
	if err := v.devboard.Setup(ctx, "", v.FwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	b := v.devboard
	if needsUpdate(ctx, s, b, v.ImagePath, true) {
		testing.ContextLog(ctx, "Image is already running and info1 is erased")
		return
	}

	testing.ContextLog(ctx, "Setting up image and running eraseflashinfo: ", v.ImagePath)
	if v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		setupCr50Image(ctx, s, b, v.ImagePath, v.FwConfigJsons, v.TestbedProperties, true)
	} else {
		testing.ContextLog(ctx, "Flashing EFI image")
		mustSucceed(s, b.Setup(ctx, v.EfiImagePath, []string{}), "Setup EFI image")
		eraseInfoPage(ctx, v, s)

		testing.ContextLog(ctx, "Flashing image under test")
		mustSucceed(s, b.Setup(ctx, v.ImagePath, v.FwConfigJsons), "Setup for image under test")

	}
}

func (c *initialFactoryImpl) UpdateAndRunEraseFlashInfo(ctx context.Context, s *testing.FixtTestState) {
	// Host emulation does not need to erase anything, it always started erased
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		return
	}

	setupImageAndEraseInfo(ctx, c.v, s)

	if c.v.TestbedProperties.TestbedType != ti50.GscH1Shield {
		eraseAPROVerificationSettings(ctx, c.v, s)
	}
	mustSucceed(s, c.v.devboard.StartSession(ctx, ti50.StrapReset), "Start testing session")
}

func (c *initialFactoryImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Start GSC Initial Factory Fixture PreTest")

	c.UpdateAndRunEraseFlashInfo(ctx, s)

	testing.ContextLog(ctx, "Board ready for test")
}

func (c *initialFactoryImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Start GSC Initial Factory Fixture PostTest")

	c.UpdateAndRunEraseFlashInfo(ctx, s)
}

func (c *initialFactoryImpl) Reset(ctx context.Context) error {
	return nil
}

func (c *initialFactoryImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	// Let's always make sure that the image from the parent SystemDevboard is
	// set up correctly after finishing with this fixture since it is possible
	// in errors cases for the image under test to not on the device anymore.
	setupImage(ctx, s.ParentValue().(*Value), s)
}
