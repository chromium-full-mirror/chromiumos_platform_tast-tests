// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GSCtool,
		Desc: "Checks that gsctool can communicate with the GSC",
		Contacts: []string{
			"chromeos-faft@google.com",
			"mruthven@chromium.org",  // GSC Firmware Developer
			"gsc-sheriff@google.com", // GSC Firmware Developers
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		SoftwareDeps: []string{"gsc"},
		Attr:         []string{"group:mainline", "group:labqual"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// GSCtool runs the gsctool utility and confirms that gsctool was able to
// communicate with the GSC (Google Security Chip) by querying its version.
func GSCtool(ctx context.Context, s *testing.State) {
	cmd := testexec.CommandContext(ctx, "gsctool", "--any", "--fwver")
	// GSC firmware versions will be of the form <epoch>.<major>.<minor>.
	const exp = `[0-9]+\.[0-9]+\.[0-9]+`
	re := regexp.MustCompile(exp)
	if out, err := cmd.Output(testexec.DumpLogOnError); err != nil {
		s.Fatalf("%q failed: %v", shutil.EscapeSlice(cmd.Args), err)
	} else if outs := string(out); !re.MatchString(outs) {
		path := filepath.Join(s.OutDir(), "gsctool.txt")
		if err := ioutil.WriteFile(path, out, 0644); err != nil {
			s.Error("Failed to save gsctool output: ", err)
		}
		s.Fatalf("Failed to find a valid GSC version: output of %q did not appear to contain a version (saved output to %s)",
			shutil.EscapeSlice(cmd.Args), filepath.Base(path))
	}
}
