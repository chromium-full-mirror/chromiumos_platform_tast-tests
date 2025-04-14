// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DspCheckCbiMatch,
		Desc: "Verify CBI values match between EC and ISH",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:dsp", "dsp_ish"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.ChromeISH()),
	})
}

// getCbiValues gets the CBI value specified by the tag for both the EC and ISH.
// This function ignores errors, it's valid to compare CBI flags that don't exist
// since they should just not exist in both EC and ISH.
//
// Returns a tuple of the EC response followed by the ISH response
func getCbiValues(ctx context.Context, s *testing.State, tag string) (string, string) {
	ecOut, _ := firmware.NewECTool(
		s.DUT(), firmware.ECToolNameMain,
	).Command(
		ctx, "cbi", "get", tag,
	).CombinedOutput()
	ishOut, _ := firmware.NewECTool(
		s.DUT(), firmware.ECToolNameISH,
	).Command(
		ctx, "cbi", "get", tag,
	).CombinedOutput()
	return string(ecOut), string(ishOut)
}

func assertStringEqual(s *testing.State, str1, str2, errorMessage string) {
	if str1 == str2 {
		return
	}
	s.Error("Expected:")
	s.Error(str1)
	s.Error("Got:")
	s.Error(str2)
	s.Fatal(errorMessage)
}

func DspCheckCbiMatch(ctx context.Context, s *testing.State) {
	// Check FW_CONFIG
	ecOut, ishOut := getCbiValues(ctx, s, "6")
	assertStringEqual(s, ecOut, ishOut, "CBI 6 does not match")

	// Check SSFC
	ecOut, ishOut = getCbiValues(ctx, s, "8")
	assertStringEqual(s, ecOut, ishOut, "CBI 8 does not match")
}
