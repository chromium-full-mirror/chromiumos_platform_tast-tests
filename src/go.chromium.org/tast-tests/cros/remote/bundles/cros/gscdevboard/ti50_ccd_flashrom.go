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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50CCDFlashrom,
		Desc:    "Measure flashrom speed over CCD",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"ecgh@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func checkContent(s *testing.State, got []byte, expect byte) {
	if !bytes.Equal(got, genContent(expect)) {
		s.Fatal("flash contents")
	}
}

func genContent(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, 32*1024*1024)
}

// Ti50CCDFlashrom measures flashrom speed over CCD.
func Ti50CCDFlashrom(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenNewCrOSImage(ctx, b, s)

	// Enable CCD.
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	pv := perf.NewValues()
	logDuration := func(label string, duration uint32) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, float64(duration))
		s.Logf("%s: %d ms", label, duration)
	}

	// Erase any previous contents.
	s.Log("Erasing")
	_, err := b.CCDFlashromErase(ctx)
	th.MustSucceed(err, "erase")

	// Measure read time.
	s.Log("Reading")
	content, d, err := b.CCDFlashromRead(ctx)
	th.MustSucceed(err, "read")
	logDuration("read", d)
	// Check erase and read worked.
	checkContent(s, content, 0xff)

	// Measure write time from erased (all 0xff) to all zero.
	s.Log("Writing")
	d, err = b.CCDFlashromWrite(ctx, genContent(0))
	th.MustSucceed(err, "write")
	// Flashrom does an internal read before writing.
	logDuration("write_with_read", d)

	// Measure erase time from all zero.
	s.Log("Erasing")
	d, err = b.CCDFlashromErase(ctx)
	th.MustSucceed(err, "erase")
	// Flashrom does an internal read before erasing.
	logDuration("erase_with_read", d)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}

}
