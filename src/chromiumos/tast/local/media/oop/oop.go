// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package oop contains common code to assist with testing functionality
// related to out-of-process video decoding and encoding.
package oop

import(
	"regexp"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/chromeproc"
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

	// numUtilProcs should be two here because the video encoder sandbox
	// opens a broker process with the same --utility-sub-type as the
	// utility process.
	if numUtilProcs != 2 {
		return errors.Errorf("expected 2 processes (broker + utility) but got %d", numUtilProcs)
	}

	return nil
}
