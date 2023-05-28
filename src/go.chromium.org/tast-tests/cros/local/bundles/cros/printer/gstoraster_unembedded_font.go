// Copyright 2020 The ChromiumOS Authors
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
		Func:     GstorasterUnembeddedFont,
		Desc:     "Tests that the gstoraster CUPS filter handles unembedded PDF fonts",
		Contacts: []string{"project-bolton@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"cups", "ghostscript"},
		Data:         []string{fontFile, fontGoldenFile},
	})
}

const (
	fontFile       = "font-test.pdf"
	fontGoldenFile = "font-golden.pwg"
)

func GstorasterUnembeddedFont(ctx context.Context, s *testing.State) {
	ghostscript.RunTest(ctx, s, "gstoraster", s.DataPath(fontFile), s.DataPath(fontGoldenFile), "" /*envVar*/)
}
