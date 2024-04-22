// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SocProperties,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks SOC model-related properties (ro.soc.*)",
		Contacts:     []string{"arcvm-eng@google.com", "matvore@chromium.org", "niwa@chromium.org"},
		BugComponent: "b:883059",

		// Exclude boards not planning to support ARCVM. They are
		// out-of-scope. (see http://go/arcvm-migration)
		HardwareDeps: hwdep.D(hwdep.SkipOnModel(
			"banon",
			"bob",
			"dru",
			"druwl",
			"dumo",
			"elm", // AUE on Container-R
			"edgar",
			"hana", // AUE on Container-R
			"kevin",
			"ultima",
		)),

		SoftwareDeps: []string{"arc", "chrome"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,

		Attr: []string{"group:mainline"},

		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"no_qemu"},
		},
			{
				// VMs may not have valid ro.soc.* values. It is often unfeasible to find the exact
				// CPU model as /proc/cpuinfo may have generic information. So we only populate
				// values for them as best effort.
				Name:              "betty",
				ExtraSoftwareDeps: []string{"qemu"},
				ExtraAttr:         []string{"informational"},
			}},
	})
}

func SocProperties(ctx context.Context, s *testing.State) {
	const (
		propertyManufacturer = "ro.soc.manufacturer"
		propertyModel        = "ro.soc.model"
	)

	a := s.FixtValue().(*arc.PreData).ARC

	// TODO(b/225373614): De-duplicate with the identical anonymous function
	// in BuildProperties, once these two tests are merged. It is tricky to
	// do now because it captures three local vars and args.
	getProperty := func(propertyName string) string {
		var value string
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			out, err := a.Command(ctx, "getprop", propertyName).Output()
			if err != nil {
				return err
			}
			value = strings.TrimSpace(string(out))

			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatalf("Failed to get %q: %v", propertyName, err)
		}

		return value
	}

	// getprop will not return error if the property is not found, just an
	// empty line (\x0a). So we save extra debugging info whenever the
	// regexp does not match.
	saveCpuinfo := false

	manufacturer := getProperty(propertyManufacturer)
	s.Logf("manufacturer: %q", manufacturer)
	// Older models need Mediatek. The correct name going forward is "MediaTek".
	// See b/333501774
	if re := regexp.MustCompile(`^(?:Intel|AMD|Media[tT]ek|Rockchip|Qualcomm)$`); !re.MatchString(manufacturer) {
		s.Errorf("%s property is missing or ill-formed: %q", propertyManufacturer, manufacturer)
		saveCpuinfo = true
	}

	model := getProperty(propertyModel)
	s.Logf("model: %q", model)
	if re := regexp.MustCompile(`^[0-9A-Za-z /-]+$`); !re.MatchString(model) {
		s.Errorf("%s property is missing or ill-formed: %q", propertyModel, model)
		saveCpuinfo = true
	}

	if saveCpuinfo {
		dest := filepath.Join(s.OutDir(), "cpuinfo")
		err := fsutil.CopyFile("/proc/cpuinfo", dest)
		s.Logf("saved <test_output_dir>/cpuinfo (err=%v)", err)

		dest = filepath.Join(s.OutDir(), "soc-compatible")
		err = fsutil.CopyFile("/proc/device-tree/compatible", dest)
		s.Logf("saved <test_output_dir>/soc-compatible (ARM only, err=%v)", err)
	}
}
