// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package font provides tests that verify font availability in the system.
package font

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Webfonts,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that common webfonts are available on the system",
		Contacts: []string{
			"chromeos-fonts@google.com",
			"skau@chromium.org",
		},
		// ChromeOS > Software > UI Foundations > Text/Fonts
		BugComponent: "b:1315543",
		Attr:         []string{"group:mainline", "informational", "group:cq-medium"},
	})
}

// Webfonts verifies that fonts that are popularly used on the internet are available on the device.
func Webfonts(ctx context.Context, s *testing.State) {
	for _, font := range []string{
		"Comic Sans MS",
		"Garamond",
		"Georgia",
		"Tahoma",
		"Trebuchet MS",
		"Verdana",
	} {
		s.Run(ctx, font, func(ctx context.Context, s *testing.State) {
			if err := testFont(ctx, font); err != nil {
				s.Error("Font not found: ", err)
			}
		})
	}
}

func testFont(ctx context.Context, fontName string) error {
	// fc-match takes a pattern for a font name and provides the best
	// match for that font on the system according to fallback rules
	// in fontconfig. Since these fonts are not aliased (remapped to
	// a different font file), the original name is expected in the output.
	// If the font can't be found, the best match is returned which is most
	// likely to be Arimo.
	//
	// Sample output for "fc-match Roboto":
	//     Roboto-Regular.ttf: "Roboto" "Regular"
	commandPath := "/usr/bin/fc-match"
	cmd := testexec.CommandContext(ctx, commandPath, fontName)

	output, err := cmd.Output(testexec.DumpLogOnError)

	if err != nil {
		return errors.Wrapf(err, "failed to run the fc-match tool for %s", fontName)
	}

	parsed := string(output)
	if !strings.Contains(parsed, fontName) {
		return errors.Wrapf(err, "Font %q is not in the system", fontName)
	}

	return nil
}
