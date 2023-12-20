// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	rpb "go.chromium.org/tast-tests/cros/services/cros/rollback"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// DeviceInfo contains the information about the DUT before rollback.
type DeviceInfo struct {
	Board     string
	Version   string
	Milestone int
}

// dumpRollbackFiles retrieves Rollback relevant files and logs. Ensures
// they are available for debugging in case of failure.
func dumpRollbackFiles(ctx context.Context, dut *dut.DUT) error {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("output directory unavailable in context")
	}

	// Use a timestamp to avoid overwriting any existing directories.
	timeStr := time.Now().UTC().Format(time.RFC3339Nano)
	dir := filepath.Join(outDir, "rollback_relevant_data", timeStr)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory %q to store rollback data", dir)
	}
	testing.ContextLogf(ctx, "Saving relevant data for Rollback in %q", dir)

	// fileInfo contains the information of the data to be retrieved.
	type fileInfo struct {
		SaveName string
		Path     string
	}

	// List of files or directories relevant for Rollback.
	// Depending on the progression of the Rollback process, some of these files
	// may not exist and it will be expected to see a failure message.
	var rollbackFiles = []fileInfo{
		{SaveName: "messages", Path: "/var/log/messages"},
		{SaveName: "oobe_config_save", Path: "/var/lib/oobe_config_save/"},
		{SaveName: "oobe_config_restore", Path: "/var/lib/oobe_config_restore/"},
		{SaveName: "pstore", Path: "/sys/fs/pstore/"},
		{SaveName: "rollback_data", Path: "/mnt/stateful_partition/unencrypted/preserve/rollback_data"},
		{SaveName: "rollback_data_tpm", Path: "/mnt/stateful_partition/unencrypted/preserve/rollback_data_tpm"},
	}

	for _, fileInfo := range rollbackFiles {
		pathDst := filepath.Join(dir, fileInfo.SaveName)
		if err := linuxssh.GetFile(ctx, dut.Conn(), fileInfo.Path, pathDst, linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLogf(ctx, "Did not download %v: %v", fileInfo.Path, err)
		}
	}

	return nil
}

// SimulatePowerwash resets the TPM and system state.
func SimulatePowerwash(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	if err := dumpRollbackFiles(ctx, dut); err != nil {
		testing.ContextLog(ctx, "Failed to dump Rollback files before powerwash: ", err)
	}
	return policyutil.EnsureTPMAndSystemStateAreReset(ctx, dut, rpcHint)
}

// SimulatePowerwashAndReboot relies on the helper method to always reboot apart
// from doing the powerwash. If the internal functionality of
// EnsureTPMAndSystemStateAreResetRemote changes in the future and does not
// reboot the device, we will need to add reboot logic here for rollback to
// happen. See b/240541326.
func SimulatePowerwashAndReboot(ctx context.Context, dut *dut.DUT) error {
	if err := dumpRollbackFiles(ctx, dut); err != nil {
		testing.ContextLog(ctx, "Failed to dump Rollback files before powerwash and reboot: ", err)
	}
	return policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, dut)
}

// DUTInfo retrieves information about the device that is necessary for rollback
// from lsb-release: board, current version, and milestone.
func DUTInfo(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) (*DeviceInfo, error) {
	lsbContent := map[string]string{
		lsbrelease.Board:     "",
		lsbrelease.Version:   "",
		lsbrelease.Milestone: "",
	}
	err := updateutil.FillFromLSBRelease(ctx, dut, rpcHint, lsbContent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all the required information from lsb-release")
	}

	milestone, err := strconv.Atoi(lsbContent[lsbrelease.Milestone])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert milestone to integer %s", lsbContent[lsbrelease.Milestone])
	}

	deviceInfo := &DeviceInfo{
		Board:     lsbContent[lsbrelease.Board],
		Version:   lsbContent[lsbrelease.Version],
		Milestone: milestone,
	}

	testing.ContextLogf(ctx, "Device information: %s (board) %s (version) %d (milestone)", deviceInfo.Board, deviceInfo.Version, deviceInfo.Milestone)
	return deviceInfo, nil
}

// ConfigureNetworks sets up the networks supported by rollback.
func ConfigureNetworks(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) ([]*rpb.NetworkInformation, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	client, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer client.Close(cleanupCtx)

	// Configure networks to check preservation across rollback.
	rollbackService := rpb.NewEnterpriseRollbackServiceClient(client.Conn)
	defer rollbackService.CloseConnections(cleanupCtx, &empty.Empty{})

	if _, err := rollbackService.Connect(ctx, &rpb.SessionState{Ownership: rpb.Ownership_LOGGED_IN}); err != nil {
		return nil, errors.Wrap(err, "failed to connect the rollback service")
	}

	response, err := rollbackService.SetUpNetworks(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure networks on client")
	}
	return response.Networks, nil
}

// SaveRollbackData manually goes through the different steps that take place
// during rollback. Returns sensitive data to check it is not accidentally
// logged anywhere during rollback.
func SaveRollbackData(ctx context.Context, dut *dut.DUT) (string, error) {
	// The .save_rollback_data flag would have been left by the update_engine on
	// an end-to-end rollback. We don't use update_engine. Place it manually.
	if err := dut.Conn().CommandContext(ctx, "touch", "/mnt/stateful_partition/.save_rollback_data").Run(); err != nil {
		return "", errors.Wrap(err, "failed to write rollback data save file")
	}

	// oobe_config_save would be started on shutdown but we need to fake
	// powerwash and call it now.
	if err := dut.Conn().CommandContext(ctx, "start", "oobe_config_save").Run(); err != nil {
		return "", errors.Wrap(err, "failed to run oobe_config_save")
	}

	// The following two commands would be done by clobber_state during powerwash
	// but the test does not powerwash.
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", `cat /var/lib/oobe_config_save/data_for_pstore > /dev/pmsg0`).Run(); err != nil {
		return "", errors.Wrap(err, "failed to read rollback key")
	}
	// Adds a newline to pstore.
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", `echo "" >> /dev/pmsg0`).Run(); err != nil {
		return "", errors.Wrap(err, "failed to add newline after rollback key")
	}

	sensitive, err := dut.Conn().CommandContext(ctx, "cat", "/var/lib/oobe_config_save/data_for_pstore").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to read sensitive data for pstore")
	}
	return string(sensitive), nil
}

// ToPreviousVersion prepares the device for rolling back, downloads the
// update to the target image, simulates a powerwash, and reboots into the new
// image.
func ToPreviousVersion(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, outDir, board string, targetMilestone int, rollbackVersion string) error {
	// Stopping ui early to prevent accidental reboots in the middle of TPM clear.
	// If you stop the ui while an update is pending, the device restarts.
	if err := dut.Conn().CommandContext(ctx, "stop", "ui").Run(); err != nil {
		return errors.Wrap(err, "failed to stop ui")
	}

	builderPath := fmt.Sprintf("%s-release/R%d-%s", board, targetMilestone, rollbackVersion)
	if err := updateutil.UpdateFromGS(ctx, dut, outDir, rpcHint, builderPath); err != nil {
		return errors.Wrapf(err, "failed to update DUT to image for %q from GS", builderPath)
	}

	// Ineffective reset is expected because rollback initiates TPM ownership.
	testing.ContextLog(ctx, "Simulating powerwash and rebooting the DUT to fake rollback")
	if err := SimulatePowerwashAndReboot(ctx, dut); err != nil && !errors.Is(err, hwsec.ErrIneffectiveReset) {
		return errors.Wrap(err, "failed to simulate powerwash and reboot into rollback image")
	}

	return nil
}

// CheckImageVersion verifies that the image after rollback has changed to
// the target version.
func CheckImageVersion(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, rollbackVersion, originalVersion string) error {
	// Check the image version.
	version, err := updateutil.ImageVersion(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to read DUT image version after the update")
	}
	testing.ContextLogf(ctx, "The DUT image version after the update is %s", version)
	if version != rollbackVersion {
		if version == originalVersion {
			return errors.New("the image version did not change after the update")
		}
		return errors.Errorf("unexpected image version after the update; got %s, want %s", version, rollbackVersion)
	}
	return nil
}

// VerifyRollbackData ensures that sensitive data was not accidentally logged
// during rollback but certain data, like network configuration, has been
// preserved.
func VerifyRollbackData(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, networks []*rpb.NetworkInformation, sensitive string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure that the sensitive data was not logged.
	logsAndCrashes := []string{"/var/log", "/var/spool/crash", "/home/chronos/crash", "/mnt/stateful_partition/unencrypted/preserve/crash", "/run/crash_reporter/crash"}
	for _, folder := range logsAndCrashes {
		err := dut.Conn().CommandContext(ctx, "grep", "-rq", sensitive, folder).Run()
		if err == nil {
			return errors.Errorf("sensitive data found by grep in folder %q", folder)
		}
	}

	client, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer client.Close(cleanupCtx)

	rollbackService := rpb.NewEnterpriseRollbackServiceClient(client.Conn)

	if _, err := rollbackService.Connect(ctx, &rpb.SessionState{Ownership: rpb.Ownership_OOBE}); err != nil {
		return errors.Wrap(err, "failed to connect the rollback service")
	}
	defer rollbackService.CloseConnections(cleanupCtx, &empty.Empty{})

	response, err := rollbackService.VerifyNetworks(ctx, &rpb.VerifyNetworksRequest{Ownership: rpb.Ownership_OOBE, Networks: networks})
	if err != nil {
		return errors.Wrap(err, "failed to verify networks in OOBE")
	}
	if !response.Successful {
		return errors.Errorf("networks were not correctly preserved in OOBE: %s", response.VerificationDetails)
	}

	if _, err := rollbackService.Login(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to login after rollback")
	}

	response, err = rollbackService.VerifyNetworks(ctx, &rpb.VerifyNetworksRequest{Ownership: rpb.Ownership_LOGGED_IN, Networks: networks})
	if err != nil {
		return errors.Wrap(err, "failed to verify networks in logged in session")
	}
	if !response.Successful {
		return errors.Errorf("networks were not correctly preserved in logged in session: %s", response.VerificationDetails)
	}

	return nil
}

// ErrPowerwashFailed is returned if the fake powerwash failed.
var ErrPowerwashFailed = errors.New("failed to simulate powerwash")

// ClearRollbackAndSystemData stops every process related to rollback,
// removes its flags and data created, and simulates powerwash.
func ClearRollbackAndSystemData(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	dut.Conn().CommandContext(ctx, "stop", "oobe_config_save").Run()

	if err := SimulatePowerwash(ctx, dut, rpcHint); err != nil {
		testing.ContextLog(ctx, "Failed to powerwash, this may happen on previous images: ", err)
		return ErrPowerwashFailed
	}

	if err := dut.Conn().CommandContext(ctx, "rm", "-f", "/mnt/stateful_partition/.save_rollback_data").Run(); err != nil {
		return errors.Wrap(err, "failed to remove data save flag")
	}

	if err := dut.Conn().CommandContext(ctx, "rm", "-f", "/mnt/stateful_partition/unencrypted/preserve/rollback_data").Run(); err != nil {
		return errors.Wrap(err, "failed to remove OpenSSL encrypted rollback data")
	}

	if err := dut.Conn().CommandContext(ctx, "rm", "-f", "/mnt/stateful_partition/unencrypted/preserve/rollback_data_tpm").Run(); err != nil {
		return errors.Wrap(err, "failed to remove TPM encrypted rollback data")
	}

	return nil
}
