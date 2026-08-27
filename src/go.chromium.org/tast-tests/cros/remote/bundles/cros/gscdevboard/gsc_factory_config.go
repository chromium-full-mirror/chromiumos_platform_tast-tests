// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// BID type - 'FCFG'
	testFactoryConfigBIDType  = ti50.BIDField(0x46434647)
	testFactoryConfigBIDFlags = ti50.BIDField(0x3ffff)
	cr50FactoryConfigError    = uint32(1287)
	ti50FactoryConfigError    = uint32(1286)
)

const (
	testFactoryConfig                  = uint64(0x7000000000000011)
	otherFactoryConfig                 = uint64(0x11)
	nvFactoryConfig     tpm2.TPMHandle = 0x013fff06
	nvFactoryConfigSize uint16         = 8
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFactoryConfig,
		Desc:    "Verify GSC get and set the factory config",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
	})
}

// GSCFactoryConfig verifies GSC can get and set the factory config.
func GSCFactoryConfig(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	var sameFactoryConfigError uint32
	var factoryConfigError uint32
	switch b.GscProperties().ChipType() {
	case ti50.GscH1:
		sameFactoryConfigError = 0
		factoryConfigError = cr50FactoryConfigError
	case ti50.GscDT, ti50.GscOT, ti50.GscHE:
		factoryConfigError = ti50FactoryConfigError
		sameFactoryConfigError = factoryConfigError
	default:
		s.Fatal("Unsupported testbed type")
	}

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")
	b.WaitUntilCCDConnected(ctx)

	config, err := getFactoryConfig(ctx, b, i, tpm)
	th.MustSucceed(err, "failed to get factory config")
	s.Logf("Factory Config: %x", config)

	if config != 0 {
		s.Fatal("Factory config not cleared")
	}
	// It should be possible to set the factory config until the BID type is set.
	err = tpm.TpmvSetBoardID(ti50.UnsetBID, testFactoryConfigBIDFlags)
	th.MustSucceed(err, "failed to set partial board id")
	s.Log("Set the board id flags")

	err = setFactoryConfig(ctx, tpm, testFactoryConfig, 0)
	th.MustSucceed(err, "failed to set factory config")

	config, err = getFactoryConfig(ctx, b, i, tpm)
	th.MustSucceed(err, "failed to get factory config")
	s.Logf("Factory Config: %x", config)

	if config != testFactoryConfig {
		s.Fatalf("Factory config mismatch: expected %x got %x", testFactoryConfig, config)
	}
	s.Log("Set the factory config to the test value")

	err = setFactoryConfig(ctx, tpm, otherFactoryConfig, factoryConfigError)
	th.MustSucceed(err, "unexpected error setting different factory config")
	s.Log("Got an error setting a different config")

	config, err = getFactoryConfig(ctx, b, i, tpm)
	th.MustSucceed(err, "failed to get factory config")
	s.Logf("Factory Config: %x", config)

	if config != testFactoryConfig {
		s.Fatalf("Factory config mismatch: expected %x got %x", testFactoryConfig, config)
	}

	err = setFactoryConfig(ctx, tpm, testFactoryConfig, sameFactoryConfigError)
	th.MustSucceed(err, "unexpected result setting the same factory config")
	s.Logf("Set the same config and got %d", sameFactoryConfigError)

	err = tpm.TpmvSetBoardID(testFactoryConfigBIDType, testFactoryConfigBIDFlags)
	th.MustSucceed(err, "failed to set board id type")
	s.Log("Set the board id type")

	err = setFactoryConfig(ctx, tpm, testFactoryConfig, factoryConfigError)
	th.MustSucceed(err, "failed to set same factory config after the board id type is set")
	s.Log("Succesfully blocked setting the same factory config after the board id is set")

	err = setFactoryConfig(ctx, tpm, otherFactoryConfig, factoryConfigError)
	th.MustSucceed(err, "failed to set different factory config after the board id type is set")
	s.Log("Succesfully blocked setting another factory config after the board id is set")

	config, err = getFactoryConfig(ctx, b, i, tpm)
	th.MustSucceed(err, "failed to get factory config")
	s.Logf("Factory Config: %x", config)

	if config != testFactoryConfig {
		s.Fatalf("Factory config mismatch: expected %x got %x", testFactoryConfig, config)
	}
}

// getFactoryConfig returns the current factory config. It reads the factory
// config from the tpm, ccd GSCTool, and the console command and verifies all
// of the values match.
func getFactoryConfig(ctx context.Context, b ti50.DevBoard, i *ti50.CrOSImage, tpm *utils.TpmHelper) (uint64, error) {
	tpmConfig, err := tpm.TpmvGetFactoryConfig()
	if err != nil {
		return 0, err
	}
	testing.ContextLogf(ctx, "TPM Factory Config: %x", tpmConfig)

	consoleConfig, err := i.FactoryConfig(ctx)
	if err != nil {
		return 0, err
	}
	testing.ContextLogf(ctx, "Console Config: %x", consoleConfig)

	gSCToolConfig, err := tpm.TpmvGetFactoryConfig()
	if err != nil {
		return 0, err
	}
	testing.ContextLogf(ctx, "GSCToolConfig Config: %x", gSCToolConfig)

	virtualConfig, err := virtualNvmemReadFactoryConfig(tpm)
	if err != nil {
		return 0, err
	}
	testing.ContextLogf(ctx, "virtual nvmem config: %x", virtualConfig)

	if gSCToolConfig != tpmConfig || consoleConfig != tpmConfig || virtualConfig != tpmConfig {
		return 0, errors.Errorf("Configs do not match gsctool %x tpm %x console %x virtual nvmem %x", gSCToolConfig, tpmConfig, consoleConfig, virtualConfig)
	}
	return tpmConfig, nil
}

// setFactoryConfig creates the factory config using tpm commands.
func setFactoryConfig(ctx context.Context, tpm *utils.TpmHelper, factoryConfig uint64, expectedStatus uint32) error {
	status, err := tpm.TpmvSetFactoryConfig(factoryConfig)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "Set Factory Config Status: %d", status)
	if status != expectedStatus {
		return errors.Errorf("status %d did not match expected status %d", status, expectedStatus)
	}
	return nil
}

// virtualNvmemReadFactoryConfig reads the factory config value
func virtualNvmemReadFactoryConfig(tpm *utils.TpmHelper) (uint64, error) {
	attr := tpm2.TPMSNVPublic{
		NVIndex: nvFactoryConfig,
		NameAlg: tpm2.TPMAlgSHA1,
		Attributes: tpm2.TPMANV{
			AuthRead: true,
			PPRead:   true,
		},
		DataSize: nvFactoryConfigSize,
	}

	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		return 0, err
	}
	nvHandle := tpm2.NamedHandle{
		Handle: nvFactoryConfig,
		Name:   *nvName,
	}

	read := tpm2.NVRead{
		AuthHandle: ti50.RootPlatformHandle,
		NVIndex:    nvHandle,
		Size:       nvFactoryConfigSize,
	}

	tpmFactoryConfig, err := read.Execute(tpm)
	if err != nil {
		return 0, err
	}
	factoryConfig := binary.LittleEndian.Uint64(tpmFactoryConfig.Data.Buffer)

	return factoryConfig, nil
}
