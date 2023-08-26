// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CPUVulnerabilities,
		Desc: "Confirm CPU vulnerabilities are mitigated",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"cpu_vuln_sysfs", "no_qemu"},
	})
}

func CPUVulnerabilities(ctx context.Context, s *testing.State) {
	vulnDir := "/sys/devices/system/cpu/vulnerabilities/"
	fileList, err := ioutil.ReadDir(vulnDir)
	if err != nil {
		s.Fatal("Failed to list vulnerability files: ", err)
	}
	for _, f := range fileList {
		fName := f.Name()
		// We are leaving "spec_rstack_overflow" unmitigated on
		// some boards: b/297466181
		if fName == "spec_rstack_overflow" {
			continue
		}
		contents, err := ioutil.ReadFile(filepath.Join(vulnDir, fName))
		if err != nil {
			s.Fatal("Can't read vulnerability file: ", err)
		}
		contents = bytes.TrimSpace(contents)
		s.Logf("%s: %s", fName, contents)
		if bytes.EqualFold(contents, []byte("vulnerable")) {
			s.Errorf("File %q has CPU vulnerabilities", fName)
		}
	}
}
