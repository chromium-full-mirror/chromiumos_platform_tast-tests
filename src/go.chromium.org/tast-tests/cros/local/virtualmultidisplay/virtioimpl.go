// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

import (
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast/core/errors"
)

type virtioGpuDummyMultiDisplayController struct {
	maxDisplays int
}

const (
	multiDisplayControlDirectory = "/sys/kernel/debug/virtgpu-dummy/outputs/"
	multiDisplayEnablePattern    = "%s%d/enabled"
)

func (c *virtioGpuDummyMultiDisplayController) DisplayEnabled(displayId int) (bool, error) {
	if displayId >= c.maxDisplays || displayId < 0 {
		return false, errors.Errorf("no such display %d, must be in the range [0,%d)", displayId, c.maxDisplays)
	}

	enabled, err := os.ReadFile(fmt.Sprintf(multiDisplayEnablePattern, multiDisplayControlDirectory, displayId))
	if err != nil {
		return false, errors.Wrapf(err, "could not read display state for display id: %d", displayId)
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
		return errors.Errorf("While setting display %d, wanted %s but read back %s", displayID, value, rb)
	}

	return nil
}

func (c *virtioGpuDummyMultiDisplayController) ensureOutputCallValid(displayId int) error {
	if err := ensureVirtioGpuDummyModuleLoaded(); err != nil {
		return err
	}

	if displayId > c.maxDisplays {
		return errors.Errorf("%d exceeds max display count: %d", displayId, c.maxDisplays)
	}

	return nil
}

func (c *virtioGpuDummyMultiDisplayController) DisplayCount() (int, error) {
	return c.maxDisplays, nil
}

func (c *virtioGpuDummyMultiDisplayController) InternalDisplayId() (int, error) {
	return 0, nil
}


// ExternalDisplayIds Returns a list of all the displays 1..maxDisplays.
func (c *virtioGpuDummyMultiDisplayController) ExternalDisplayIds() ([]int, error) {
	list := make([]int, c.maxDisplays - 1)
	for i := range list {
		list[i] = i + 1
	}
	return list, nil
}
