// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	gsctoolFactoryConfigRE = regexp.MustCompile(`raw value: ([0-9a-fA-F]+)`)
	// BID type - 'FCFG'
	testFactoryConfigBIDType  = ti50.BIDField(0x46434647)
	testFactoryConfigBIDFlags = ti50.BIDField(0x3ffff)
	cr50FactoryConfigError    = uint32(1287)
	ti50FactoryConfigError    = uint32(1286)
)

const (
	testFactoryConfig  = 0x7000000000000011
	otherFactoryConfig = 0x11
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFactoryConfig,
		Desc:    "Verify GSC get and set the factory config",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
	})
}

// GSCFactoryConfig verifies GSC can get and set the factory config.
func GSCFactoryConfig(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	var sameFactoryConfigError uint32
	var factoryConfigError uint32
	switch f.TestbedProperties.TestbedType {
	case ti50.GscH1Shield:
		sameFactoryConfigError = 0
		factoryConfigError = cr50FactoryConfigError
	case ti50.GscDTShield, ti50.GscOpentitanCw310Fpga, ti50.GscOTShield, ti50.GscHostEmulation:
		factoryConfigError = ti50FactoryConfigError
		sameFactoryConfigError = factoryConfigError
	default:
		s.Fatal("Unsupported testbed type")
	}

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell, ti50.CcdSuzyQ)
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

// gSCToolFactoryConfig reads the factory config with gsctool over ccd.
func gSCToolFactoryConfig(ctx context.Context, b ti50.DevBoard) (uint64, error) {
	out, err := b.GSCToolCommand(ctx, "", "--factory_config")
	if err != nil {
		return 0, errors.Wrap(err, "failed to run GSCTool factory config")
	}
	match := gsctoolFactoryConfigRE.FindStringSubmatch(string(out))
	config, err := strconv.ParseUint(match[1], 16, 64)
	if err != nil {
		return 0, errors.Errorf("failed to parse GSCTool factory config value from %s: %s", match[1], err)
	}
	return config, nil
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

	if gSCToolConfig != tpmConfig || consoleConfig != tpmConfig {
		return 0, errors.Errorf("Configs do not match gsctool %x tpm %x console %x", gSCToolConfig, tpmConfig, consoleConfig)
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
