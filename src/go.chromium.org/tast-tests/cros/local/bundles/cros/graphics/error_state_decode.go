// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const i915ErrorStateDecode = "/usr/bin/aubinator_error_decode"

type errorStateDecodingParams struct {
	encodedErrorStateFilename string
	expectedMatches           []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ErrorStateDecode,
		Desc: "Verify i915_error_state decoding works as expected",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"jchinlee@chromium.org",
			"ihf@chromium.org",
		},
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Attr:         []string{"group:graphics", "graphics_perbuild", "group:mainline", "group:hw_agnostic", "informational"},
		Fixture:      "graphicsNoChrome",
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		Params: []testing.Param{
			{
				Name: "cfm_m120",
				Val: errorStateDecodingParams{
					encodedErrorStateFilename: "dri_error_logs/i915_error_state_cfm_m120",
					expectedMatches: []string{
						"Detected GEN9 chipset",
						"Address 0x0000334223fb0000 GGTT",
						"HW Context (vcs0) at 0x00000000 fff8f000",
						"0x016b42e4:  0x13000002:  MI_FLUSH_DW",
						"0x016b42e4:  0x13000002 : Dword 0",
					},
				},
				ExtraData: []string{"dri_error_logs/i915_error_state_cfm_m120"},
			},
			{
				Name: "cfm_m120_full",
				Val: errorStateDecodingParams{
					encodedErrorStateFilename: "dri_error_logs/i915_error_state_cfm_m120",
					expectedMatches:           nil,
				},
				ExtraData: []string{"dri_error_logs/i915_error_state_cfm_m120", "dri_error_logs/i915_error_state_cfm_m120_decoded.txt"},
			},
		},
	})
}

// ErrorStateDecode verifies expected behavior for error state log decoding, in particular, of
// i915_error_state by aubinator_error_decode.
func ErrorStateDecode(ctx context.Context, s *testing.State) {
	// We disable the syslog checks as decoding a canned error state binary should not trigger any hangs etc.
	// But if we decided to extend the test in the future to induce a life hang and decode that then we for
	// sure want to ignore this synthetic problem.
	graphics.DisableSysLogCheck(s.TestName())

	testOpt := s.Param().(errorStateDecodingParams)

	filename := testOpt.encodedErrorStateFilename
	testing.ContextLogf(ctx, "Decoding %s with %s", i915ErrorStateDecode, filename)
	stdout, stderr, err := testexec.CommandContext(
		ctx, i915ErrorStateDecode, s.DataPath(filename),
	).SeparatedOutput(testexec.DumpLogOnError)

	if err != nil {
		output := append(stdout, stderr...)
		s.Fatalf("%s failed on %s: %s", i915ErrorStateDecode, filename, output)
	}

	if testOpt.expectedMatches == nil {
		expectedDecodedFilename := s.DataPath(filename + "_decoded.txt")
		if err := validateDecodingFull(ctx, s, expectedDecodedFilename, string(stdout)); err != nil {
			s.Error("Failed to match full decoded log: ", err)
		}
	} else if err := validateDecoding(ctx, string(stdout), testOpt); err != nil {
		s.Error("Decoding failed to validate: ", err)
	}
}

// validateDecodingFull verifies that the decoded error state byte-matches the expected file.
func validateDecodingFull(ctx context.Context, s *testing.State, expectedDecodedFilename, decodedLog string) error {
	decodedFile, err := os.Create(filepath.Join(s.OutDir(), "i915_error_state_decoded.txt"))
	if err != nil {
		return errors.Wrap(err, "failed to create file for decoded log")
	}

	if _, err := decodedFile.Write([]byte(decodedLog)); err != nil {
		return errors.Wrap(err, "failed to write decoded log to temp file")
	}
	decodedFile.Close()

	// We diff both files ignoring all whitespace using the -w option.
	stdout, stderr, err := testexec.CommandContext(
		ctx, "diff", "-w", expectedDecodedFilename, decodedFile.Name(),
	).SeparatedOutput(testexec.DumpLogOnError)

	// Nonempty diff is also error.
	if err != nil {
		return errors.Wrap(err, string(append(stdout, stderr...)))
	}
	// We delete the decoded file only if it is identical to the expected file, otherwise it will be uploaded for inspection.
	os.Remove(decodedFile.Name())
	return nil
}

// validateDecoding validates specific expected behavior from decoding, i.e. line matches.
func validateDecoding(ctx context.Context, decodedFile string, params errorStateDecodingParams) error {
	var failedMatches []string
	for _, ex := range params.expectedMatches {
		if strings.Contains(decodedFile, ex) {
			testing.ContextLog(ctx, "Found match: ", ex)
		} else {
			testing.ContextLog(ctx, "Missing match: ", ex)
			failedMatches = append(failedMatches, ex)
		}
	}

	if failedMatches != nil {
		return errors.Errorf("%d expected matches missing: %v", len(failedMatches), failedMatches)
	}

	return nil
}
