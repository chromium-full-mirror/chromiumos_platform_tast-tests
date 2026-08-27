// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCCCDFlashrom,
		Desc:    "Measure flashrom speed over CCD",
		Timeout: 30 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func checkContent(s *testing.State, got []byte, expect byte) {
	if !bytes.Equal(got, genContent(expect, len(got))) {
		s.Fatal("flash contents")
	}
}

func genContent(fill byte, length int) []byte {
	return bytes.Repeat([]byte{fill}, length)
}

// GSCCCDFlashrom measures flashrom speed over CCD.
func GSCCCDFlashrom(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	flashInfo := b.ProbeSPIFlashChip(ctx, i)
	flashSizeMb := float64(flashInfo.FlashSize) / 1024 / 1024
	s.Logf("Flash name: %s", flashInfo.Name)
	s.Logf("Flash size: %v MB", flashSizeMb)

	// Reset again after ProbeSPIFlashChip, enable CCD.
	b.ResetWithStraps(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	pv := perf.NewValues()
	logDuration := func(label string, duration uint32) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, float64(duration))
		perMb := float64(duration) / flashSizeMb
		pv.Set(perf.Metric{
			Name:      label + "_per_mb",
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, perMb)
		s.Logf("%s: %d ms", label, duration)
		s.Logf("%s_per_mb: %v ms", label, perMb)
	}

	// Erase any previous contents.
	s.Log("Erasing")
	_, err := b.CCDFlashromErase(ctx)
	th.MustSucceed(err, "erase")

	// Measure read time.
	s.Log("Reading")
	content, r, err := b.CCDFlashromRead(ctx)
	th.MustSucceed(err, "read")
	logDuration("read", r)
	// Check erase and read worked.
	checkContent(s, content, 0xff)
	if len(content) != flashInfo.FlashSize {
		s.Errorf("Read size %d != flash size", len(content))
	}

	// Measure write time from erased (all 0xff) to all zero.
	s.Log("Writing")
	w, err := b.CCDFlashromWrite(ctx, genContent(0, len(content)))
	th.MustSucceed(err, "write")
	// Flashrom does an internal read before writing.
	logDuration("write_with_read", w)

	// Measure erase time from all zero.
	s.Log("Erasing")
	e, err := b.CCDFlashromErase(ctx)
	th.MustSucceed(err, "erase")
	// To keep all results in one place print earlier collected values one
	// more time.
	s.Logf("read: %d ms", r)
	s.Logf("write_with_read: %d ms", w)
	// Flashrom does an internal read before erasing.
	logDuration("erase_with_read", e)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}

}
