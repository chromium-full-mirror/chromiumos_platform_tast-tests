// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type virtioGpuDummyMultiDisplayController struct {
	maxDisplays int
}

const (
	multiDisplayControlDirectory = "/sys/kernel/debug/virtgpu-dummy/outputs/"
	multiDisplayEnablePattern    = "%s%d/enabled"
)

func (c *virtioGpuDummyMultiDisplayController) DisplayEnabled(displayID int) (bool, error) {
	if displayID >= c.maxDisplays || displayID < 0 {
		return false, errors.Errorf("no such display %d, must be in the range [0,%d)", displayID, c.maxDisplays)
	}

	enabled, err := os.ReadFile(fmt.Sprintf(multiDisplayEnablePattern, multiDisplayControlDirectory, displayID))
	if err != nil {
		return false, errors.Wrapf(err, "could not read display state for display id: %d", displayID)
	}

	return strings.TrimSpace(string(enabled)) == "1", nil
}

func (c *virtioGpuDummyMultiDisplayController) EnableDisplay(displayID int) error {
	return c.writeToDisplayDebugFs(displayID, "1")
}

func (c *virtioGpuDummyMultiDisplayController) DisableDisplay(displayID int) error {
	return c.writeToDisplayDebugFs(displayID, "0")
}

func (c *virtioGpuDummyMultiDisplayController) writeToDisplayDebugFs(displayID int, value string) error {
	if err := c.ensureOutputCallValid(displayID); err != nil {
		return errors.Wrapf(err, "output call for output %d invalid", displayID)
	}

	displayPath := fmt.Sprintf(multiDisplayEnablePattern, multiDisplayControlDirectory, displayID)
	if err := os.WriteFile(displayPath, []byte(value), 0600); err != nil {
		return errors.Wrapf(err, "could not write %s to display debug path: %q", value, displayPath)
	}

	readback, err := os.ReadFile(displayPath)
	if err != nil {
		return errors.Wrapf(err, "could not open display debug path: %q for reading", displayPath)
	}

	if rb := string(readback[:]); rb != value {
		return errors.Errorf("set display enabled for display %d to %s but read back %s", displayID, value, rb)
	}

	return nil
}

func (c *virtioGpuDummyMultiDisplayController) ensureOutputCallValid(displayID int) error {
	if err := ensureVirtioGpuDummyModuleLoaded(); err != nil {
		return err
	}

	if displayID > c.maxDisplays {
		return errors.Errorf("%d exceeds max display count: %d", displayID, c.maxDisplays)
	}

	return nil
}

func (c *virtioGpuDummyMultiDisplayController) DisplayCount() (int, error) {
	return c.maxDisplays, nil
}

func (c *virtioGpuDummyMultiDisplayController) InternalDisplayID() (int, error) {
	return 0, nil
}

// ExternalDisplayIDs Returns a list of all the displays 1..maxDisplays.
func (c *virtioGpuDummyMultiDisplayController) ExternalDisplayIDs() ([]int, error) {
	list := make([]int, c.maxDisplays-1)
	for i := range list {
		list[i] = i + 1
	}
	return list, nil
}

func (c *virtioGpuDummyMultiDisplayController) AdditionalFixtureSetup(ctx context.Context) error {
	testing.ContextLog(ctx, "Entering virtio-gpu-dummy controller setup")

	if err := killDrmProcessesAndServices(ctx); err != nil {
		return errors.Wrap(err, "could not kill DRM processes")
	}

	testing.ContextLog(ctx, "Checking if VKMS is loaded")
	if err := ensureModuleLoadedOrNot("vkms", true); err == nil {
		testing.ContextLog(ctx, "VKMS Detected, removing")
		// If we can't unload vkms we are in trouble and should just bail out.
		// We're going to leave tast in a bad state otherwise.
		if err := testexec.CommandContext(ctx, "rmmod", "vkms").Run(); err != nil {
			return errors.Wrap(err, "could not unload module vkms")
		}
	}

	outputsArg := fmt.Sprintf("max_outputs=%d", c.maxDisplays)
	testing.ContextLog(ctx, "Loading virtio-gpu-dummy")
	if err := testexec.CommandContext(ctx, "modprobe", "virtio-gpu-dummy", outputsArg).Run(); err != nil {
		return errors.Wrap(err, "could not run modprobe for module for virtio-gpu-dummy")
	}

	if err := resumeDrmServices(ctx); err != nil {
		return errors.Wrap(err, "could not resume drm services")
	}

	return nil
}

func (c *virtioGpuDummyMultiDisplayController) AdditionalFixtureTeardown(ctx context.Context) error {
	if err := killDrmProcessesAndServices(ctx); err != nil {
		return errors.Wrap(err, "could not kill DRM processes")
	}

	if err := testexec.CommandContext(ctx, "rmmod", "virtio-gpu-dummy").Run(); err != nil {
		return errors.Wrap(err, "could not unload module virtio-gpu-dummy")
	}

	if err := testexec.CommandContext(ctx, "modprobe", "vkms").Run(); err != nil {
		return errors.Wrap(err, "could not load module vkms")
	}

	if err := resumeDrmServices(ctx); err != nil {
		return errors.Wrap(err, "could not resume drm services")
	}

	return nil
}
