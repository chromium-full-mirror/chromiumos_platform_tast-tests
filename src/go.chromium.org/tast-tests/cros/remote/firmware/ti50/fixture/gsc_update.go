// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast/core/testing"
)

const (
	// GSCUpdate fixture ensures the image under test is running after each test.
	GSCUpdate = "gscUpdate"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            GSCUpdate,
		Desc:            "Ensure the correct gsc image is running before every test",
		Contacts:        []string{"gsc-sheriff@google.com", "mruthven@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Impl:            &updateImpl{},
		SetUpTimeout:    rescueTwiceTimeout,
		PostTestTimeout: rescueTwiceTimeout,
		Parent:          SystemDevboard,
	})
}

type updateImpl struct {
	v *Value
}

func (c *updateImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if s.ParentValue() != nil {
		c.v = s.ParentValue().(*Value)
	}
	// Host emulation does not need to erase anything, it always started erased
	if c.v.TestbedProperties.TestbedType == ti50.GscHostEmulation {
		return c.v
	}
	if c.v.ImagePath == "" {
		s.Fatal("Update fixture must specify a image (i.e. through buildurl var)")
	}
	testing.ContextLog(ctx, "Saving images for GSC Update Fixture")
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
	return c.v
}

func (c *updateImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (c *updateImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	b := c.v.devboard
	mustSucceed(s, b.EndSession(ctx), "End image under test session")
	// Let's always make sure that the image from the parent SystemDevboard is
	// set up correctly after finishing each test since it's possible the test
	// failed before restoring the image.
	setupImage(ctx, c.v, s)
	mustSucceed(s, b.StartSession(ctx, ti50.StrapReset), "Start testing session")
}

func (c *updateImpl) Reset(ctx context.Context) error {
	return nil
}

func (c *updateImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}
