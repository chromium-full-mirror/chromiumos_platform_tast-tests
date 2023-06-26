// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
		`Error scheduling IBs`, // b/288942766
		`GPU reset`,
		`VM_L2_PROTECTION_FAULT_STATUS`, // b/271644551
		`VRAM is lost`,

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

// SetHangCheckTimer sets the hangcheck timer to d to allow longer gpu runtime before hangcheck kicks in.
// Notice that it is expected to fail if running on older kernels or kernel which doesn't support hangcheck_period_ms.
// Notice that the unit of hangcheck timer is millisecond and the function would fail if d is smaller or equal to 1 millisecond.
func SetHangCheckTimer(ctx context.Context, d time.Duration) error {
	if d < 1*time.Millisecond {
		return errors.Errorf("invalid hangcheck timer parameter, %v, hangcheck timer must be greater or equal to 1 millisecond", d)
	}
	path, err := GetValidKernelDriverDebugFile(ctx, []string{"hangcheck_period_ms"})
	if err != nil {
		return errors.Wrap(err, "failed to get hangcheck file")
	}
	periodMs := int64(d / time.Millisecond)
	if err := ioutil.WriteFile(path, []byte(fmt.Sprintf("%d", periodMs)), 0600); err != nil {
		return errors.Wrapf(err, "failed to write %d to %s", periodMs, path)
	}
	testing.ContextLogf(ctx, "Wrote %d to %s", periodMs, path)
	return nil
}

// GetHangCheckTimer returns the current hangcheck duration timer.
// Notice that it is expected to fail if running on older kernels or kernels which doesn't support hangcheck_period_ms.
func GetHangCheckTimer(ctx context.Context) (time.Duration, error) {
	p, err := GetValidKernelDriverDebugFile(ctx, []string{"hangcheck_period_ms"})
	if err != nil {
		return -1, errors.Wrap(err, "failed to get hangcheck file")
	}

	b, err := ioutil.ReadFile(p)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to read %s", p)
	}
	s := strings.TrimSpace(string(b))
	if len(s) == 0 {
		return -1, errors.Errorf("%s is empty", p)
	}
	d, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return -1, errors.Wrapf(err, "malformed content in %s: %s", p, s)
	}
	return time.Duration(d) * time.Millisecond, nil
}
