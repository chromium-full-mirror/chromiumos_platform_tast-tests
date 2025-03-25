// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bufio"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast-tests/cros/remote/compliance/teledyne"
	"go.chromium.org/tast/core/testing"
)

type teleParam struct {
	TestName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TeledyneCompliance,
		Desc:         "Runs a suite of USB PD Compliance tests on Teledyne testers and extracts the results",
		Contacts:     []string{"chromeos-usb-champs@google.com", "jstanko@google.com", "kamilplucinski@google.com", "bszpila@google.com"},
		BugComponent: "b:1507626",
		Attr:         []string{"group:typec", "typec_compliance_ex350"},
		Params: []testing.Param{
			{
				Name: "usbcompliance",
				Val:  &teleParam{TestName: "usbcompliance"},
			},
		},
		Fixture:     "complianceHostFixture",
		Timeout:     10 * time.Minute,
		ServiceDeps: []string{"tast.cros.baserpc.FileSystem"},
	})
}

func TeledyneCompliance(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*compliance.TestFixture)

	controller := teledyne.NewController(fixt.Host)

	// run the Teledyne script on the windows host that starts the compliance tests
	compliance, err := controller.RunCompliance(ctx)
	if err != nil {
		s.Fatal("Failed to run test via Teledyne CLI: ", err)
	}
	s.Log(compliance)

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get out dir")
	}

	if err := controller.Host.GetFile(ctx, "'"+"C:\\Users\\Public\\Compliance\\Teledyne\\reports\\output\\*"+"'", outDir); err != nil {
		s.Fatal("Failed to fetch file from compliance host: ", err)
	}
	files, err := controller.Host.GetFileNames(ctx, "C:\\Users\\Public\\Compliance\\Teledyne\\reports\\output\\")
	if err != nil {
		s.Fatal("Failed to check empty directory: ", err)
	}
	fileNames := strings.Split(files, "\r\n")
	for _, filename := range fileNames {
		if strings.Contains(filename, "html") {
			file, err := os.Open(filepath.Join(outDir, filename))
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			foundFailed := false
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "FAILED") || strings.Contains(line, "ERROR") || strings.Contains(line, "Errors encountered") {
					foundFailed = true
					break
				}
			}

			if err := scanner.Err(); err != nil {
				s.Fatal("Scanning html file failed: ", err)
			}

			if foundFailed {
				s.Fatal("Failures found on html report, indicating test failed")
			}
			s.Log("No instances of FAILED found")
			return
		}
	}
}
