// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/graphics/

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/local/graphics"
)

func TestPmTestParams(t *testing.T) {
	type param struct {
		Name         string
		Attr         []string
		LoopCount    int
		PmMode       string
		SuspendMode  string
		Timeout      time.Duration
		Fixture      string
		HardwareDeps string
	}
	inList := func(str graphics.PmTestMode, list []graphics.PmTestMode) bool {
		for _, l := range list {
			if str == l {
				return true
			}
		}
		return false
	}

	var pmtestParams []param
	for _, b := range []string{"", "bringup"} {
		idx := 0
		for _, variant := range []string{"", "chrome", "webglaquarium"} {
			var s []string
			if b != "" {
				s = append(s, b)
			}
			if variant != "" {
				s = append(s, variant)
			}
			suffix := strings.Join(s, "_")
			for _, pm := range []graphics.PmTestMode{graphics.PmTestNone, graphics.PmTestFreezer, graphics.PmTestDevices, graphics.PmTestPlatform, graphics.PmTestProcessors, graphics.PmTestCore} {
				for _, suspendMode := range []string{"graphics.SuspendS0ix", "graphics.SuspendS3"} {
					// In S0, Processors and Core are not supported.
					if suspendMode == "graphics.SuspendS0ix" && inList(pm, []graphics.PmTestMode{graphics.PmTestProcessors, graphics.PmTestCore}) {
						continue
					}
					mode := "s0"
					if suspendMode == "graphics.SuspendS3" {
						mode = "s3"
					}
					name := fmt.Sprintf("%02d_%v_%v", idx, mode, pm)
					if suffix != "" {
						name += "_" + suffix
					}

					attr := []string{"graphics_perbuild"}
					if suffix == "bringup" {
						attr = []string{"graphics_manual", "graphics_bringup"}
					}

					loopCount := 2
					if suffix == "bringup" {
						loopCount = 100
					}

					timeout := 3 * time.Minute
					if suffix == "bringup" {
						timeout = 20 * time.Minute
					}

					hardwareDeps := "hwdep.SuspendToIdle()"
					if suspendMode == "graphics.SuspendS3" {
						hardwareDeps = "hwdep.SuspendToMem()"
					}

					fixture := "graphicsNoChrome"
					if suffix == "webglaquarium" {
						fixture = "chromeGraphicsWebContent.webglaquarium"
					} else if suffix == "chrome" {
						fixture = "chromeGraphics"
					}
					pmtestParams = append(pmtestParams, param{
						Name:         name,
						Attr:         attr,
						SuspendMode:  suspendMode,
						PmMode:       string(pm),
						LoopCount:    loopCount,
						Timeout:      timeout,
						Fixture:      fixture,
						HardwareDeps: hardwareDeps,
					})
					idx++
				}
			}
		}
	}
	code := genparams.Template(t, `{{ range . }}{
	Name: "{{ .Name }}",
	Val: pmTestParam{
		pmMode: {{ .PmMode | fmt }},
		suspendMode: {{ .SuspendMode }},
		count:  {{ .LoopCount | fmt }},
	},
	{{ if .HardwareDeps }} ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}), {{ end }}
	{{ if .Fixture }} Fixture: {{ .Fixture | fmt }}, {{ end }}
	{{ if .Attr }}ExtraAttr: {{ .Attr | fmt }}, {{ end }}
	Timeout: {{ .Timeout | fmt}},
  },
  {{ end }}`, pmtestParams)
	genparams.Ensure(t, "pmtest.go", code)
}
