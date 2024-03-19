// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package oop contains common code to assist with testing functionality
// related to out-of-process video decoding and encoding.
package oop

import (
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/chromeproc"
	"go.chromium.org/tast/core/errors"
)

const videoEncoderUtilSubType = "media.mojom.VideoEncodeAcceleratorProviderFactory"

// VerifyOneUtilityEncoderProcessWasStarted checks that only one utility
// process is opened, regardless of the number of encoders opened.
func VerifyOneUtilityEncoderProcessWasStarted() error {
	procs, err := chromeproc.GetUtilityProcesses()

	if err != nil {
		return errors.Wrap(err, "failed to GetUtilityProcesses()")
	}

	re := regexp.MustCompile(` --?utility-sub-type=([\w\.]+)(?: |$)`)
	numUtilProcs := 0

	for _, proc := range procs {
		cmdline, err := proc.Cmdline()
		if err != nil {
			return errors.Wrap(err, "failed to get cmdline")
		}

		matches := re.FindStringSubmatch(cmdline)
		if len(matches) < 2 {
			continue
		}

		procName := matches[1]
		if procName == videoEncoderUtilSubType {
			numUtilProcs++
		}
	}

	if numUtilProcs != 1 {
		return errors.Errorf("expected 1 video utility process but got %d", numUtilProcs)
	}

	return nil
}
