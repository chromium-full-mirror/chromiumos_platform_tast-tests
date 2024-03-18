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

// SysLogCategory is the category we use to check syslog.
type SysLogCategory string

const (
	// SysLogGpuHangs are hang signatures.
	SysLogGpuHangs SysLogCategory = "GPU hangs"
	// SysLogAmdGpuErrors are signatures for amdgpu errors.
	SysLogAmdGpuErrors = "AMDGPU error"
	// SysLogKernelSplats are signatures for kernel splats.
	SysLogKernelSplats = "Kernel splats"
	// SysLogMediatekIOMMUErrors are signatures for mediatek driver IOMMU errors.
	SysLogMediatekIOMMUErrors = "Mediatek IOMMU fault"
	// SysLogMediatekVideoErrors are signatures for mediatek video driver errors.
	SysLogMediatekVideoErrors = "Mediatek video error"
	// SysLogQualcommVideoErrors are signatures for qualcomm video driver errors.
	SysLogQualcommVideoErrors = "Qualcomm video error"
	// SysLogAll is a metacategory that can be used to ignore all categories. It should not be used in sysLogSignatureMap.
	SysLogAll SysLogCategory = "*"
)

type sysLogChecker struct {
	category SysLogCategory                               // category this checker belongs to.
	re       *regexp.Regexp                               // regular expression it tries to look for.
	handler  func(context.Context, *syslog.Reader) string // handler to find informative information from the reader.
}

var (
	sysLogSkipKernelTimeStamp = regexp.MustCompile(`\[\s*[0-9]+\.[0-9]+\]\s(.*)`) // Skip over the kernel timestamps to capture the rest of the line.

	// kernelSplatsHandler tries to analysis informative messages from a kernel splats.
	kernelSplatsHandler = func(ctx context.Context, reader *syslog.Reader) string {
		// Only checks for kernel Warning and Error.
		reader.UpdateFilter(syslog.Severities(syslog.Warning, syslog.Err), syslog.Program("kernel"))

		// x86: Skip over the code segment register to capture the text after the colon.
		// arm: look for pc and capture text after colon.
		x86RIPRegex := regexp.MustCompile(`RIP:\s+[0-9a-f]+:(.+)`)
		armPCRegex := regexp.MustCompile(`pc\s:\s(.+)`)

		// Normally we return the first match of a regexp. For splats this is not informative.
		// Read a few lines further for the RIP or pc entry.
		// This typically appears 5-15 lines after the splat starts.
		for i := 1; i < 20; i++ {
			e, err := reader.Read()
			if err != nil {
				continue
			}
			if match := x86RIPRegex.FindStringSubmatch(e.Content); match != nil {
				testing.ContextLog(ctx, "Found kernel splat detail with regex: ", x86RIPRegex.String())
				return match[1]
			}
			if match := armPCRegex.FindStringSubmatch(e.Content); match != nil {
				testing.ContextLog(ctx, "Found kernel splat detail with regex: ", armPCRegex.String())
				return match[1]
			}
		}
		testing.ContextLog(ctx, "Failed to read kernel splat detail")
		return "Kernel splat without RIP: detail"
	}

	allCheckers = []sysLogChecker{{
		// NB: http://shortn/_xXhPgTm1aW (Google-internal link) ***MUST*** be
		// kept updated to any changes made to the checkers below. Also note
		// any subsequent changes needed to said file if adding a category.
		// Checker to check GPU hangs.
		category: SysLogGpuHangs,
		re: regexp.MustCompile(strings.Join([]string{
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
			`No more free UVD handles!`,     // grunt b/266003084
			`VRAM is lost`,
			// mediatek
			`mtk-mdp.*: cmdq timeout`,
			`scp ipi .* ack time out !`,
			// mediatek/PowerVR
			`PVR_K:.*CheckForStalledCCB.*CCCB has not progressed`,   // b/303630620
			`Possible stalled client RGX contexts detected`,
			`PVR_K:.*Trying to identify stalled context...\(force\)`,
		}, "|")),
	}, {
		// Checker to check amd drivers error.
		category: SysLogAmdGpuErrors,
		re: regexp.MustCompile(strings.Join([]string{
			`Error scheduling IBs`,          // b/288942766
			`VM_L2_PROTECTION_FAULT_STATUS`, // b/271644551
		}, "|")),
	}, {
		// Checker to check the kernel splats.
		category: SysLogKernelSplats,
		re:       regexp.MustCompile(`------------\[ cut here \]------------`),
		handler:  kernelSplatsHandler,
	}, {
		// Checker to check mediatek IOMMU errors
		category: SysLogMediatekIOMMUErrors,
		re:       regexp.MustCompile(`mtk-iommu .*: fault`),
	}, {
		// Checker to check mediatek video driver errors.
		category: SysLogMediatekVideoErrors,
		re:       regexp.MustCompile(`\[MTK_(V4L2|VCODEC)\]\[ERROR\]`),
	}, {
		// Checker to check qualcomm video errors.
		category: SysLogQualcommVideoErrors,
		re: regexp.MustCompile(strings.Join([]string{
			`qcom-venus .*video-codec: SFR message from FW:`,
			`qcom-venus-decoder .*video-codec:video-decoder: dec: event session error`,
		}, "|")),
	}}
	// ignoreCategoriesMap maps testName to a list of SysLogCategory it would like to ignore when calling checkSysLog.
	ignoreCategoriesMap = map[string][]SysLogCategory{}
)

// CheckSysLog checks signatures from the reader. It returns error if failed to read the file or certain patterns are detected.
func CheckSysLog(ctx context.Context, testName string, reader *syslog.Reader) error {
	if reader == nil {
		return errors.New("nil syslog.Reader")
	}

	inList := func(a SysLogCategory, b []SysLogCategory) bool {
		for _, v := range b {
			if a == v {
				return true
			}
		}
		return false
	}

	ignoreCategories, ok := ignoreCategoriesMap[testName]
	if ok && inList(SysLogAll, ignoreCategories) {
		testing.ContextLogf(ctx, "Test %s requested to ignore all syslog checks", testName)
		return nil
	}

	var checkers []sysLogChecker
	var categories []SysLogCategory
	for _, checker := range allCheckers {
		if ok && inList(checker.category, ignoreCategories) {
			testing.ContextLogf(ctx, "Test %s requested to ignore check for `%v`", testName, checker.category)
			continue
		}
		checkers = append(checkers, checker)
		categories = append(categories, checker.category)
	}
	testing.ContextLogf(ctx, "Checking syslog with following categories: %q", categories)

	for {
		e, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "failed to read syslog")
		}
		if e.Program == "kernel" {
			// Remove kernel timestamp from beginning of line if found.
			if match := sysLogSkipKernelTimeStamp.FindStringSubmatch(e.Content); match != nil {
				e.Content = match[1]
			}
		}
		for _, checker := range checkers {
			if !checker.re.MatchString(e.Content) {
				continue
			}
			msg := e.Content
			if checker.handler != nil {
				msg = checker.handler(ctx, reader)
			}
			// This line must put after handler otherwise the regex may be caught.
			testing.ContextLog(ctx, "Found with following regex: ", checker.re.String())
			return errors.Errorf("%v: %s", checker.category, msg)
		}

		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context expired while parsing syslog")
		}
	}
	return nil
}

// DisableSysLogCheck skips checking the given categories after running test. If ignoreCategories is not set, skips all category checks.
// e.g. DisableSysLogCheck("graphics.IgtKms.kms_flip") to disable all syslog checks.
// e.g. DisableSysLogCheck("graphics.IgtKms.kms_flip", SysLogAll) to disable all syslog checks.
// e.g. DisableSysLogCheck("graphics.IgtKms.kms_flip", SysLogGpuHangs, SysLogKernelSplats) to disable checking GPU hangs and kernel splats.
func DisableSysLogCheck(testName string, ignoreCategories ...SysLogCategory) {
	// If empty, ignore all categories.
	if ignoreCategories == nil {
		ignoreCategories = []SysLogCategory{SysLogAll}
	}
	ignoreCategoriesMap[testName] = ignoreCategories
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
