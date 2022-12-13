// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"io"
	"regexp"
	"strings"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/syslog"
	"chromiumos/tast/testing"
)

var (
	hangSignatures = []string{
		// i915
		`drm:i915_hangcheck_elapsed`,
		`drm:i915_hangcheck_hung`,
		`GPU HANG: ecode `,
		`drm/i915: Resetting chip after gpu hang`,
		`GPU HANG:.+\b[H|h]ang on (rcs0|vcs0|vecs0)`,
		`Hangcheck timer elapsed...`,

		// msm, freedreno
		`hangcheck recover!`,

		// amdgpu
		`amdgpu: GPU reset begin!`,

		// mediatek
		`mtk-mdp.*: cmdq timeout`,
		`scp ipi .* ack time out !`,
		`mtk-iommu .*: fault`,

		// Qualcomm
		`qcom-venus .*video-codec: SFR message from FW:`,
	}
	disableHangCheck = false
)

// checkHangs checks gpu hangs from the reader. It returns error if failed to read the file or gpu hang patterns are detected.
func checkHangs(ctx context.Context, reader *syslog.Reader) error {
	if disableHangCheck {
		// Enable hangcheck for the next call.
		disableHangCheck = false
		testing.ContextLog(ctx, "DisableHangCheck detected. Skipping checking GPU hangs")
		return nil
	}

	if reader == nil {
		return errors.New("nil syslog.Reader")
	}

	// Join regexp to save time.
	re := regexp.MustCompile(strings.Join(hangSignatures, "|"))
	for {
		e, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "failed to read syslog")
		}
		matches := re.FindAllStringSubmatch(e.Line, -1)
		if len(matches) > 0 {
			return errors.Errorf("GPU hang: %s", e.Content)
		}
	}
	return nil
}

// DisableHangCheck skips the next GPU hang check.
// Only DisableHangCheck is provided as checkHangs are often called in fixture's preTest function which is out of the test control.
// And checkHangs would re-enable the flag for the next test run.
func DisableHangCheck() {
	disableHangCheck = true
}
