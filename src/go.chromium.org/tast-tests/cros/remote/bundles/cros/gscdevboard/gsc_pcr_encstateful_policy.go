// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

type testGSCPCREncstatefulPolicy struct {
	extend    string
	digest    string
	writeable bool
}

const (
	// This is a unsupported PCR0 value. Protected spaces should not be
	// writeable with unknown PCR0 values.
	extendUnknown2 = "1000000000000000000000000000000000000000000000000000000000000000"
	digestUnknown2 = "a44a029e04493b8d2fe7893391c2b3ceefec1603c585aad6203f2d14e07bfead"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCPCREncstatefulPolicy,
		Desc:    "Test EncStateful can be updated in the right PCR0 states",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "normal_allows_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    ti50.ExtendNormalBoot,
				digest:    ti50.DigestNormalBoot,
				writeable: true,
			},
		}, {
			Name: "rec_blocks_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    ti50.ExtendRecBoot,
				digest:    ti50.DigestRecBoot,
				writeable: false,
			},
		}, {
			Name: "dev_allows_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    ti50.ExtendDevBoot,
				digest:    ti50.DigestDevBoot,
				writeable: true,
			},
		}, {
			Name: "rec_dev_blocks_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    ti50.ExtendRecDevBoot,
				digest:    ti50.DigestRecDevBoot,
				writeable: false,
			},
		}, {
			Name: "unknown_blocks_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    extendUnknown2,
				digest:    digestUnknown2,
				writeable: false,
			},
		}, {
			Name: "zeroed_pcr_blocks_update",
			Val: testGSCPCREncstatefulPolicy{
				extend:    "",
				digest:    ti50.ZeroPCR,
				writeable: false,
			},
		}},
	})
}

// makeEncStatefulFile create the 40 bytes encstateful file with the specified hash and correct crc8.
func makeEncStatefulFile(hash []byte) []byte {
	out := make([]byte, 40)
	out[0] = 0x10                // version 1.0
	out[1] = byte(len(out))      // size
	out[2] = 0                   // crc -- filled in later
	out[3] = 0x00                // flags
	out[4] = 0                   // version (1 of 4)
	out[5] = 0                   // version (2 of 4)
	out[6] = 0                   // version (3 of 4)
	out[7] = 0                   // version (4 of 4)
	copy(out[8:], hash)          // payload
	out[2] = utils.Crc8(out[3:]) // crc
	return out
}

// GSCPCREncstatefulPolicy verifies data can be written to the EncStateful nvmem space when PCR0 is in the correct state
func GSCPCREncstatefulPolicy(ctx context.Context, s *testing.State) {
	testParams := s.Param().(testGSCPCREncstatefulPolicy)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// Create random hash string
	hash := make([]byte, 32)
	_, err := rand.Read(hash)
	if err != nil {
		s.Fatal("Error getting random hash: ", err)
	}
	s.Log("Setting up EncStateful file with random hash: ", hash)
	file := makeEncStatefulFile(hash)

	attr := ti50.EncstatefulAttr()
	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		s.Fatal("Failed to get NV name: ", err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: attr.NVIndex,
		Name:   *nvName,
	}
	// Define space in NV storage and clean up afterwards
	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}

	s.Log("Restarting GSC with appropriate straps")

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)

	if testParams.extend != "" {
		tpm.PCRExtendCheckDigest(0, testParams.extend, testParams.digest)
	}

	ccdstate, err := i.Command(ctx, "ccdstate")
	th.MustSucceed(err, "failed to get ccdstate")
	s.Log("ccdstate:", ccdstate)
	if !strings.Contains(strings.ToLower(ccdstate), strings.ToLower(testParams.digest)) {
		s.Errorf("Digest not found in ccdstate output: %s", ccdstate)
	}

	_, err = def.Execute(tpm)
	if !testParams.writeable {
		if err == nil {
			s.Fatal("Defined EncStateful when it should be unwriteable")
		}
		s.Log("Blocked defining EncStateful after PCR0 extend")
		return
	}
	th.MustSucceed(err, "failed to define EncStateful after PCR0 is extended")
	defer tpm.NvUndefineSpace(attr)
	s.Log("Defined EncStateful after PCR0 extend")

	s.Log("Write EncStateful")
	// Write the data into the EncStateful space
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: file,
		},
		Offset: 0,
	}
	_, err = write.Execute(tpm)
	th.MustSucceed(err, "Failed to write EncStateful")
	s.Log("Wrote EncStateful after PCR0 extend")

	read := tpm2.NVRead{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Size:       40,
	}
	readData, err := read.Execute(tpm)
	if err != nil {
		s.Fatal("Unexpected NVRead response: ", err)
	}
	s.Logf("wrote: %+v", file)
	s.Logf("read: %+v", readData)
	if !bytes.Equal(readData.Data.Buffer, file) {
		s.Fatal("Did not read back original file contents")
	}
}
