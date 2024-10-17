// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast-tests/cros/remote/compliance/ex350"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"
)

var (
	ex350ReportDir = `C:\Users\CrOSECMinion\Documents\EllisysReports`
)

type param struct {
	TestName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     Ex350ComplianceSmoke,
		Desc:     "Runs a suite of USB PD Compliance tests on EX350 and extracts the results",
		Contacts: []string{"chromeos-usb-champs@google.com", "jstanko@chromium.org"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_compliance_ex350"},
		Params: []testing.Param{
			{
				Name: "sinktests",
				Val:  &param{TestName: "Deterministic USB PD 3.0 Sink Tests"},
			},
			{
				Name: "tdpdsrce1",
				Val:  &param{TestName: "TD.PD.SRC.E1"},
			},
			{
				Name: "tdpdsrce2",
				Val:  &param{TestName: "TD.PD.SRC.E2"},
			},
			{
				Name: "tdpdsrce3",
				Val:  &param{TestName: "TD.PD.SRC.E3"},
			},
			{
				Name: "tdpdsrce4",
				Val:  &param{TestName: "TD.PD.SRC.E4"},
			},
			{
				Name: "tdpdsrce5",
				Val:  &param{TestName: "TD.PD.SRC.E5"},
			},
			{
				Name: "tdpdsrce6",
				Val:  &param{TestName: "TD.PD.SRC.E6"},
			},
			{
				Name: "tdpdsrce7",
				Val:  &param{TestName: "TD.PD.SRC.E7"},
			},
			{
				Name: "tdpdsrce8",
				Val:  &param{TestName: "TD.PD.SRC.E8"},
			},
			{
				Name: "tdpdsrce9",
				Val:  &param{TestName: "TD.PD.SRC.E9"},
			},
			{
				Name: "tdpdsrce10",
				Val:  &param{TestName: "TD.PD.SRC.E10"},
			},
			{
				Name: "tdpdsrce11",
				Val:  &param{TestName: "TD.PD.SRC.E11"},
			},
			{
				Name: "tdpdsrce12",
				Val:  &param{TestName: "TD.PD.SRC.E12"},
			},
			{
				Name: "tdpdsrce13",
				Val:  &param{TestName: "TD.PD.SRC.E13"},
			},
			{
				Name: "tdpdsrce14",
				Val:  &param{TestName: "TD.PD.SRC.E14"},
			},
			{
				Name: "tdpdsrce15",
				Val:  &param{TestName: "TD.PD.SRC.E15"},
			},
			{
				Name: "tdpdsrce16",
				Val:  &param{TestName: "TD.PD.SRC.E16"},
			},
		},
		Fixture:     "complianceHostFixture",
		Timeout:     5 * time.Minute,
		ServiceDeps: []string{"tast.cros.baserpc.FileSystem"},
	})
}

func Ex350ComplianceSmoke(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*compliance.TestFixture)
	// Initialize variables
	testName := s.Param().(*param).TestName
	model, err := reporters.New(s.DUT()).Model(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}
	vifPath := fmt.Sprintf("C:\\Users\\CrOSECMinion\\%s_vif.xml", model)

	controller := ex350.NewController(fixt.Host)
	// run the ex350 command on the windows host
	compliance, err := controller.RunCompliance(ctx, `ex350-62857`, vifPath, testName, ex350ReportDir)
	if err != nil {
		s.Fatal("Failed to run test via ex350 CLI: ", err)
	}
	s.Log(compliance)

	dir, err := controller.Host.GetNewestDirectory(ctx, ex350ReportDir)
	if err != nil {
		s.Fatal("Failed to get newest directory name: ", err)
	}

	finalDir := ex350ReportDir + "\\" + dir
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get out dir")
	}

	if err := controller.Host.GetFile(ctx, "'"+finalDir+"\\*"+"'", outDir); err != nil {
		s.Fatal("Failed to fetch file from compliance host: ", err)
	}

	files, err := controller.Host.GetFileNames(ctx, finalDir)
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
				if strings.Contains(line, "FAILED") {
					foundFailed = true
					break
				}
			}

			if err := scanner.Err(); err != nil {
				s.Fatal("Scanning html file failed: ", err)
			}

			if foundFailed {
				s.Fatal("Failed Found on html report, indicating test failed")
			}
			s.Log("No instances of FAILED found")
			return
		}
	}
	s.Fatalf("No html report was found in the expected directory of %s", finalDir)
}
