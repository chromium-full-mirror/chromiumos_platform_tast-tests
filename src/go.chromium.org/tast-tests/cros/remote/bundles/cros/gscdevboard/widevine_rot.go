// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    WidevineRot,
		Desc:    "Verify reads of the NVMEM Widevine ROT index field components",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"vbendeb@google.com",
			"cros-hwsec@google.com",
		},
		BugComponent: "b:452450415",
		Attr: []string{
			"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly",
		},
		Fixture: fixture.GSCOpenCCD,
	})
}

func getRotField(s *testing.State, w ti50.WvRotReadResponse, index uint32) [32]byte {
	var field [32]byte

	if index > 2 {
		s.Fatal("Invalid Widevine ROT field index: ", index)
	}

	if len(w.Data) != 96 {
		s.Fatalf("Unexpected Widevine ROT size: got %d, want 96", len(w.Data))
	}

	copy(field[:], w.Data[index*32:(index+1)*32])
	return field
}

func getRotSeed(s *testing.State, w ti50.WvRotReadResponse) [32]byte {
	return getRotField(s, w, 0)
}

func getHdcpSeed(s *testing.State, w ti50.WvRotReadResponse) [32]byte {
	return getRotField(s, w, 1)
}

func getGscCounterSeed(s *testing.State, w ti50.WvRotReadResponse) [32]byte {
	return getRotField(s, w, 2)
}

type wvRotFields struct {
	RotSeed        [32]byte
	HdcpSeed       [32]byte
	GscCounterSeed [32]byte
}

func (f *wvRotFields) init(s *testing.State, w ti50.WvRotReadResponse) {
	f.RotSeed = getRotSeed(s, w)
	f.HdcpSeed = getHdcpSeed(s, w)
	f.GscCounterSeed = getGscCounterSeed(s, w)
}

func WidevineRot(ctx context.Context, s *testing.State) {
	// 1. Setup Board connection (SPI/I2C/GPIO)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)

	// Let's read the Widevine ROT space twice
	wvResp0, err0 := tpm.ReadWidevineRot(0x60, 0)
	if err0 != nil {
		s.Fatal("Failed to read Widevine ROT NVMEM: ", err0)
	}
	wvResp1, err1 := tpm.ReadWidevineRot(0x60, 0)
	if err1 != nil {
		s.Fatal("Failed to read Widevine ROT NVMEM for the second time: ", err1)
	}

	r := tpm.PCRExtend(0, "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd")
	if r != nil {
		s.Fatal("PCR Extend error:", r)
	}

	// Let's read the Widevine ROT again, ROT and HDCP seeds should be different now
	wvResp2, err2 := tpm.ReadWidevineRot(0x60, 0)
	if err2 != nil {
		s.Fatal("Failed to read Widevine ROT NVMEM after PCR extend: ", err2)
	}

	b.SimulateApS3(ctx, tpm)

	// Read Widevine ROT again, GSC counter seed should be different now
	wvResp3, err3 := tpm.ReadWidevineRot(0x60, 0)
	if err3 != nil {
		s.Fatal("Failed to read Widevine ROT NVMEM after PLT_RST_L toggle: ", err3)
	}

	// Finally reset the GSC and read Widevine ROT again.
	tpm = b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	wvResp4, err4 := tpm.ReadWidevineRot(0x60, 0)
	if err4 != nil {
		s.Fatal("Failed to read Widevine ROT NVMEM after GSC reset: ", err4)
	}

	// now let's analyze the results. The first two reads should be identical.
	if !bytes.Equal(wvResp0.Data, wvResp1.Data) {
		s.Fatal("Widevine ROT read mismatch between first two reads")
	}

	// Cache the results in an array
	res := [5]wvRotFields{}
	res[0].init(s, wvResp0) // Original read
	res[1].init(s, wvResp1) // Second read, should be identical
	res[2].init(s, wvResp2) // After PCR0 extend
	res[3].init(s, wvResp3) // After PLT_RST_L toggle
	res[4].init(s, wvResp4) // After GSC reset

	// Rot rest should have changed.
	if res[0].RotSeed == res[2].RotSeed {
		s.Fatalf("Widevine ROT seed did not change after PCR0 extend: 0x%x vs 0x%x", res[0].RotSeed, res[2].RotSeed)
	}

	// HDCP seed should change after PCR0 extend
	if res[2].HdcpSeed == res[0].HdcpSeed {
		s.Fatalf("Widevine HDCP seed did not change after PCR0 extend: 0x%x", res[0].HdcpSeed)
	}

	// GSC counter seed should not have changed.
	if res[0].GscCounterSeed != res[2].GscCounterSeed {
		s.Fatalf("Widevine GSC counter seed changed after PCR0 extend: 0x%x vs 0x%x", res[0].GscCounterSeed, res[2].GscCounterSeed)
	}

	// PLT_RST_L toggle should have affected only GSC counter seed
	if res[3].RotSeed != res[2].RotSeed {
		s.Fatalf("Widevine ROT seed changed after PLT_RST_L toggle: 0x%x vs 0x%x", res[2].RotSeed, res[3].RotSeed)
	}
	if res[3].HdcpSeed != res[2].HdcpSeed {
		s.Fatalf("Widevine HDCP seed changed after PLT_RST_L toggle: 0x%x vs 0x%x", res[2].HdcpSeed, res[3].HdcpSeed)
	}
	if res[3].GscCounterSeed == res[2].GscCounterSeed {
		s.Fatalf("Widevine GSC counter seed did not change after PLT_RST_L toggle: 0x%x", res[2].GscCounterSeed)
	}

	// After GSC reset all ROT and HDCP seeds should be the same as the original value, GSC counter seed should be different.
	if res[4].RotSeed != res[0].RotSeed {
		s.Fatalf("Widevine ROT seed after GSC reset mismatch: 0x%x vs 0x%x", res[0].RotSeed, res[4].RotSeed)
	}
	if res[4].HdcpSeed != res[0].HdcpSeed {
		s.Fatalf("Widevine HDCP seed after GSC reset mismatch: 0x%x vs 0x%x", res[0].HdcpSeed, res[4].HdcpSeed)
	}
	if res[4].GscCounterSeed == res[0].GscCounterSeed {
		s.Fatalf("Widevine GSC counter seed did not change after GSC reset: 0x%x", res[0].GscCounterSeed)
	}

	// All good so far. Let's try partialreads of the Widevine ROT space now.
	// Array of offset/size pairs to read different slices of the Widevine ROT space.
	ranges := [][]uint16{
		{1, 1},       // ROT seed, first 1 byte
		{0x13, 0x10}, // Span between ROT seed and HDCP seed
		{0x20, 0x20}, // HDCP seed exactly
		{0x27, 0x15}, // Span between HDCP seed and GSC counter seed
		{0x11, 0x45}, // Cross all fields
	}

	for _, r := range ranges {
		wvResp, err := tpm.ReadWidevineRot(r[1], r[0])
		if err != nil {
			s.Fatalf("Failed to read Widevine ROT NVMEM for the range of %v, %v", r, err)
		}

		if !bytes.Equal(wvResp.Data, wvResp4.Data[r[0]:r[0]+r[1]]) {
			s.Fatal("Widevine ROT partial read mismatch for the range of ", r)
		}
	}

}
