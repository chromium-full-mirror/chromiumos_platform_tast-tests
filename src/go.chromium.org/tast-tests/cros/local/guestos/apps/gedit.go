// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// gedit's app name was "Text Editor" from 3.20 through 42.0. We don't care
// about versions older than 3.20.
const geditAppNameVersion = 43

// GeditAppName returns the desktop entry name for gedit.
func GeditAppName(ctx context.Context, guest vm.Guest) (string, error) {
	out, err := guest.Command(ctx, "gedit", "--version").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get gedit version")
	}

	re := regexp.MustCompile(`gedit - Version (\d+)\.(\d+)(?:\.(\d+))?`)
	match := re.FindSubmatch(out)
	if match == nil {
		return "", errors.Wrapf(err, "failed to parse gedit version from %q", out)
	}

	major, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse gedit version number %q", match[1])
	}

	if major < geditAppNameVersion {
		return "Text Editor", nil
	}

	return "gedit", nil
}
