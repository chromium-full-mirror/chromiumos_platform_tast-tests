// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package slimrootfsutils

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "slimRootfs",
		Desc:            "Provides a logged in Chrome session and cleans up VM images on TearDown",
		Contacts:        []string{"cros-virt-devices-guests@google.com", "uekawa@google.com"},
		BugComponent:    "b:1248538", // ChromeOS > Platform > Virtualization > Device and Guests
		Impl:            &slimRootfsFixture{},
		Parent:          "chromeLoggedIn",
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		TearDownTimeout: 30 * time.Second,
	})
}

// FixtData is the data returned by the slimRootfs fixture.
type FixtData struct {
	chrome    *chrome.Chrome
	concierge *vm.Concierge
}

// Chrome implements the chrome.HasChrome interface.
func (d *FixtData) Chrome() *chrome.Chrome {
	return d.chrome
}

// Concierge returns the Concierge instance.
func (d *FixtData) Concierge() *vm.Concierge {
	return d.concierge
}

// slimRootfsFixture implements testing.FixtureImpl.
type slimRootfsFixture struct {
	fixtData *FixtData
}

func (f *slimRootfsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(chrome.HasChrome).Chrome()
	concierge, err := vm.NewConcierge(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to connect to concierge: ", err)
	}

	f.fixtData = &FixtData{
		chrome:    cr,
		concierge: concierge,
	}
	return f.fixtData
}

func (f *slimRootfsFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	concierge := f.fixtData.concierge

	disks, err := concierge.ListAllVMDisks(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to list VM disks for cleanup: ", err)
		return
	}

	for _, disk := range disks {
		name := disk.GetName()
		if strings.HasPrefix(name, DefaultVMName) {
			if err := concierge.DestroyDiskImage(ctx, name); err != nil {
				testing.ContextLogf(ctx, "Failed to clean up VM %s: %v", name, err)
			}
		}
	}
}

func (f *slimRootfsFixture) Reset(ctx context.Context) error {
	// Let the parent fixture (chromeLoggedIn) handle resetting.
	return nil
}

func (f *slimRootfsFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *slimRootfsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
