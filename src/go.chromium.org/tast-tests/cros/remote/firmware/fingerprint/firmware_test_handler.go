// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"context"
	"time"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/daemons"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast-tests/cros/remote/sysutil"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// FirmwareTest provides a common framework for fingerprint firmware tests.
type FirmwareTest struct {
	dut                      *rpcdut.RPCDUT
	servo                    *servo.Proxy
	fpBoard                  fp.BoardName
	firmwareFile             FirmwareFile
	daemonState              []daemons.DaemonState
	needsRebootAfterFlashing bool
	cleanupTime              time.Duration
	dutTempDir               string
}

// NewFirmwareTest creates and initializes a new fingerprint firmware test.
// enableHWWP indicates whether the test should enable hardware write protect.
// enableSWWP indicates whether the test should enable software write protect.
func NewFirmwareTest(ctx context.Context, dut *rpcdut.RPCDUT, servoSpec, outDir string, firmwareFile *FirmwareFile, enableHWWP, enableSWWP bool) (firmwareTest *FirmwareTest, initError error) {
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to servo")
	}

	// Reserve 5 seconds for cleanup when this function fails.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	t := &FirmwareTest{dut: dut, servo: pxy}
	// Close servo connection when this function is going to return an error.
	defer func(ctx context.Context) {
		if initError != nil {
			t.servo.Close(ctx)
		}
	}(cleanupCtx)

	t.fpBoard, err = Board(ctx, t.dut)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get fingerprint board")
	}

	t.firmwareFile = *firmwareFile
	if firmwareFile.KeyType == KeyTypeMp {
		if err := ValidateBuildFwFile(ctx, t.dut, t.fpBoard, firmwareFile.FilePath); err != nil {
			return nil, errors.Wrap(err, "failed to validate MP build firmware file")
		}
	}
	t.needsRebootAfterFlashing, err = NeedsRebootAfterFlashing(ctx, t.dut)
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine if reboot is needed")
	}

	// Disable rootfs verification. It's necessary in order to disable biod
	// upstart job if board needs reboot after flashing.
	rootfsIsWritable, err := sysutil.IsRootfsWritable(ctx, t.dut.RPC())
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if rootfs is writable")
	}
	if !rootfsIsWritable && t.needsRebootAfterFlashing {
		testing.ContextLog(ctx, "Making rootfs writable")
		// Since MakeRootfsWritable will reboot the device, we must call
		// RPCClose/RPCDial before/after calling MakeRootfsWritable.
		if err := t.dut.RPCClose(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to close rpc")
		}
		// Rootfs must be writable in order to disable the upstart job.
		if err := sysutil.MakeRootfsWritable(ctx, t.dut.DUT(), t.dut.RPCHint()); err != nil {
			return nil, errors.Wrap(err, "failed to make rootfs writable")
		}
		if err := t.dut.RPCDial(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to redial rpc")
		}
	} else if rootfsIsWritable && !t.needsRebootAfterFlashing {
		testing.ContextLog(ctx, "WARNING: The rootfs is writable")
	}

	// Get upstart service client instance.
	upstartService, err := t.UpstartService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get upstart client instance")
	}

	t.daemonState, err = daemons.StopDaemons(ctx, upstartService, []string{
		biodUpstartJobName,
	})
	// Start daemons when this function is going to return an error.
	defer func(ctx context.Context) {
		if initError != nil {
			testing.ContextLog(ctx, "NewFirmwareTest failed, restore daemon state")

			// Get upstart service client instance and restore daemons.
			upstartService, err := t.UpstartService(ctx)
			if err == nil {
				err = daemons.RestoreDaemons(ctx, upstartService, true, t.daemonState)
			}

			if err != nil {
				testing.ContextLog(ctx, "Failed to restart daemons: ", err)
			}
		}
	}(cleanupCtx)
	// Check if daemons were stopped correctly.
	if err != nil {
		return nil, err
	}

	t.cleanupTime = timeForCleanup

	if t.needsRebootAfterFlashing {
		// Disable biod upstart job so that it doesn't interfere with the test when
		// we reboot.
		testing.ContextLogf(ctx, "Disabling %s job", biodUpstartJobName)
		if _, err := upstartService.DisableJob(ctx, &platform.DisableJobRequest{JobName: biodUpstartJobName}); err != nil {
			return nil, errors.Wrap(err, "failed to disable biod upstart job")
		}
		// Enable biod service when this function is going to return an error.
		defer func(ctx context.Context) {
			if initError != nil {
				testing.ContextLog(ctx, "NewFirmwareTest failed, let's re-enable biod upstart job")

				// Get upstart service client instance and enable biod service.
				upstartService, err := t.UpstartService(ctx)
				if err == nil {
					_, err = upstartService.EnableJob(ctx, &platform.EnableJobRequest{JobName: biodUpstartJobName})
				}

				if err != nil {
					testing.ContextLog(ctx, "Failed to re-enable biod upstart job: ", err)
				}
			}
		}(cleanupCtx)

		// Account for the additional time that rebooting adds.
		t.cleanupTime += 3 * time.Minute
	}

	if t.needsRebootAfterFlashing || (t.firmwareFile.KeyType != KeyTypeMp) {
		// Disable FP updater so that it doesn't interfere with the test
		// when we reboot. Please note that we don't need to disable
		// rootfs verification before disabling fingerprint updater.
		if err := DisableFPUpdater(ctx, t.dut); err != nil {
			return nil, errors.Wrap(err, "failed to disable updater")
		}
		// Enable FP updater when this function is going to return an error.
		defer func(ctx context.Context) {
			if initError != nil {
				testing.ContextLog(ctx, "NewFirmwareTest failed, let's re-enable FP updater")
				if err := EnableFPUpdater(ctx, dut); err != nil {
					testing.ContextLog(ctx, "Failed to re-enable FP updater: ", err)
				}
			}
		}(cleanupCtx)
	}

	t.dutTempDir, err = t.DutfsClient().TempDir(ctx, "", dutTempPathPattern)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create remote working directory")
	}

	// Check FPMCU state and reflash if needed. Remove SWWP if needed.
	if err := InitializeKnownState(ctx, t.dut, outDir, pxy, t.fpBoard, t.firmwareFile, t.needsRebootAfterFlashing, !enableSWWP); err != nil {
		return nil, errors.Wrap(err, "initializing known state failed")
	}

	// Double check our work in the previous step.
	if err := CheckValidFlashState(ctx, t.dut, t.fpBoard, t.firmwareFile); err != nil {
		return nil, err
	}

	if err := InitializeHWAndSWWriteProtect(ctx, t.dut, pxy, t.fpBoard, enableHWWP, enableSWWP); err != nil {
		return nil, errors.Wrap(err, "initializing write protect failed")
	}

	return t, nil
}

// Close cleans up the fingerprint test and restore the FPMCU firmware to the
// original image and state.
func (t *FirmwareTest) Close(ctx context.Context) error {
	testing.ContextLog(ctx, "Tearing down")

	// Reserve 1 second for closing servo connection.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	// Always close servo connection no matter what happens.
	defer t.servo.Close(cleanupCtx)

	// The test can fail when DUT is disconnected (e.g. context timeout
	// while reconnecting to DUT). In this case we should attempt to connect
	// to the DUT. When connecting fails, we shouldn't proceed because
	// without healthy connection there is nothing we can do.
	if !t.dut.RPCConnected(ctx) {
		testing.ContextLog(ctx, "Reconnecting to the DUT")
		if err := t.dut.Connect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT")
		}
	}

	var firstErr error

	// Always flash MP firmware during clean up.
	firmwareFile, err := NewMPFirmwareFile(ctx, t.dut)
	if err != nil {
		firstErr = err
	}
	if err := ReimageFPMCU(ctx, t.dut, t.servo, firmwareFile.FilePath, t.needsRebootAfterFlashing); err != nil {
		firstErr = err
	}

	// ReimageFPMCU reboots the DUT at least once. Sometimes after a reboot,
	// the connection to the DUT is broken. In this case we should try to
	// reconnect to the DUT, because we need to enable daemons.
	if !t.dut.RPCConnected(ctx) {
		testing.ContextLog(ctx, "Lost connection to the DUT during ReimageFPMCU. Trying to reconnect")
		if err := t.dut.Connect(ctx); err != nil {
			// The story ends here. Remaining part of this function
			// depends on healthy connection to the DUT. If we are
			// not able to connect, we will return firstErr or
			// connecting error if firstErr is nil.
			if firstErr != nil {
				testing.ContextLog(ctx, "Failed to connect to DUT: ", err)
				return firstErr
			}
			return errors.Wrap(err, "failed to connect to DUT")
		}
	}

	// Get upstart service client instance.
	upstartService, err := t.UpstartService(ctx)
	if err != nil && firstErr == nil {
		firstErr = err
	}

	if t.needsRebootAfterFlashing {
		if upstartService != nil {
			// If biod upstart job disabled, re-enable it
			resp, err := upstartService.IsJobEnabled(ctx, &platform.IsJobEnabledRequest{JobName: biodUpstartJobName})
			if err == nil && !resp.Enabled {
				testing.ContextLogf(ctx, "Enabling %s job", biodUpstartJobName)
				if _, err := upstartService.EnableJob(ctx, &platform.EnableJobRequest{JobName: biodUpstartJobName}); err != nil && firstErr == nil {
					firstErr = err
				}
			} else if err != nil && firstErr == nil {
				firstErr = err
			}
		}

		// Delete temporary working directory and contents
		// If we rebooted, the directory may no longer exist.
		tempDirExists, err := t.DutfsClient().Exists(ctx, t.dutTempDir)
		if err == nil && tempDirExists {
			if err := t.DutfsClient().RemoveAll(ctx, t.dutTempDir); err != nil && firstErr == nil {
				firstErr = errors.Wrapf(err, "failed to remove temp directory: %q", t.dutTempDir)
			}
		} else if err != nil && firstErr == nil {
			firstErr = errors.Wrapf(err, "failed to check existence of temp directory: %q", t.dutTempDir)
		}
	}

	if t.needsRebootAfterFlashing || (t.firmwareFile.KeyType != KeyTypeMp) {
		// If FP updater disabled, re-enable it.
		fpUpdaterEnabled, err := IsFPUpdaterEnabled(ctx, t.dut)
		if err == nil && !fpUpdaterEnabled {
			if err := EnableFPUpdater(ctx, t.dut); err != nil && firstErr == nil {
				firstErr = err
			}
		} else if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if upstartService != nil {
		if err := daemons.RestoreDaemons(ctx, upstartService, t.needsRebootAfterFlashing, t.daemonState); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// DUT gets the RPCDUT.
func (t *FirmwareTest) DUT() *rpcdut.RPCDUT {
	return t.dut
}

// Servo gets the servo proxy.
func (t *FirmwareTest) Servo() *servo.Proxy {
	return t.servo
}

// RPCClient gets the RPC client.
func (t *FirmwareTest) RPCClient() *rpc.Client {
	return t.dut.RPC()
}

// UpstartService gets the upstart service client.
func (t *FirmwareTest) UpstartService(ctx context.Context) (platform.UpstartServiceClient, error) {
	if !t.dut.RPCConnected(ctx) {
		return nil, errors.New("RPC connection is not available")
	}
	return platform.NewUpstartServiceClient(t.RPCClient().Conn), nil
}

// FirmwareFile gets the firmware file.
func (t *FirmwareTest) FirmwareFile() FirmwareFile {
	return t.firmwareFile
}

// NeedsRebootAfterFlashing describes whether DUT needs to be rebooted after flashing.
func (t *FirmwareTest) NeedsRebootAfterFlashing() bool {
	return t.needsRebootAfterFlashing
}

// DutfsClient gets the dutfs client.
func (t *FirmwareTest) DutfsClient() *dutfs.Client {
	return dutfs.NewClient(t.RPCClient().Conn)
}

// CleanupTime gets the amount of time needed for cleanup.
func (t *FirmwareTest) CleanupTime() time.Duration {
	return t.cleanupTime
}

// DUTTempDir gets the temporary directory created on the DUT.
func (t *FirmwareTest) DUTTempDir() string {
	return t.dutTempDir
}

// FPBoard gets the fingerprint board name.
func (t *FirmwareTest) FPBoard() fp.BoardName {
	return t.fpBoard
}

// IsFPUpdaterEnabled returns true if the fingerprint updater is enabled.
func IsFPUpdaterEnabled(ctx context.Context, dut *rpcdut.RPCDUT) (bool, error) {
	if !dut.RPCConnected(ctx) {
		return false, errors.New("RPC connection is not available")
	}

	fs := dutfs.NewClient(dut.RPC().Conn)
	disabled, err := fs.Exists(ctx, disableFpUpdaterPath)
	return !disabled, err
}

// EnableFPUpdater enables the fingerprint updater if it is disabled.
func EnableFPUpdater(ctx context.Context, dut *rpcdut.RPCDUT) error {
	if !dut.RPCConnected(ctx) {
		return errors.New("RPC connection is not available")
	}

	fs := dutfs.NewClient(dut.RPC().Conn)
	testing.ContextLog(ctx, "Enabling the fingerprint updater")
	if err := fs.Remove(ctx, disableFpUpdaterPath); err != nil {
		return errors.Wrapf(err, "failed to remove %q", disableFpUpdaterPath)
	}
	// Sync filesystem to make sure that FP updater is enabled correctly.
	if err := dut.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to sync DUT")
	}
	return nil
}

// DisableFPUpdater disables the fingerprint updater if it is enabled.
func DisableFPUpdater(ctx context.Context, dut *rpcdut.RPCDUT) error {
	if !dut.RPCConnected(ctx) {
		return errors.New("RPC connection is not available")
	}

	fs := dutfs.NewClient(dut.RPC().Conn)
	testing.ContextLog(ctx, "Disabling the fingerprint updater")
	if err := fs.WriteFile(ctx, disableFpUpdaterPath, nil, 0); err != nil {
		return errors.Wrapf(err, "failed to create %q", disableFpUpdaterPath)
	}
	// Sync filesystem to make sure that FP updater is disabled correctly.
	if err := dut.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to sync DUT")
	}
	return nil
}
