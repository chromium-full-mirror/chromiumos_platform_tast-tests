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
	"go.chromium.org/tast-tests/cros/remote/compliance/ucd"
	"go.chromium.org/tast/core/testing"
)

type ucdParam struct {
	TestName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         UCDCompliance,
		Desc:         "Runs a suite of DP Compliance tests on UCD500 and extracts the results",
		Contacts:     []string{"chromeos-usb-champs@google.com", "jstanko@chromium.org", "kamilplucinski@google.com", "bszpila@google.com"},
		BugComponent: "b:1507626",
		Attr:         []string{"group:typec", "typec_compliance_ex350"},
		Params: []testing.Param{
			{
				Name: "dpcompliance",
				Val:  &ucdParam{TestName: "DPCompliance"},
			},
		},
		Fixture:     "complianceHostFixture",
		Timeout:     10 * time.Minute,
		ServiceDeps: []string{"tast.cros.baserpc.FileSystem"},
	})
}

func UCDCompliance(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*compliance.TestFixture)

	controller := ucd.NewController(fixt.Host)

	// run the UCD script on the windows host that starts the compliance tests
	compliance, err := controller.RunCompliance(ctx)
	if err != nil {
		s.Fatal("Failed to run test via UCD CLI: ", err)
	}
	s.Log(compliance)

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get out dir")
	}

	// Extracts report file
	if err := controller.Host.GetFile(ctx, "'"+"C:\\Users\\Public\\Compliance\\UCD\\Unigraf\\latest.html"+"'", outDir); err != nil {
		s.Fatal("Failed to fetch file from compliance host: ", err)
	}
	file, err := os.Open(filepath.Join(outDir, "latest.html"))
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	foundFailed := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "FAILED") {
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
