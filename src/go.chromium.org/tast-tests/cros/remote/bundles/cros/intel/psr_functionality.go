// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PsrFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Functional PSR testing",
		BugComponent: "b:1038263", // 152642 > 154116 > Intel > vPRO
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "moises.veleta@intel.com"},
		Timeout:      time.Minute,
		Attr:         []string{"group:experimental"},
		SoftwareDeps: []string{"chrome", "reboot", "intel_psr"},
	})
}

func PsrFunctionality(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	d, err := rpcdut.NewRPCDUT(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer func(ctx context.Context) {
		if err := d.Close(ctx); err != nil {
			s.Fatal("Failed to close RPCDUT: ", err)
		}
	}(ctx)

	psrBeforeReboot, err := runAndParseIntelPsrTool(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get PSR: ", err)
	}

	logState := psrBeforeReboot["Log State"]
	if logState != "STARTED" {
		s.Fatalf("Unexpected log state: %s, expected: STARTED", logState)
	}

	s.Log("Reboot the device")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	psrAfterReboot, err := runAndParseIntelPsrTool(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get PSR: ", err)
	}

	preWarmReset, err := strconv.Atoi(psrBeforeReboot["Cumulative number of warm resets"])
	if err != nil {
		s.Fatal("Failed to convert initial warm reset to int: ", err)
	}

	postWarmReset, err := strconv.Atoi(psrAfterReboot["Cumulative number of warm resets"])
	if err != nil {
		s.Fatal("Failed to convert increased warm reset to int: ", err)
	}

	if preWarmReset+1 != postWarmReset {
		s.Fatalf("Failed to increase warm_reset_counter, prewarm count: %d, post: %d", preWarmReset, postWarmReset)
	}
}

func runAndParseIntelPsrTool(ctx context.Context, dut *dut.DUT) (map[string]string, error) {
	out, err := dut.Conn().CommandContext(ctx, "intel-psrtool", "-d").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute 'intel-psrtool -d' command")
	}
	psrToolOutput := make(map[string]string)
	r := regexp.MustCompile(`\t(.*):(.*)`)
	for _, match := range r.FindAllStringSubmatch(string(out), -1) {
		k := strings.TrimSpace(match[1])
		v := strings.TrimSpace(match[2])
		psrToolOutput[k] = v
	}
	return psrToolOutput, nil
}
