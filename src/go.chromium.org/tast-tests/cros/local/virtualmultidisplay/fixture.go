// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package virtualmultidisplay contains fixtures, utilities, and interfaces associated with virtual display driver based (VM) multi display testing.
package virtualmultidisplay

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	VirtualMultiDisplay = "virtualMultiDisplay"
)

var drmServices = [...]string{"ui", "cros-camera"}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: VirtualMultiDisplay,
		// go/tast-multi-display-v1
		Desc: "Device is setup for multiple display enable/disable testing",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"brpol@google.com",
		},
		Impl: NewMultiDisplayFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout + ui.StartTimeout,
		ResetTimeout:    30 * time.Second,
		PostTestTimeout: 30 * time.Second,
		TearDownTimeout: 30 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     fixture.ChromeLoggedInMultiDisplay,
		Desc:     "Logged into a user session with multi dipslay",
		Contacts: []string{"brpol@chromium.org"},
		Parent:   VirtualMultiDisplay,
		Impl: chrome.NewLoggedInFixtureWithParentState(func(s *testing.FixtState) interface{} {
			return s.ParentValue()
		}, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.ExtraArgs("—drm-virtual-connector-is-external")}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	fixtureConfig := arc.DefaultBootedFixtureConfig()
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs("—drm-virtual-connector-is-external")}, nil
	}
	fixtureConfig.ParentStateProvider = func(s *testing.FixtState) interface{} {
		return s.ParentValue()
	}
	testing.AddFixture(&testing.Fixture{
		Name:   arc.ArcBootedMultiDisplay,
		Desc:   "ARC is booted and multi display is setup",
		Parent: VirtualMultiDisplay,
		Contacts: []string{
			"brpol@chromium.org",
			"arcvm-eng-team@google.com",
		},
		Impl:            arc.NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})
}

const (
	// defaultMaxOutputs is the default number of maximum outputs to configure in the virtual display controller.
	defaultMaxOutputs = 2
)

// HasVirtualDisplayController provides the DisplayController and information on max outputs available.
type HasVirtualDisplayController interface {
	// VirtualDisplayController returns the VirtualDisplayController from the fixture state.
	VirtualDisplayController() VirtualDisplayController
}

// multiDisplayFixture contains state and configuration related to virtual displays.
//
// You can configure this fixture using environment variables on your device in the form:
// TAST_MULTIDISPLAY_SNAKE_CASE_NAME,
// eg. TAST_MULTIDISPLAY_MAX_OUTPUTS
type multiDisplayFixture struct {
	// Max number of output displays.
	maxOutputs int

	// Interface to control displays (plug in, unplug, etc).
	multiDisplayController VirtualDisplayController
}

func (f *multiDisplayFixture) VirtualDisplayController() VirtualDisplayController {
	return f.multiDisplayController
}

// NewMultiDisplayFixture creates a default multi display fixture with extra config specified by fOpts.
// See multiDisplayFixture for how various parameters can be overridden in the environment.
func NewMultiDisplayFixture(fOpts chrome.OptionsCallback) testing.FixtureImpl {
	maxOutputsStr, exists := os.LookupEnv("TAST_MULTIDISPLAY_MAX_OUTPUTS")

	if !exists {
		return &multiDisplayFixture{
			maxOutputs: defaultMaxOutputs,
		}
	}

	maxOutputs, err := strconv.Atoi(maxOutputsStr)
	if err != nil {
		return &multiDisplayFixture{
			maxOutputs: defaultMaxOutputs,
		}
	}

	return &multiDisplayFixture{
		maxOutputs: maxOutputs,
	}
}

func (f *multiDisplayFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := killDrmProcessesAndServices(ctx); err != nil {
		s.Error("Could not kill DRM processes: ", err)
	}

	s.Log("Checking if VKMS is loaded")
	if err := ensureModuleLoadedOrNot("vkms", true); err == nil {
		s.Log("VKMS Detected, removing")
		// Unload VKMS if it is loaded.
		if err := testexec.CommandContext(ctx, "rmmod", "vkms").Run(); err != nil {
			s.Error("Could not unload module vkms: ", err)
		}
	}

	outputsArg := fmt.Sprintf("max_outputs=%d", f.maxOutputs)
	s.Log("Loading virtio-gpu-dummy")
	if err := testexec.CommandContext(ctx, "modprobe", "virtio-gpu-dummy", outputsArg).Run(); err != nil {
		s.Fatal("Could not run modprobe for module for virtio-gpu-dummy: ", err)
	}

	if err := resumeDrmServices(ctx); err != nil {
		s.Fatal("Could not resume drm services: ", err)
	}

	if multidisplayController, err := multidisplayController(f); err != nil {
		s.Fatal("Could not get multi display controller: ", err)
	} else {
		f.multiDisplayController = multidisplayController
	}

	return f
}

func multidisplayController(f *multiDisplayFixture) (*virtioGpuDummyMultiDisplayController, error) {
	if kernelVersion, _, err := sysutil.KernelVersionAndArch(); err != nil {
		return nil, errors.Wrap(err, "could not get kernel version and arch")
	} else if !kernelVersion.Is(5, 15) {
		return nil, errors.New("in order to use virtio-gpu-dummy multi display controller, the system must be a betty vm with kernel 5.15")
	}

	return &virtioGpuDummyMultiDisplayController{maxDisplays: f.maxOutputs}, nil
}

func (f *multiDisplayFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := killDrmProcessesAndServices(ctx); err != nil {
		s.Error("Could not kill DRM processes: ", err)
	}

	if err := testexec.CommandContext(ctx, "rmmod", "virtio-gpu-dummy").Run(); err != nil {
		s.Fatal("Could not unload module virtio-gpu-dummy: ", err)
	}

	if err := testexec.CommandContext(ctx, "modprobe", "vkms").Run(); err != nil {
		s.Fatal("Could not load module vkms: ", err)
	}

	if err := resumeDrmServices(ctx); err != nil {
		s.Fatal("Could not resume drm services: ", err)
	}
}

func (f *multiDisplayFixture) Reset(ctx context.Context) error {
	for i := 1; i < f.maxOutputs; i++ {
		if err := f.multiDisplayController.DisableDisplay(i); err != nil {
			return err
		}
	}

	if err := f.multiDisplayController.EnableDisplay(0); err != nil {
		return err
	}

	return nil
}

func (f *multiDisplayFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *multiDisplayFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
