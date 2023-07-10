// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	"go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	prepTimeout              = 2 * time.Minute
	verificationTimeout      = 2 * time.Minute
	rebootTimeout            = 1 * time.Minute
	cleanupTimeout           = 2 * time.Minute
	omahaInvalidationTimeout = prepTimeout + updateutil.UpdateTimeout + verificationTimeout + rebootTimeout + cleanupTimeout
)

// Firmware crossystem flags.
// References same flags used in firmware updater
// in chromiumos/src/platform/vboot_reference/updater.c:set_try_cookies.
const (
	// The next FW slot to try on next boot.
	fwTryNextFlag = "fw_try_next"
	// The currently booted FW slot.
	mainFWActFlag = "mainfw_act"
	// How many times to try the next FW slot.
	fwTryCountFlag = "fw_try_count"
	// Boot result of the currently booted FW slot.
	fwResultFlag = "fw_result"
)

const (
	// The crossystem `fwTryCountFlag` is set to `resetFWTryCount`
	// when a firmware update is invalidated, to indicate
	// that currently booted firmware successfully booted.
	resetFWTryCount = "0"
	// Arbitrary value to set the crossystem `fwTryCountFlag` to
	// to indicate the number of tries to try the next firmware update.
	// Used to fake a firmware update.
	updateFWTryCount = "5"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OmahaInvalidation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the update engine invalidates installed updates if Omaha issues an invalidation",
		Contacts: []string{
			"artyomchen@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		Fixture:      fixture.Autoupdate,
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "crossystem"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: omahaInvalidationTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.Served),
			pci.SearchFlag(&policy.RebootAfterUpdate{}, pci.Served),
			pci.SearchFlag(&policy.UptimeLimit{}, pci.Served),
		},
	})
}

func OmahaInvalidation(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(cleanupCtx)
	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer func(ctx context.Context) {
		if cl != nil {
			cl.Close(ctx)
		}
	}(cleanupCtx)

	// Enroll the DUT and set the policy.
	pb := policy.NewBlob()
	// Disable autoupdates and reboot after updates explicitly,
	// so that they do not conflict with the test.
	pb.AddPolicies([]policy.Policy{
		&policy.UptimeLimit{Val: 0},
		&policy.DeviceAutoUpdateDisabled{Val: false},
		&policy.RebootAfterUpdate{Val: false},
	})
	pJSON, err := json.Marshal(pb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
		SkipLogin:  true,
	}); err != nil {
		s.Fatal("Failed to enroll: ", err)
	}
	defer func(ctx context.Context) {
		if cl != nil {
			// Recreating since the client can have a stale RPC connection
			// after the reboot.
			policyClient := pspb.NewPolicyServiceClient(cl.Conn)
			policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	// Save current root and firmware partition information
	// to verify the invalidation later.
	rootdev, err := s.DUT().Conn().CommandContext(ctx, "rootdev", "-s").Output()
	if err != nil {
		s.Fatal("Failed to get current root partition: ", err)
	}

	mainFWAct, err := getCrossystemFlag(ctx, s.DUT(), mainFWActFlag)
	if err != nil {
		s.Fatal("Failed to get current firmware partition: ", err)
	}

	// Read the current builder path from the lsb file
	// for the in-place update.
	lsbContent := map[string]string{
		lsbrelease.BuilderPath: "",
	}
	if err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent); err != nil {
		s.Fatal("Failed to read from lsb file: ", err)
	}
	// Builder path is used in selecting the update image.
	builderPath := lsbContent[lsbrelease.BuilderPath]
	if builderPath == "" {
		s.Fatal("Builder path is missing")
	}

	// Update the DUT in-place.
	if err := updateutil.UpdateFromGS(ctx, s.DUT(), s.OutDir(), s.RPCHint(), builderPath); err != nil {
		s.Fatal("Failed to update image: ", err)
	}

	if err := fakeFirmwareUpdate(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to fake firmware update: ", err)
	}
	defer func(ctx context.Context) {
		if err := revertFakeFirmwareUpdate(ctx, s.DUT()); err != nil {
			s.Error("Failed to reset fake firmware update: ", err)
		}
	}(cleanupCtx)

	// Configure nebraska to invalidate the installed update.
	nebraskaClient := nebraska.NewServiceClient(cl.Conn)
	startResponse, err := nebraskaClient.Start(ctx, &nebraska.StartRequest{})
	if err != nil {
		s.Fatal("Failed to start Nebraska")
	}
	defer func(ctx context.Context) {
		if cl != nil {
			// Recreating since the client can have a stale RPC connection
			// after the reboot.
			nebraskaClient := nebraska.NewServiceClient(cl.Conn)
			nebraskaClient.Stop(ctx, &empty.Empty{})
		}
	}(cleanupCtx)

	if _, err := nebraskaClient.SetInvalidateLastUpdate(ctx, &nebraska.SetInvalidateLastUpdateRequest{InvalidateLastUpdate: true}); err != nil {
		s.Fatal("Failed to configure Nebraska for invalidation")
	}

	// Force an update check to invalidate the update.
	updateClient := autoupdate.NewUpdateServiceClient(cl.Conn)
	updateClient.CheckForUpdate(ctx, &autoupdate.UpdateRequest{
		OmahaUrl: fmt.Sprintf("http://127.0.0.1:%d/update", startResponse.Port),
	})

	// Verify that the update is invalidated.
	if err := verifyInvalidatedUpdate(ctx, s.DUT(), mainFWAct, string(rootdev)); err != nil {
		s.Fatal("Failed to verify the update invalidation: ", err)
	}

	// Reboot and try to ssh into a device
	// to verify that the device still boots properly.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	// Recreating because the RPC connection is stale after the reboot.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
}

// fakeFirmwareUpdate fakes a firmware update by setting appropriate crossystem
// flags indicating the update.
// Sets the next FW slot to try on next boot to alternative slot.
func fakeFirmwareUpdate(ctx context.Context, dut *dut.DUT) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := getCrossystemFlag(ctx, dut, mainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to get current firmware partition")
	}

	// Decide the alternative FW slot.
	var fwNext string
	if mainFWAct == "A" {
		fwNext = "B"
	} else if mainFWAct == "B" {
		fwNext = "A"
	} else {
		return errors.New("failed to find alternative FW slot")
	}

	// Set the fw_try_next crossystem flag to change the next FW slot to try.
	if err := setCrossystemFlag(ctx, dut, fwTryNextFlag, fwNext); err != nil {
		return errors.Wrap(err, "failed to change next FW slot")
	}

	// Set the fw_try_count crossystem flag to indicate number of tries to try the next FW slot.
	if err := setCrossystemFlag(ctx, dut, fwTryCountFlag, updateFWTryCount); err != nil {
		return errors.Wrap(err, "failed to change FW slot try count")
	}

	return nil
}

// revertFakeFirmwareUpdate reverts crossystem flags set by FakeFirmwareUpdate.
func revertFakeFirmwareUpdate(ctx context.Context, dut *dut.DUT) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := getCrossystemFlag(ctx, dut, mainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to get current firmware partition")
	}

	// Set the fw_try_next crossystem flag to the currently booted FW partition.
	if err := setCrossystemFlag(ctx, dut, fwTryNextFlag, mainFWAct); err != nil {
		return errors.Wrap(err, "failed to change next FW slot")
	}
	// Set the fw_try_count crossystem flag to zero.
	if err := setCrossystemFlag(ctx, dut, fwTryCountFlag, resetFWTryCount); err != nil {
		return errors.Wrap(err, "failed to change FW slot try count")
	}

	return nil
}

// verifyInvalidatedUpdate verifies that an update is invalidated by checking
// that the root and the firmware partitions are reset to previous states.
// Also verifies that the appropriate firmware flags are correctly reset.
func verifyInvalidatedUpdate(ctx context.Context, dut *dut.DUT, prevMainFwAct, prevRootSlot string) error {
	// Get the currently booted FW partition slot.
	mainFWAct, err := getCrossystemFlag(ctx, dut, mainFWActFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read main_fw_act from crossystem")
	}

	// Get a value of the fw_try_next flag.
	fwTryNext, err := getCrossystemFlag(ctx, dut, fwTryNextFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_try_next from crossystem")
	}

	// Get a value of the fw_try_count flag.
	fwTryCount, err := getCrossystemFlag(ctx, dut, fwTryCountFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_try_count from crossystem")
	}

	// Get a value of the fw_result flag.
	fwResult, err := getCrossystemFlag(ctx, dut, fwResultFlag)
	if err != nil {
		return errors.Wrap(err, "failed to read fw_result from crossystem")
	}

	// Verify that flags are properly reset.
	if mainFWAct != prevMainFwAct || mainFWAct != fwTryNext || fwTryCount != resetFWTryCount || fwResult != "success" {
		return errors.New("Firmware update is not invalidated")
	}

	// Verify that the root partition is properly reset.
	rootdev, err := dut.Conn().CommandContext(ctx, "rootdev", "-s").Output()
	if string(rootdev) != prevRootSlot {
		return errors.New("OS update is not invalidated")
	}
	return nil
}

func setCrossystemFlag(ctx context.Context, dut *dut.DUT, flag, value string) error {
	if err := dut.Conn().CommandContext(ctx, "crossystem", fmt.Sprintf("%s=%s", flag, value)).Run(); err != nil {
		return errors.Wrapf(err, "failed to set crossystem %s=%s", flag, value)
	}

	return nil
}

func getCrossystemFlag(ctx context.Context, dut *dut.DUT, flag string) (string, error) {
	value, err := dut.Conn().CommandContext(ctx, "crossystem", flag).Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to read crossystem %s", flag)
	}

	return string(value), nil
}
