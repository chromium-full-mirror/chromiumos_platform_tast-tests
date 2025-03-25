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
		Func:     Ex350Compliance,
		Desc:     "Runs a suite of USB PD Compliance tests on EX350 and extracts the results",
		Contacts: []string{"chromeos-usb-champs@google.com", "jstanko@chromium.org", "kamilplucinski@google.com", "bszpila@google.com"},
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
			{
				Name: "testpdprotall1",
				Val:  &param{TestName: "TEST.PD.PROT.ALL.1"},
			},
			{
				Name: "testpdprotall2",
				Val:  &param{TestName: "TEST.PD.PROT.ALL.2"},
			},
			{
				Name: "testpdprotall3",
				Val:  &param{TestName: "TEST.PD.PROT.ALL.3"},
			},
			{
				Name: "testpdprotall4",
				Val:  &param{TestName: "TEST.PD.PROT.ALL.4"},
			},
			{
				Name: "testpdprotall5",
				Val:  &param{TestName: "TEST.PD.PROT.ALL.5"},
			},
			{
				Name: "testpdprotall31",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.1"},
			},
			{
				Name: "testpdprotall32",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.2"},
			},
			{
				Name: "testpdprotall33",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.3"},
			},
			{
				Name: "testpdprotall34",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.4"},
			},
			{
				Name: "testpdprotall35",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.5"},
			},
			{
				Name: "testpdprotall36",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.6"},
			},
			{
				Name: "testpdprotall37",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.7"},
			},
			{
				Name: "testpdprotall38",
				Val:  &param{TestName: "TEST.PD.PROT.ALL3.8"},
			},
			{
				Name: "testpdprotport31",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.1"},
			},
			{
				Name: "testpdprotport32",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.2"},
			},
			{
				Name: "testpdprotport33",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.3"},
			},
			{
				Name: "testpdprotport34",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.4"},
			},
			{
				Name: "testpdprotport35",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.5"},
			},
			{
				Name: "testpdprotport36",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.6"},
			},
			{
				Name: "testpdprotport37",
				Val:  &param{TestName: "TEST.PD.PROT.PORT3.7"},
			},
			{
				Name: "testpdprotsrc1",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.1 Get_Source_Cap Response"},
			},
			{
				Name: "testpdprotsrc2",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.2"},
			},
			{
				Name: "testpdprotsrc3",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.3"},
			},
			{
				Name: "testpdprotsrc4",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.4"},
			},
			{
				Name: "testpdprotsrc5",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.5"},
			},
			{
				Name: "testpdprotsrc6",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.6"},
			},
			{
				Name: "testpdprotsrc7",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.7"},
			},
			{
				Name: "testpdprotsrc8",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.8"},
			},
			{
				Name: "testpdprotsrc9",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.9"},
			},
			{
				Name: "testpdprotsrc10",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.10"},
			},
			{
				Name: "testpdprotsrc11",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.11"},
			},
			{
				Name: "testpdprotsrc12",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.12"},
			},
			{
				Name: "testpdprotsrc13",
				Val:  &param{TestName: "TEST.PD.PROT.SRC.13"},
			},
			{
				Name: "testpdprotsrc31",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.1 SourceCapabilityTimer Timeout"},
			},
			{
				Name: "testpdprotsrc32",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.2"},
			},
			{
				Name: "testpdprotsrc33",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.3"},
			},
			{
				Name: "testpdprotsrc34",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.4"},
			},
			{
				Name: "testpdprotsrc35",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.5"},
			},
			{
				Name: "testpdprotsrc36",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.6"},
			},
			{
				Name: "testpdprotsrc37",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.7"},
			},
			{
				Name: "testpdprotsrc38",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.8"},
			},
			{
				Name: "testpdprotsrc39",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.9"},
			},
			{
				Name: "testpdprotsrc310",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.10"},
			},
			{
				Name: "testpdprotsrc311",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.11"},
			},
			{
				Name: "testpdprotsrc312",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.12"},
			},
			{
				Name: "testpdprotsrc313",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.13"},
			},
			{
				Name: "testpdprotsrc314",
				Val:  &param{TestName: "TEST.PD.PROT.SRC3.14"},
			},
			{
				Name: "testpdprotsnk1",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.1 Get_Sink_Cap Response"},
			},
			{
				Name: "testpdprotsnk2",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.2"},
			},
			{
				Name: "testpdprotsnk3",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.3"},
			},
			{
				Name: "testpdprotsnk4",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.4"},
			},
			{
				Name: "testpdprotsnk5",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.5"},
			},
			{
				Name: "testpdprotsnk6",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.6"},
			},
			{
				Name: "testpdprotsnk7",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.7"},
			},
			{
				Name: "testpdprotsnk8",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.8"},
			},
			{
				Name: "testpdprotsnk9",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.9"},
			},
			{
				Name: "testpdprotsnk10",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.10"},
			},
			{
				Name: "testpdprotsnk11",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.11"},
			},
			{
				Name: "testpdprotsnk12",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.12"},
			},
			{
				Name: "testpdprotsnk13",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.13"},
			},
			{
				Name: "testpdprotsnk14",
				Val:  &param{TestName: "TEST.PD.PROT.SNK.14"},
			},
			{
				Name: "testpdprotsnk31",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.1"},
			},
			{
				Name: "testpdprotsnk32",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.2"},
			},
			{
				Name: "testpdprotsnk33",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.3"},
			},
			{
				Name: "testpdprotsnk34",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.4"},
			},
			{
				Name: "testpdprotsnk35",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.5"},
			},
			{
				Name: "testpdprotsnk36",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.6"},
			},
			{
				Name: "testpdprotsnk37",
				Val:  &param{TestName: "TEST.PD.PROT.SNK3.7"},
			},
			{
				Name: "testpdvdmsrc1",
				Val:  &param{TestName: "TEST.PD.VDM.SRC.1"},
			},
			{
				Name: "testpdvdmsrc2",
				Val:  &param{TestName: "TEST.PD.VDM.SRC.2"},
			},
			{
				Name: "testpdvdmsnk1",
				Val:  &param{TestName: "TEST.PD.VDM.SNK.1"},
			},
			{
				Name: "testpdvdmsnk2",
				Val:  &param{TestName: "TEST.PD.VDM.SNK.2"},
			},
			{
				Name: "testpdvdmsnk5",
				Val:  &param{TestName: "TEST.PD.VDM.SNK.5"},
			},
			{
				Name: "testpdvdmsnk6",
				Val:  &param{TestName: "TEST.PD.VDM.SNK.6"},
			},
			{
				Name: "testpdvdmsnk7",
				Val:  &param{TestName: "TEST.PD.VDM.SNK.7"},
			},
			{
				Name: "testpdeprsrc31",
				Val:  &param{TestName: "TEST.PD.EPR.SRC3.1 EPR Entry Process - UUT as VCONN Source"},
			},
			{
				Name: "testpdeprsrc310",
				Val:  &param{TestName: "TEST.PD.EPR.SRC3.10"},
			},
			{
				Name: "testpdeprsnk31",
				Val:  &param{TestName: "TEST.PD.EPR.SNK3.1"},
			},
			{
				Name: "testpdusb4drst1",
				Val:  &param{TestName: "TEST.PD.USB4.DRST.1"},
			},
			{
				Name: "testpdusb4drst2",
				Val:  &param{TestName: "TEST.PD.USB4.DRST.2"},
			},
			{
				Name: "td411",
				Val:  &param{TestName: "TD.4.1.1"},
			},
			{
				Name: "td461",
				Val:  &param{TestName: "TD.4.6.1"},
			},
			{
				Name: "td462",
				Val:  &param{TestName: "TD.4.6.2"},
			},
			{
				Name: "td463",
				Val:  &param{TestName: "TD.4.6.3"},
			},
			{
				Name: "td464",
				Val:  &param{TestName: "TD.4.6.4"},
			},
			{
				Name: "td465",
				Val:  &param{TestName: "TD.4.6.5"},
			},
			{
				Name: "td466",
				Val:  &param{TestName: "TD.4.6.6"},
			},
			{
				Name: "td481",
				Val:  &param{TestName: "TD.4.8.1"},
			},
			{
				Name: "td483",
				Val:  &param{TestName: "TD.4.8.3"},
			},
			{
				Name: "td492",
				Val:  &param{TestName: "TD.4.9.2"},
			},
			{
				Name: "td493",
				Val:  &param{TestName: "TD.4.9.3"},
			},
			{
				Name: "td494",
				Val:  &param{TestName: "TD.4.9.4"},
			},
			{
				Name: "td4101",
				Val:  &param{TestName: "TD.4.10.1"},
			},
			{
				Name: "td4102",
				Val:  &param{TestName: "TD.4.10.2"},
			},
			{
				Name: "td4104",
				Val:  &param{TestName: "TD.4.10.4"},
			},
			{
				Name: "td4105",
				Val:  &param{TestName: "TD.4.10.5"},
			},
			{
				Name: "td411d1",
				Val:  &param{TestName: "TD.4.11.1"},
			},
		},
		Fixture:     "complianceHostFixture",
		Timeout:     8 * time.Minute,
		ServiceDeps: []string{"tast.cros.baserpc.FileSystem"},
	})
}

func Ex350Compliance(ctx context.Context, s *testing.State) {
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
