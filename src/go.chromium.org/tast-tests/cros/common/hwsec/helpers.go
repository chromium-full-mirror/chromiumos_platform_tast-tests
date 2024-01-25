// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

/*
This file implements miscellaneous and unsorted helpers.
*/

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	tmpb "go.chromium.org/chromiumos/system_api/tpm_manager_proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const fingerprintLoginFlagFile = "/var/lib/biod/force_fp_login"

// CmdHelper provides various helper functions that could be shared across all
// hwsec integration test base on CmdRunner.
type CmdHelper struct {
	cmdRunner        CmdRunner
	cryptohome       *CryptohomeClient
	devicemanagement *DeviceManagementClient
	tpmManager       *TPMManagerClient
	daemonController *DaemonController
}

// AttestationHelper provides various helper functions that could be shared across all
// hwsec integration test base on AttestationClient.
type AttestationHelper struct {
	attestation *AttestationClient
}

// CmdTPMClearHelper provides various helper functions that could be shared across all
// hwsec integration test base on CmdHelper & TPMClearer.
type CmdTPMClearHelper struct {
	CmdHelper
	tpmClearer TPMClearer
}

// FullHelper is the full version of all kinds of helper that could be shared across all
// hwsec integration test regardless of run-type, i.e., remote or local.
type FullHelper struct {
	CmdTPMClearHelper
	AttestationHelper
}

// CleanupFunc is the function signature of cleanup functions returned by this package. When
// a function's return values include a CleanupFunc, it means that it should be run in the
// cleanup step if the function returns successfully.
type CleanupFunc func(context.Context) error

// NewCmdHelper creates a new CmdHelper, with r responsible for CmdRunner.
func NewCmdHelper(r CmdRunner) *CmdHelper {
	return &CmdHelper{
		cmdRunner:        r,
		cryptohome:       NewCryptohomeClient(r),
		devicemanagement: NewDeviceManagementClient(r),
		tpmManager:       NewTPMManagerClient(r),
		daemonController: NewDaemonController(r),
	}
}

// NewAttestationHelper creates a new AttestationHelper, with ac responsible for AttestationDBus.
func NewAttestationHelper(ac AttestationDBus) *AttestationHelper {
	return &AttestationHelper{
		attestation: NewAttestationClient(ac),
	}
}

// NewCmdTPMClearHelper creates a new CmdTPMClearHelper, with ch responsible for CmdHelper and th responsible for TPMClearer.
func NewCmdTPMClearHelper(ch *CmdHelper, tc TPMClearer) *CmdTPMClearHelper {
	return &CmdTPMClearHelper{*ch, tc}
}

// NewFullHelper creates a new FullHelper, with ch responsible for CmdTPMClearHelper and ah responsible for AttestationHelper.
func NewFullHelper(ch *CmdTPMClearHelper, ah *AttestationHelper) *FullHelper {
	return &FullHelper{*ch, *ah}
}

// CmdRunner exposes the cmdRunner of helper
func (h *CmdHelper) CmdRunner() CmdRunner { return h.cmdRunner }

// CryptohomeClient exposes the cryptohome of helper
func (h *CmdHelper) CryptohomeClient() *CryptohomeClient { return h.cryptohome }

// DeviceManagementClient exposes the devicemanagement of helper
func (h *CmdHelper) DeviceManagementClient() *DeviceManagementClient { return h.devicemanagement }

// TPMManagerClient exposes the tpmManager of helper
func (h *CmdHelper) TPMManagerClient() *TPMManagerClient { return h.tpmManager }

// DaemonController exposes the daemonController of helper
func (h *CmdHelper) DaemonController() *DaemonController { return h.daemonController }

// AttestationClient exposes the attestation of helper
func (h *AttestationHelper) AttestationClient() *AttestationClient { return h.attestation }

// TPMClearer exposes the tpmClearer of helper
func (h *CmdTPMClearHelper) TPMClearer() TPMClearer { return h.tpmClearer }

// EnsureTPMIsReady ensures the TPM is ready when the function returns |nil|.
// Otherwise, returns any encountered error.
func (h *CmdHelper) EnsureTPMIsReady(ctx context.Context, timeout time.Duration) error {
	info, err := h.tpmManager.GetNonsensitiveStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ensure ownership due to error in |GetNonsensitiveStatus|")
	}
	if !info.IsOwned {
		if _, err := h.tpmManager.TakeOwnership(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure ownership due to error in |TakeOwnership|")
		}
	}
	return testing.Poll(ctx, func(context.Context) error {
		info, err := h.tpmManager.GetNonsensitiveStatus(ctx)
		if err != nil {
			return errors.New("error during checking TPM readiness")
		}
		if info.IsOwned {
			return nil
		}
		return errors.New("haven't confirmed to be owned")
	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: PollingInterval,
	})
}

// EnsureIsPreparedForEnrollment ensures the DUT is prepareed for enrollment
// when the function returns |nil|. Otherwise, returns any encountered error.
func (h *AttestationHelper) EnsureIsPreparedForEnrollment(ctx context.Context, timeout time.Duration) error {
	return testing.Poll(ctx, func(context.Context) error {
		// intentionally ignores error; retry the operation until timeout.
		isPrepared, err := h.attestation.IsPreparedForEnrollment(ctx)
		if err != nil {
			return err
		}
		if !isPrepared {
			return errors.New("not prepared yet")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: PollingInterval,
	})
}

// RemoveFile would delete the file
func (h *CmdHelper) RemoveFile(ctx context.Context, filename string) error {
	_, err := h.cmdRunner.Run(ctx, "rm", "-f", "--", filename)
	return err
}

// RemoveAll would delete all the files and dirs
func (h *CmdHelper) RemoveAll(ctx context.Context, filename string) error {
	_, err := h.cmdRunner.Run(ctx, "rm", "-rf", "--", filename)
	return err
}

// ReadFile would read data from the file
func (h *CmdHelper) ReadFile(ctx context.Context, filename string) ([]byte, error) {
	return h.cmdRunner.Run(ctx, "cat", "--", filename)
}

// WriteFile would write data into the file
func (h *CmdHelper) WriteFile(ctx context.Context, filename string, data []byte) error {
	// Because we may pass NULL('\0') byte in data, using base64 to encode the data could resolve the escape character issue.
	// Using "echo" or "printf" would require a complex escaping rule to let it work correctly.
	b64String := base64.StdEncoding.EncodeToString(data)
	echoStrCmd := fmt.Sprintf("echo %s", shutil.Escape(b64String))
	b64DecCmd := fmt.Sprintf("base64 -d > %s", shutil.Escape(filename))
	cmd := fmt.Sprintf("%s | %s", echoStrCmd, b64DecCmd)
	if _, err := h.cmdRunner.Run(ctx, "sh", "-c", cmd); err != nil {
		return errors.Wrap(err, "failed to echo string")
	}
	return nil
}

// GetTPMManagerLocalData would read the tpm_manager local_tpm_data.
// Note: Get the data without stopping tpm_managerd may result stale data.
func (h *CmdHelper) GetTPMManagerLocalData(ctx context.Context) ([]byte, error) {
	return h.ReadFile(ctx, "/var/lib/tpm_manager/local_tpm_data")
}

// SetTPMManagerLocalData would write the local_tpm_data.
// Because tpm_managerd may cache the local data in the memory, we would need to restart tpm_managerd after modifying the data.
func (h *CmdHelper) SetTPMManagerLocalData(ctx context.Context, data []byte) error {
	return h.WriteFile(ctx, "/var/lib/tpm_manager/local_tpm_data", data)
}

// DropResetLockPermissions drops the reset lock permissions and return a callback to restore the permissions.
func (h *CmdHelper) DropResetLockPermissions(ctx context.Context) (restoreFunc func(ctx context.Context) error, retErr error) {
	stopDaemon := func(daemon *DaemonInfo, retErr *error) {
		if err := h.daemonController.Stop(ctx, daemon); err != nil {
			*retErr = errors.Wrapf(err, "failed to stop %s", daemon.DaemonName)
		}
	}
	startDaemon := func(daemon *DaemonInfo, retErr *error) {
		if err := h.daemonController.Start(ctx, daemon); err != nil {
			if retErr == nil {
				*retErr = errors.Wrapf(err, "failed to start %s", daemon.DaemonName)
			} else {
				testing.ContextLogf(ctx, "Failed to start %s: %v", daemon.DaemonName, err)
			}
		}
	}

	// Stop Cryptohome because it contains TPM status cache.
	if stopDaemon(CryptohomeDaemon, &retErr); retErr != nil {
		return nil, retErr
	}
	// Restart it after finishing all operations.
	defer startDaemon(CryptohomeDaemon, &retErr)

	// Stop TPM Manager before modifying its local data.
	if stopDaemon(TPMManagerDaemon, &retErr); retErr != nil {
		return nil, retErr
	}
	// Restart it after finishing all operations.
	defer startDaemon(TPMManagerDaemon, &retErr)

	rawData, err := h.GetTPMManagerLocalData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get local TPM data")
	}

	var data tmpb.LocalData
	if err := proto.Unmarshal(rawData, &data); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal local TPM data")
	}

	// Drop the owner password, so tpm_manager couldn't use it to create owner delegate on TPM1.2 device.
	data.OwnerPassword = []byte{}
	// Drop the owner delegate, so tpm_manager couldn't use it to reset DA counter on TPM1.2 device.
	data.OwnerDelegate = &tmpb.AuthDelegate{}

	// Drop the lockout password, so tpm_manager couldn't use it to reset DA counter on TPM2.0 device.
	data.LockoutPassword = []byte{}

	newData, err := proto.Marshal(&data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal local TPM data")
	}

	// Write back the data into the local data of tpm_manager.
	if err := h.SetTPMManagerLocalData(ctx, newData); err != nil {
		return nil, errors.Wrap(err, "failed to set local TPM data")
	}

	return func(ctx context.Context) (retErr error) {
		// Stop Cryptohome because it contains TPM status cache.
		if stopDaemon(CryptohomeDaemon, &retErr); retErr != nil {
			return retErr
		}
		// Restart it after finishing all operations.
		defer startDaemon(CryptohomeDaemon, &retErr)

		// Stop TPM Manager before modifying its local data.
		if stopDaemon(TPMManagerDaemon, &retErr); retErr != nil {
			return retErr
		}
		// Restart it after finishing all operations.
		defer startDaemon(TPMManagerDaemon, &retErr)

		// Restore the local data.
		if err := h.SetTPMManagerLocalData(ctx, rawData); err != nil {
			return errors.Wrap(err, "failed to restore local TPM data")
		}
		return nil
	}, nil
}

// GetTPMVersion would rteurn the TPM version, for example: "1.2", "2.0"
func (h *CmdHelper) GetTPMVersion(ctx context.Context) (string, error) {
	out, err := h.cmdRunner.Run(ctx, "tpmc", "tpmver")
	// Trailing newline char is trimmed.
	return strings.TrimSpace(string(out)), err
}

// ErrIneffectiveReset is returned if the TPM is owned after reset attempt.
var ErrIneffectiveReset = errors.New("ineffective reset of TPM")

// ensureTPMIsReset ensures the TPM is reset when the function returns nil.
// Otherwise, returns any encountered error.
// Optionally removes files from the DUT to simulate a powerwash.
func (h *CmdTPMClearHelper) ensureTPMIsReset(ctx context.Context, removeFiles bool) error {
	if err := h.daemonController.WaitForAllDBusServices(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for hwsec D-Bus services to be ready")
	}

	ownershipData, err := h.cmdRunner.Run(ctx, "hwsec-ownership-id", "id")
	if err != nil {
		return errors.Wrap(err, "failed to get ownership ID")
	}
	ownershipID := strings.TrimSpace(string(ownershipData))

	if err := h.tpmClearer.PreClearTPM(ctx); err != nil {
		return errors.Wrap(err, "failed to pre clear TPM")
	}

	h.restartDaemonsAndInvoke(ctx, func(ctx context.Context) error {
		if err := h.tpmClearer.ClearTPM(ctx); err != nil {
			return errors.Wrap(err, "failed to clear TPM")
		}

		if removeFiles {
			if err := h.ensureSystemStateIsReset(ctx); err != nil {
				return errors.Wrap(err, "failed to reset system state files")
			}
		}

		if err := h.tpmClearer.PostClearTPM(ctx); err != nil {
			return errors.Wrap(err, "failed to post clear TPM")
		}
		return nil
	})

	if err != nil {
		if err := h.saveTPMClearLogs(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to save TPM clear logs: ", err)
		}
		return errors.Wrap(err, "failed to ensure TPM is reset")
	}

	testing.ContextLog(ctx, "Waiting for system to be ready after reset TPM")
	if err := h.daemonController.WaitForAllDBusServices(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for hwsec D-Bus services to be ready")
	}

	if _, err = h.cmdRunner.Run(ctx, "hwsec-ownership-id", "diff", "--id="+ownershipID); err != nil {
		if err := h.saveTPMClearLogs(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to save TPM clear logs: ", err)
		}
		// If the ownership ID is not changed, the reset was not successful
		return ErrIneffectiveReset
	}

	return nil
}

func (h *CmdTPMClearHelper) ensureSystemStateIsReset(ctx context.Context) error {
	args := append([]string{"-rf", "--"}, SystemStateFiles...)
	if out, err := h.cmdRunner.Run(ctx, "rm", args...); err != nil {
		return errors.Wrapf(err, "failed to remove files to clear ownership: %s", string(out))
	}

	command := "rm -rf " + strings.Join(SystemStateGlobs, " ")
	if out, err := h.cmdRunner.Run(ctx, "bash", "-c", command); err != nil {
		return errors.Wrapf(err, "failed to remove files to clear ownership: %s", string(out))
	}

	if out, err := h.cmdRunner.Run(ctx, "bash", "-c", "vgchange -ay; lvremove -ff /dev/*/cryptohome*"); err != nil {
		// Ignore errors on failure, it is possible that the device doesn't support LVM or doesn't have any dm-crypt user crpytohomes.
		testing.ContextLog(ctx, "Failed to remove user logical volumes (this might be expected if the device doesn't support LVM): ", err, string(out))
	}

	// Run tmpfiles to restore the removed folders and permissions.
	if out, err := h.cmdRunner.Run(ctx, "/usr/bin/systemd-tmpfiles", "--create", "--remove", "--boot", "--prefix", "/home", "--prefix", "/var/lib"); err != nil {
		testing.ContextLog(ctx, "Failed to run tmpfiles: ", err, string(out))
	}
	return nil
}

// saveTPMClearLogs saves the logs which are related to the TPM clear failure to outdir.
// For more information, please see b/172876417.
func (h *CmdTPMClearHelper) saveTPMClearLogs(ctx context.Context) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return errors.New("failed to get name of directory")
	}
	if _, err := os.Stat(dir); err != nil {
		return errors.Wrap(err, "output directory not found")
	}

	logDir := filepath.Join(dir, "tpm_clear_faillog")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create directory for tpm clear faillog")
	}

	saveLogFile := func(from, to string) error {
		rawOutput, err := h.ReadFile(ctx, from)
		if err != nil {
			return errors.Wrapf(err, "failed to get the %s", from)
		}

		path := filepath.Join(logDir, to)
		f, err := os.Create(path)
		if err != nil {
			return errors.Wrapf(err, "failed to create %s", to)
		}
		defer f.Close()

		if _, err := f.Write(rawOutput); err != nil {
			return errors.Wrapf(err, "failed to write data to %s", to)
		}
		return nil
	}

	if err := saveLogFile("/sys/firmware/log", "firmware_log"); err != nil {
		testing.ContextLog(ctx, "Failed to save firmware_log: ", err)
	}
	return nil
}

// EnsureTPMIsReset ensures the TPM is reset when the function returns nil.
// Otherwise, returns any encountered error.
func (h *CmdTPMClearHelper) EnsureTPMIsReset(ctx context.Context) error {
	return h.ensureTPMIsReset(ctx, false)
}

// EnsureTPMAndSystemStateAreReset ensures the TPM is reset (if the device has an enabled TPM)
// and simulates a powerwash by wiping system state files and restarting daemons.
func (h *CmdTPMClearHelper) EnsureTPMAndSystemStateAreReset(ctx context.Context) error {
	status, err := h.TPMManagerClient().GetNonsensitiveStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get TPM status")
	}
	// If TPM isn't enabled (e.g. device doesn't have allowed TPM), only clear system state files
	if !status.IsEnabled {
		return h.restartDaemonsAndInvoke(ctx, h.ensureSystemStateIsReset)
	}
	return h.ensureTPMIsReset(ctx, true)
}

func (h *CmdTPMClearHelper) restartDaemonsAndInvoke(ctx context.Context, f func(ctx context.Context) error) error {
	if err := h.daemonController.TryStop(ctx, UIDaemon); err != nil {
		// ui might not be running because there's no guarantee that it's running when we start the test.
		// If we actually failed to stop ui and something ends up being wrong, then we can use the logging
		// below to let whoever that's debugging this problem find out.
		testing.ContextLog(ctx, "Failed to stop ui, this is normal if ui was not running: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.daemonController.Ensure(ctx, UIDaemon); err != nil {
			testing.ContextLog(ctx, "Failed to ensure ui daemon: ", err)
		}
	}(ctx)

	if err := h.daemonController.TryStopDaemons(ctx, StatefulDaemons); err != nil {
		// Stateful daemons might not be running because there is no guarantee
		// that it is running when we start the test. If we actually failed to
		// stop them and something ends up being wrong, then we can use the
		// logging below to let whoever that's debugging this problem find out.
		testing.ContextLog(ctx, "Failed to stop Stateful daemons, this is normal if they were not running: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.daemonController.EnsureDaemons(ctx, StatefulDaemons); err != nil {
			testing.ContextLog(ctx, "Failed to ensure Stateful daemons: ", err)
		}
	}(ctx)

	if err := h.daemonController.TryStopDaemons(ctx, HighLevelTPMDaemons); err != nil {
		// High-level TPM daemons might not be running because there's no guarantee that it's running when we start the test.
		// If we actually failed to stop them and something ends up being wrong, then we can use the logging
		// below to let whoever that's debugging this problem find out.
		testing.ContextLog(ctx, "Failed to stop High-level TPM daemons, this is normal if they were not running: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.daemonController.EnsureDaemons(ctx, HighLevelTPMDaemons); err != nil {
			testing.ContextLog(ctx, "Failed to ensure High-level TPM daemons: ", err)
		}
	}(ctx)

	return f(ctx)
}

// EnableFingerprintLogin enables the fingerprint login service by creating a flag file
// that's checked by biod.
func (h *CmdTPMClearHelper) EnableFingerprintLogin(ctx context.Context) (CleanupFunc, error) {
	if _, err := h.cmdRunner.RunWithCombinedOutput(ctx, "mkdir", "-p", path.Dir(fingerprintLoginFlagFile)); err != nil {
		return nil, errors.Wrap(err, "failed to create the FingerprintLogin flag file directory")
	}
	if _, err := h.cmdRunner.RunWithCombinedOutput(ctx, "touch", fingerprintLoginFlagFile); err != nil {
		return nil, errors.Wrap(err, "failed to write the FingerprintLogin flag file")
	}
	cleanupFlag := func(ctx context.Context) error {
		if _, err := h.cmdRunner.Run(ctx, "rm", fingerprintLoginFlagFile); err != nil {
			return errors.Wrap(err, "failed to remove the FingerprintLogin flag file")
		}
		return nil
	}
	// The fingerprint login feature needs a biod restart to take effect.
	if err := h.daemonController.Restart(ctx, BiometricsDaemon); err != nil {
		cleanupFlag(ctx)
		return nil, errors.Wrap(err, "failed to restart biod")
	}
	return (func(ctx context.Context) error {
		if err := cleanupFlag(ctx); err != nil {
			return err
		}
		if err := h.daemonController.Restart(ctx, BiometricsDaemon); err != nil {
			return errors.Wrap(err, "failed to restart biod")
		}
		return nil
	}), nil
}

// GetAllVolatileFlags returns a map of the output from `tpmc getvf` cmd which returns the ST CLEAR flags.
func (h *CmdHelper) GetAllVolatileFlags(ctx context.Context) (map[string]string, error) {
	flags := make(map[string]string)

	out, err := h.cmdRunner.Run(ctx, "tpmc", "getvf")
	if err != nil {
		return flags, errors.Wrap(err, "failed to get st clear flags")
	}

	/*
		Example output for TPM 2.0:
			phEnable 0
			shEnable 1
			ehEnable 1
			phEnableNV 1
			orderly 0

		Example output for TPM 1.2:
			deactivated 0
			physicalPresence 0
			physicalPresenceLock 1
			bGlobalLock 1
	*/

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		split := strings.Fields(strings.TrimSpace(line))
		flags[strings.TrimSpace(split[0])] = strings.TrimSpace(split[len(split)-1])
	}

	return flags, nil

}

// GetAllPermanentFlags returns a map of the output from `tpmc getpf` cmd which returns the permanent flags.
func (h *CmdHelper) GetAllPermanentFlags(ctx context.Context) (map[string]string, error) {
	flags := make(map[string]string)

	out, err := h.cmdRunner.Run(ctx, "tpmc", "getpf")
	if err != nil {
		return flags, errors.Wrap(err, "failed to get permanent flags")
	}

	/*
		Example output for TPM 2.0:
			lockoutAuthSet: 1
			disableClear: 0
			inLockout: 0
			tpmGeneratedEPS: 1
			ownerAuthSet: 1
			endorsementAuthSet: 1

		Example output for TPM 1.2:
			disable 0
			ownership 1
			deactivated 0
			physicalPresenceHWEnable 0
			physicalPresenceCMDEnable 1
			physicalPresenceLifetimeLock 1
			nvLocked 1
	*/

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		split := strings.Fields(strings.TrimSpace(line))
		flags[strings.TrimSpace(split[0])] = strings.TrimSpace(split[len(split)-1])
	}

	return flags, nil

}

// GetSpacePermissions returns the output from `tpmc getp space` cmd for the given space.
func (h *CmdHelper) GetSpacePermissions(ctx context.Context, space string) (string, error) {
	out, err := h.cmdRunner.Run(ctx, "tpmc", "getp", space)
	if err != nil {
		return "", errors.Wrap(err, "failed to get space permission")
	}

	trimmedOut := strings.TrimSpace(string(out))
	rePerm := regexp.MustCompile(fmt.Sprintf(`space %s has permissions (0x[0-9A-Fa-f]+)`, space))
	match := rePerm.FindStringSubmatch(trimmedOut)

	if match == nil {
		return "", errors.Errorf("failed to parse space permission, got output: %s", trimmedOut)
	}

	return match[1], nil
}
