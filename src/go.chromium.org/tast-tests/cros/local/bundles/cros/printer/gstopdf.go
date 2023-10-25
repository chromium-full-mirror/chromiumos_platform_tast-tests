// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/ghostscript"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Gstopdf,
		Desc:     "Tests that the gstopdf CUPS filter produces expected output",
		Contacts: []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		// Test currently requires cros_internal, likely due to fonts.
		SoftwareDeps: []string{"cros_internal", "cups", "ghostscript"},
		Data:         []string{"gstopdf_input.ps", "gstopdf_golden.pdf"},
	})
}

func Gstopdf(ctx context.Context, s *testing.State) {
	const (
		gsFilter = "gstopdf"
		input    = "gstopdf_input.ps"
		golden   = "gstopdf_golden.pdf"
		envVar   = "CUPS_SERVERBIN=/usr/libexec/cups"
	)
	ghostscript.RunTest(ctx, s, gsFilter, s.DataPath(input), s.DataPath(golden), envVar)
}
