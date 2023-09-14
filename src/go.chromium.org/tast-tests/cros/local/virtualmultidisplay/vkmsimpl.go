// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualmultidisplay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type vkmsMultiDisplayController struct {
	maxDisplays int
}

const (
	testDevicePath                = "/sys/kernel/config/vkms/test"
	vkmsDeviceEnablePath          = "/sys/kernel/config/vkms/test/enabled"
	vkmsConnectorEnablePath       = "/sys/kernel/config/vkms/test/connectors/connector%d"
	vkmsConnectedAttributePattern = "/sys/kernel/config/vkms/test/connectors/connector%d/connected"
	vkmsPlanePattern              = "/sys/kernel/config/vkms/test/planes/plane%d"
	vkmsCrtcPattern               = "/sys/kernel/config/vkms/test/crtcs/crtc%d"
	vkmsEncoderPattern            = "/sys/kernel/config/vkms/test/encoders/encoder%d"
	vkmsConnectorPattern          = "/sys/kernel/config/vkms/test/connectors/connector%d"
	connectorZeroEnablePath       = "/sys/kernel/config/vkms/test/connectors/connector0/connected"
)

// DisplayEnabled checks if a display is enabled by reading the vkms configfs info.
func (c *vkmsMultiDisplayController) DisplayEnabled(displayID int) (bool, error) {
	if displayID >= c.maxDisplays || displayID < 0 {
		return false, errors.Errorf("no such display %d, must be in the range [0,%d)", displayID, c.maxDisplays)
	}

	enabled, err := os.ReadFile(fmt.Sprintf(vkmsConnectedAttributePattern, displayID))
	if err != nil {
		return false, errors.Wrapf(err, "could not read display state for display id: %d", displayID)
	}

	return strings.TrimSpace(string(enabled)) == "1", nil
}

// EnableDisplay uses vkms configfs enabled attribute to enable display.
func (c *vkmsMultiDisplayController) EnableDisplay(displayID int) error {
	return c.writeToDisplayConfigFs(displayID, "1")
}

// DisableDisplay uses vkms configfs enabled attribute to disable display.
func (c *vkmsMultiDisplayController) DisableDisplay(displayID int) error {
	return c.writeToDisplayConfigFs(displayID, "0")
}

// DisplayCount returns number of max displays (disabled or not).
func (c *vkmsMultiDisplayController) DisplayCount() (int, error) {
	return c.maxDisplays, nil
}

// InternalDisplayID returns 0 for the internal display ID.
func (c *vkmsMultiDisplayController) InternalDisplayID() (int, error) {
	return 0, nil
}

// ExternalDisplayIDs Returns a list of all the displays 1..maxDisplays.
func (c *vkmsMultiDisplayController) ExternalDisplayIDs() ([]int, error) {
	list := make([]int, c.maxDisplays-1)
	for i := range list {
		list[i] = i + 1
	}
	return list, nil
}

func (c *vkmsMultiDisplayController) AdditionalFixtureSetup(ctx context.Context) error {
	testing.ContextLog(ctx, "Entering vkms controller setup")

	if err := killDrmProcessesAndServices(ctx); err != nil {
		return errors.Wrap(err, "could not kill DRM processes")
	}

	// Make sure the config directory exists and that configFS is mounted there
	if err := os.MkdirAll("/sys/kernel/config", os.ModePerm); err != nil {
		return errors.Wrap(err, "could not create configfs directory")
	}
	configfsRegexp := regexp.MustCompile("configfs /sys/kernel/config configfs .*rw.*")
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return errors.Wrap(err, "could not read mounts")
	}
	match := configfsRegexp.Find(mounts)

	if match == nil {
		// configfs not mounted, mount it.
		if err := testexec.CommandContext(ctx, "mount", "-t", "configfs", "none", "/sys/kernel/config").Run(); err != nil {
			return errors.Wrap(err, "could not mount configfs")
		}
	}

	// Load VKMS configured for testing setup.
	outputsArgs := []string{"vkms", "enable_overlay=1", "enable_writeback=1", "enable_cursor=1", "enable_default_device=0"}
	testing.ContextLog(ctx, "Reloading vkms")
	if err := testexec.CommandContext(ctx, "modprobe", outputsArgs...).Run(); err != nil {
		return errors.Wrap(err, "could not run modprobe for module for vkms")
	}

	if _, err := os.Stat("/sys/kernel/config/vkms"); err != nil {
		return errors.Wrap(err, "VKMS has not configfs entry, check kernel version")
	}

	if err := os.MkdirAll(testDevicePath, os.ModePerm); err != nil {
		return errors.Wrap(err, "could not create VKMS device")
	}

	for i := 0; i < c.maxDisplays; i++ {
		// Create plane and make it primary.
		plane := fmt.Sprintf(vkmsPlanePattern, i)
		if err := os.Mkdir(plane, os.ModePerm); err != nil {
			return errors.Wrap(err, "could not create planes dir")
		}

		if err := os.WriteFile(fmt.Sprintf("%s/type", plane), []byte("1"), 0600); err != nil {
			return errors.Wrap(err, "could not set plane type")
		}

		crtc := fmt.Sprintf(vkmsCrtcPattern, i)
		if err := os.Mkdir(crtc, os.ModePerm); err != nil {
			return errors.Wrap(err, "could not create crtc configfs directory")
		}
		if err := os.Symlink(crtc, fmt.Sprintf("%s/possible_crtcs/crtc%d", plane, i)); err != nil {
			return errors.Wrap(err, "could not link possile crtc")
		}

		encoder := fmt.Sprintf(vkmsEncoderPattern, i)
		if err := os.Mkdir(encoder, os.ModePerm); err != nil {
			return errors.Wrap(err, "could not create encoder configfs directory")
		}
		if err := os.Symlink(crtc, fmt.Sprintf("%s/possible_crtcs/crtc%d", encoder, i)); err != nil {
			return errors.Wrap(err, "could not link possile crtc")
		}

		connector := fmt.Sprintf(vkmsConnectorPattern, i)
		if err := os.Mkdir(connector, os.ModePerm); err != nil {
			return errors.Wrap(err, "could not create connector configfs directory")
		}
		if err := os.Symlink(encoder, fmt.Sprintf("%s/possible_encoders/encoder%d", connector, i)); err != nil {
			return errors.Wrap(err, "could not link possible encoder")
		}
	}

	// Create cursor plane
	const cursorPlane = "/sys/kernel/config/vkms/test/planes/cursor"
	if err := os.Mkdir(cursorPlane, os.ModePerm); err != nil {
		return errors.Wrap(err, "could not create cursor plane dir")
	}
	f, err := os.OpenFile(fmt.Sprintf("%s/type", cursorPlane), os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "could not open type configfs attribute")
	}
	defer f.Close()
	if _, err := f.WriteString("2"); err != nil { // 2 is the value for DRM_PLANE_CURSOR
		return errors.Wrap(err, "could not set plane type")
	}
	for i := 0; i < c.maxDisplays; i++ {
		crtc := fmt.Sprintf(vkmsCrtcPattern, i)
		os.Symlink(crtc, fmt.Sprintf("%s/possible_crtcs/crtc%d", cursorPlane, i))
	}

	// Print out a view of the VKMS devices in configfs
	treeCommand := testexec.CommandContext(ctx, "tree", "/sys/kernel/config/vkms")
	// var treeOutput bytes.Buffer
	// treeCommand.Stdout = &treeOutput
	// if err := treeCommand.Run(); err != nil {
	// 	return errors.Wrap(err, "could not get stdout from tree command")
	// }
	treeOutput, err := treeCommand.Output()
	if err != nil {
		return errors.Wrap(err, "could not execute tree command")
	}

	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get directory for saving files")
	}
	path := fmt.Sprintf("%s/vkms-configfs-tree.txt", dir)
	err = os.WriteFile(path, treeOutput, 0600)
	if err != nil {
		return errors.Wrap(err, "failed to create test vkms tree output file")
	}
	testing.ContextLogf(ctx, "VKMS configfs directory tree log file: %s", path)

	if err := os.WriteFile(connectorZeroEnablePath, []byte("1"), 0600); err != nil {
		return errors.Wrap(err, "could not enable connector 0")
	}

	if err := os.WriteFile(vkmsDeviceEnablePath, []byte("1"), 0600); err != nil {
		return errors.Wrap(err, "could not enable vkms device")
	}

	// Tests expect one display to be enabled at first.
	if err := c.EnableDisplay(0); err != nil {
		return errors.Wrap(err, "could not enable display 0 in vkms")
	}

	if err := resumeDrmServices(ctx); err != nil {
		return errors.Wrap(err, "could not resume drm services")
	}

	return nil
}

func (c *vkmsMultiDisplayController) writeToDisplayConfigFs(displayID int, value string) error {
	if err := c.ensureOutputCallValid(displayID); err != nil {
		return errors.Wrapf(err, "output call for output %d invalid", displayID)
	}

	connectorPath := fmt.Sprintf(vkmsConnectedAttributePattern, displayID)
	if err := os.WriteFile(connectorPath, []byte(value), 0600); err != nil {
		return errors.Wrap(err, "could not write connector file")
	}

	if readback, err := os.ReadFile(connectorPath); err != nil {
		return errors.Wrap(err, "could not read back connector file")
	} else if string(readback[0]) != value {
		return errors.Errorf("set display enabled for display %d to %s but read back %s", displayID, value, string(readback[:]))
	}

	return nil
}

func (c *vkmsMultiDisplayController) ensureOutputCallValid(displayID int) error {
	if err := ensureVkmsDriverLoaded(); err != nil {
		return err
	}

	if displayID > c.maxDisplays {
		return errors.Errorf("%d exceeds max display count: %d", displayID, c.maxDisplays)
	}

	return nil
}

func (c *vkmsMultiDisplayController) AdditionalFixtureTeardown(ctx context.Context) error {
	if err := killDrmProcessesAndServices(ctx); err != nil {
		return errors.Wrap(err, "could not kill DRM processes")
	}

	if _, err := os.Stat("/sys/kernel/config/vkms/test"); err == nil {
		// Remove all configfs created DRI components.
		connectors, err := filepath.Glob("/sys/kernel/config/vkms/test/connectors/*")
		if err != nil {
			return errors.Wrap(err, "could not find connectors config dirs")
		}
		for _, connector := range connectors {
			// Remove possible encoders
			pencs, err := filepath.Glob(fmt.Sprintf("%s/possible_encoders/*", connector))
			if err != nil {
				return errors.Errorf("no possible encoders found for connector %s", connector)
			}
			for _, penc := range pencs {
				if err := os.Remove(penc); err != nil {
					return errors.Wrap(err, "couldn't remove possible encoder symlink")
				}
			}

			// Remove the connector
			if err := unix.Rmdir(connector); err != nil {
				return errors.Wrap(err, "could not remove connector dir")
			}
		}

		encoders, err := filepath.Glob("/sys/kernel/config/vkms/test/encoders/*")
		if err != nil {
			return errors.Wrap(err, "could not find encoders config dirs")
		}
		for _, encoder := range encoders {
			pcrtcs, err := filepath.Glob(fmt.Sprintf("%s/possible_crtcs/*", encoder))
			if err != nil {
				return errors.Errorf("no possible encoders found for encoder %s", encoder)
			}
			for _, pcrtc := range pcrtcs {
				if err := os.Remove(pcrtc); err != nil {
					return errors.Wrap(err, "couldn't remove possible crtc symlink")
				}
			}

			if err := unix.Rmdir(encoder); err != nil {
				return errors.Wrap(err, "could not remove encoder dir")
			}
		}

		planes, err := filepath.Glob("/sys/kernel/config/vkms/test/planes/*")
		if err != nil {
			return errors.Wrap(err, "could not find planes config dirs")
		}
		for _, plane := range planes {
			pcrtcs, err := filepath.Glob(fmt.Sprintf("%s/possible_crtcs/*", plane))
			if err != nil {
				return errors.Errorf("no possible encoders found for encoder %s", plane)
			}
			for _, pcrtc := range pcrtcs {
				if err := os.Remove(pcrtc); err != nil {
					return errors.Wrap(err, "couldn't remove possible crtc symlink")
				}
			}

			if err := unix.Rmdir(plane); err != nil {
				return errors.Wrap(err, "could not remove plane dir")
			}
		}

		crtcs, err := filepath.Glob("/sys/kernel/config/vkms/test/crtcs/*")
		if err != nil {
			return errors.Wrap(err, "could not find crtcs config dirs")
		}
		for _, crtc := range crtcs {
			if err := unix.Rmdir(crtc); err != nil {
				return errors.Wrap(err, "could not remove crtc dir")
			}
		}

		// Remove vkms device.
		if err := unix.Rmdir("/sys/kernel/config/vkms/test"); err != nil {
			return errors.Wrap(err, "could not remove vkms config dir")
		}
	}

	// Re enable vkms with default parameters.
	if err := testexec.CommandContext(ctx, "modprobe", "vkms").Run(); err != nil {
		return errors.Wrap(err, "could not load module vkms")
	}

	if err := resumeDrmServices(ctx); err != nil {
		return errors.Wrap(err, "could not resume drm services")
	}

	return nil
}
