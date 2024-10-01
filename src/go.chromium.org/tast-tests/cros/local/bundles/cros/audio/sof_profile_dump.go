// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"go.chromium.org/tast-tests/cros/local/audio/sof"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// DeviceIdentity represents a determistic key for SOF profile.
//
// Technically the SOF profile can be determined by SKU ID (in CBI), i.e. property
// "label-sku" in Swarming. Note that one model may possess multiple sku-ids. For
// readaability purposes both model and firmware IDs are attached along.
type deviceIdentity struct {
	// Model is the name derived from "cros_config / name"
	Model string `json:"model"`
	// Frid is the coreboot firmware ID derived from "cros_config /identity frid"
	Frid string `json:"frid"`
	// SkuID is the SKU ID derived from "cros_config /identity sku-id"
	SkuID int `json:"sku-id"`
}

type deviceProfile struct {
	Device deviceIdentity      `json:"device"`
	Fw     sof.ProfileArtifact `json:"fw"`
	Tplg   sof.ProfileArtifact `json:"tplg"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SofProfileDump,
		Desc:         "Dump SOF profile information from kernel debugfs",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "johnylin@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational",
		},
		HardwareDeps: hwdep.D(hwdep.SOFAudioDSP()),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func getDeviceIdentity(ctx context.Context) (devID deviceIdentity, err error) {
	devID.Model, err = crosconfig.Get(ctx, "/", "name")
	if err != nil {
		return devID, errors.Wrap(err, "failed to get model name")
	}

	// frid and sku-id are not required attributes for CBI. If absent, an empty
	// string will be returned instead.
	devID.Frid, err = crosconfig.Get(ctx, "/identity", "frid")
	if err != nil && !crosconfig.IsNotFound(err) {
		return devID, errors.Wrap(err, "failed to invoke `cros_config /identity frid`")
	}

	var skuIDStr string
	skuIDStr, err = crosconfig.Get(ctx, "/identity", "sku-id")
	if err != nil && !crosconfig.IsNotFound(err) {
		return devID, errors.Wrap(err, "failed to invoke `cros_config /identity sku-id`")
	}

	devID.SkuID = 0
	if skuIDStr != "" {
		devID.SkuID, err = strconv.Atoi(skuIDStr)
		if err != nil {
			return devID, errors.Wrap(err, "failed to convert sku-id to int")
		}
	}

	return devID, nil
}

func dumpProfile(ctx context.Context) ([]byte, error) {
	profile, err := sof.GetProfile(ctx)
	if err != nil {
		if !sof.IsProfileNoInquiry(err) {
			return nil, errors.Wrap(err, "failed to get SOF profile")
		}

		ver, _, kerr := sysutil.KernelVersionAndArch()
		if kerr != nil {
			return nil, errors.Wrap(kerr, "failed to get kernel version")
		}
		testing.ContextLog(ctx, "No inquiry into SOF profile on device kernel ", ver.String())

		if ver.IsOrLater(6, 1) {
			return nil, errors.Errorf("Profile inquiry failed on device kernel %s", ver.String())
		}

		// SOF profile is not liable to inquiry for ChromeOS kernel
		// version lower than 6.1. If that is the case on DUT, skip
		// dumping profile.
		return nil, nil
	}

	devID, err := getDeviceIdentity(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device identity")
	}

	profDump := deviceProfile{
		Device: devID,
		Fw:     profile.Firmware,
		Tplg:   profile.Topology,
	}

	byteDump, err := json.MarshalIndent(profDump, "", "    ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode JSON dump data")
	}

	return byteDump, nil
}

func dumpCstate(ctx context.Context) []byte {
	byteCstate, err := sof.GetCstateRawOutput(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Skipped collecting cstate due to error: ", err)
		return nil
	}

	return byteCstate
}

func SofProfileDump(ctx context.Context, s *testing.State) {

	profileDump, err := dumpProfile(ctx)
	if err != nil {
		s.Fatal("Failed to dump profile: ", err)
	}
	if profileDump != nil {
		s.Log("SOF profile to dump: ", string(profileDump))

		if err := os.WriteFile(filepath.Join(s.OutDir(), "sof_profile.json"), profileDump, 0644); err != nil {
			s.Error("Failed to write output file: ", err)
		}
	}

	if cstateDump := dumpCstate(ctx); cstateDump != nil {
		s.Log("SOF cstate to dump: ", string(cstateDump))

		if !json.Valid(cstateDump) {
			s.Log("Invalid cstate output as JSON")
		}
		if err := os.WriteFile(filepath.Join(s.OutDir(), "sof_comp_state.json"), cstateDump, 0644); err != nil {
			s.Error("Failed to write output file: ", err)
		}
	}
}
