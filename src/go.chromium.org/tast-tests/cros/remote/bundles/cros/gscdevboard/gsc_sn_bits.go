// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"encoding/hex"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

const (
	testSNBitsBIDType = 0x5b5b5b5b
	// These flags should work with all test image flags (0x10, 0x10000, and 0x20000)
	testSNBitsBIDFlags = 0x37f7f
	testSNBits         = "010101020202040404080808"
	erasedSNBits       = "ffffffffffffffffffffffff"
)

// snBits is the TPM handle for the sn bits
const snBits tpm2.TPMHandle = 0x013fff01
const snBitsSize uint16 = 16

type testSNBitsConfig struct {
	SNBits         string
	BIDType        ti50.BIDField
	FactoryDisable bool
	CanSetSNBits   bool
	ExpectedSNBits string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCSNBits,
		Desc:    "Verify GSC can get/set sn bits",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Params: []testing.Param{{
			Name: "standard",
			Val: testSNBitsConfig{
				SNBits:         testSNBits,
				BIDType:        ti50.UnsetBID,
				FactoryDisable: false,
				CanSetSNBits:   true,
				ExpectedSNBits: testSNBits,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "long",
			Val: testSNBitsConfig{
				SNBits:         testSNBits + "ff",
				BIDType:        ti50.UnsetBID,
				FactoryDisable: false,
				CanSetSNBits:   false,
				ExpectedSNBits: erasedSNBits,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			Name: "short",
			Val: testSNBitsConfig{
				SNBits:         "010101020202",
				BIDType:        ti50.UnsetBID,
				FactoryDisable: false,
				CanSetSNBits:   false,
				ExpectedSNBits: erasedSNBits,
			},
			ExtraAttr: []string{"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			// Ti50 devices don't block setting SN bits based on the
			// board ID. SN bits should be settable when the board
			// ID type and flags are set.
			Name: "ti50_bid_type_is_set",
			Val: testSNBitsConfig{
				SNBits:         testSNBits,
				BIDType:        testSNBitsBIDType,
				FactoryDisable: false,
				CanSetSNBits:   true,
				ExpectedSNBits: testSNBits,
			},
			ExtraAttr: []string{"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			// Ti50 blocks setting SN bits after factory mode is
			// disabled. Verify Ti50 can't set SN bits after factory
			// mode is disabled.
			Name: "ti50_factory_disable",
			Val: testSNBitsConfig{
				SNBits:         testSNBits,
				BIDType:        ti50.UnsetBID,
				FactoryDisable: true,
				CanSetSNBits:   false,
				ExpectedSNBits: erasedSNBits,
			},
			ExtraAttr: []string{"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310"},
		}, {
			// Cr50 blocks setting SN bits after the board ID type
			// is set. Verify Cr50 can't set SN bits after setting
			// the board ID type.
			Name: "cr50_bid_type_is_set",
			Val: testSNBitsConfig{
				SNBits:         testSNBits,
				BIDType:        testSNBitsBIDType,
				FactoryDisable: false,
				CanSetSNBits:   false,
				ExpectedSNBits: erasedSNBits,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}, {
			// Cr50 blocks setting SN bits after the board ID type
			// is set. It doesn't care about factory mode. Verify
			// Cr50 can still set SN bits after factory mode is
			// disabled as long as the board ID type isn't set.
			Name: "cr50_factory_disable",
			Val: testSNBitsConfig{
				SNBits:         testSNBits,
				BIDType:        ti50.UnsetBID,
				FactoryDisable: true,
				CanSetSNBits:   true,
				ExpectedSNBits: testSNBits,
			},
			ExtraAttr: []string{"gsc_h1_shield"},
		}},
	})
}

// GSCSNBits verifies the BID flags can be set without setting the type.
func GSCSNBits(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	config := s.Param().(testSNBitsConfig)

	sn, err := hex.DecodeString(config.SNBits)
	th.MustSucceed(err, "failed to decode sn")
	expectedSN, err := hex.DecodeString(config.ExpectedSNBits)
	th.MustSucceed(err, "failed to decode expected SN")

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	tpmSNBits, err := tpmReadSNBits(b, i, tpm)
	th.MustSucceed(err, "failed to read serial from tpm")
	s.Log("tpm SN: ", tpmSNBits)

	bid, err := i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Log("Got board id")
	if !bid.IsErased {
		s.Fatal("Board ID is set")
	}
	s.Logf("BID: %+v", bid)

	err = tpm.TpmvSetBoardID(config.BIDType, testSNBitsBIDFlags)
	th.MustSucceed(err, "failed to set board id")

	bid, err = i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("BID: %+v", bid)
	if bid.Type != config.BIDType {
		s.Fatalf("Unexpected BID type: Expected %x Got %x", config.BIDType, bid.Type)
	}
	if bid.FlagsAreErased || bid.Flags != testSNBitsBIDFlags {
		s.Fatal("Flags were not set correctly")
	}

	if config.FactoryDisable {
		err = tpm.TpmvFactoryModeDisable()
		if err != nil {
			s.Fatalf("Failed to disable factory mode: %s", err)
		}
	}

	err = tpm.TpmvSetSNBits(sn)
	if config.CanSetSNBits {
		th.MustSucceed(err, "failed to set SN bits")
	} else if err == nil {
		s.Error("Set SN bits when there should've been an error")
	} else {
		s.Logf("Set SN bits correctly rejected: %s", err)
	}

	tpmSNBits, err = tpmReadSNBits(b, i, tpm)
	th.MustSucceed(err, "failed to read serial from tpm")
	s.Log("tpm SN: ", tpmSNBits)
	if !bytes.Equal(tpmSNBits, expectedSN) {
		s.Errorf("Failed to set SN bits: expected %v got %v", expectedSN, tpmSNBits)
	}

	if !config.CanSetSNBits {
		return
	}
	// Verify the command is rejected after SN bits is already set
	err = tpm.TpmvSetSNBits(sn)
	if err == nil {
		s.Fatal("Set SN bits after they were already set")
	}
	s.Logf("SN bits already set: %s", err)
}

// tpmReadSNBits reads the GSC serial number
func tpmReadSNBits(b utils.DevboardHelper, i *ti50.CrOSImage, tpm *utils.TpmHelper) ([]byte, error) {
	attr := tpm2.TPMSNVPublic{
		NVIndex: snBits,
		NameAlg: tpm2.TPMAlgSHA1,
		Attributes: tpm2.TPMANV{
			AuthRead: true,
			PPRead:   true,
		},
		DataSize: snBitsSize,
	}

	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		return nil, err
	}
	nvHandle := tpm2.NamedHandle{
		Handle: snBits,
		Name:   *nvName,
	}

	read := tpm2.NVRead{
		AuthHandle: ti50.RootPlatformHandle,
		NVIndex:    nvHandle,
		Size:       snBitsSize,
	}

	tpmSNbits, err := read.Execute(tpm)
	if err != nil {
		return nil, err
	}
	return tpmSNbits.Data.Buffer[4:16], nil
}
