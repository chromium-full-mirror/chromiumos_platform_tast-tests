// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usb

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GatherPortsData,
		Desc: "Gather data about the I/O ports",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Contacts:     []string{"chromeos-usb-champs@google.com", "danielgeorgem@google.com"},
		Attr:         []string{"group:unowned"},
	})
}

func GatherPortsData(ctx context.Context, s *testing.State) {
	gatherInfo(ctx, s, "lspci", "output_lspci")
	gatherInfo(ctx, s, "status typecd", "output_status_typecd")
	gatherInfo(ctx, s, "modetest -c", "output_modetest")
	gatherInfo(ctx, s, "cros_config / name", "output_cros_config")
	gatherInfo(ctx, s, "ectool pdchipinfo 0", "output_ectool_pdchip")
	gatherInfo(ctx, s, "lsusb -v", "output_lsusb")
}

func gatherInfo(ctx context.Context, s *testing.State, command, filenName string) {
	out, err := testexec.CommandContext(ctx, "sh", "-c", command).Output()

	if err != nil {
		s.Log("Can't execute command: ", command)
		// Write empty file instead of exit code of command to indicate failure.
		writeOutput(s, filenName, make([]byte, 0))
	} else {
		writeOutput(s, filenName, out)
	}
}

func writeOutput(s *testing.State, fileName string, fileContent []byte) {
	if err := ioutil.WriteFile(filepath.Join(s.OutDir(), fileName), fileContent, 0640); err != nil {
		s.Log("Can't write output of: ", fileName)
	}
}
