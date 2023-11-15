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
	// Signatures in /var/log/messages to look for.
	sysLogSignatureMap = map[string]*regexp.Regexp{
		"GPU hangs": regexp.MustCompile(strings.Join([]string{
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
			`amdgpu_job_timedout.*ring \S+ timeout`, // DRM_ERROR
			`GPU reset`,
			`IB test failed on gfx`,         // kernel 5.x, b/307550145
			`failed testing IB on GFX ring`, // kernel 4.x, b/307550145
			`VRAM is lost`,
			// mediatek
			`mtk-mdp.*: cmdq timeout`,
			`scp ipi .* ack time out !`,
		}, "|")),
		"Problematic strings": regexp.MustCompile(strings.Join([]string{
			// amdgpu
			`Error scheduling IBs`,          // b/288942766
			`VM_L2_PROTECTION_FAULT_STATUS`, // b/271644551
			// mediatek
			`mtk-iommu .*: fault`,
			`\[MTK_(V4L2|VCODEC)\]\[ERROR\]`,
			// Qualcomm
			`qcom-venus .*video-codec: SFR message from FW:`,
			`qcom-venus-decoder .*video-codec:video-decoder: dec: event session error`,
			// Kernel splats
			`------------\[ cut here \]------------`,
		}, "|")),
	}
	disableSysLogCheck = false
)

// checkSysLog checks signatures from the reader. It returns error if failed to read the file or certain patterns are detected.
func checkSysLog(ctx context.Context, reader *syslog.Reader) error {
	if disableSysLogCheck {
		// Enable hangcheck for the next call.
		disableSysLogCheck = false
		testing.ContextLog(ctx, "DisableSysLogCheck detected. Skipping checking syslog")
		return nil
	}
	if reader == nil {
		return errors.New("nil syslog.Reader")
	}
	for {
		e, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "failed to read syslog")
		}

		for category, re := range sysLogSignatureMap {
			if re.MatchString(e.Content) {
				// Only output the full regex once we already found to prevent the reader reads the output itself.
				testing.ContextLog(ctx, "Found with following regex: ", re.String())
				return errors.Errorf("%v: %s", category, e.Content)
			}
		}

		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context expired while parsing syslog")
		}
	}
	return nil
}

// DisableSysLogCheck skips the next syslog check.
// Only DisableSysLogCheck is provided as checkSysLog are often called in fixture's preTest function which is out of the test control.
// And checkSysLog would re-enable the flag for the next test run.
func DisableSysLogCheck() {
	disableSysLogCheck = true
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
