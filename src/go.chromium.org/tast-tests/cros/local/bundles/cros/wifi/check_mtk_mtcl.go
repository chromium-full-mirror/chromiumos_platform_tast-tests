// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"bytes"
	"context"
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CheckMtkMTCL,
		Desc:         "Runs a preliminary check on device MTCL method for devices with MediaTek 6GHz capable WiFi",
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
			"chromeos-faft@google.com",
		},
		SoftwareDeps: []string{"wifi"},
		Attr: []string{"group:mainline", "group:wificell", "wificell_func", "wificell_dut_validation", "group:labqual",
			// Updating the MTCL tables in CBFS can break this test.
			"group:firmware", "firmware_bios", "firmware_level5",
		},
		// NB: This test is currently only valid on the MT7922 chipset. See http://go/cros-wifi-mtk-disable-6g for implementation details of the feature being tested.
		HardwareDeps:    hwdep.D(hwdep.WifiDevice(hwdep.MediaTekMT7922PCIE)),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates, "sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

// mtcl stores bytes that correspond to the MTCL definition.
type mtcl struct {
	Name           [4]byte
	Version        byte
	Support6g      byte
	CountryList6g  [4]byte
	Reserved       [2]byte
	Support59g     byte
	CountryList59g [4]byte
	Reserved2      [2]byte
}

func (m mtcl) Validate() error {
	// The name MUST be 'MTCL'
	if string(m.Name[:]) != "MTCL" {
		return errors.Errorf("unsupported name: %v", string(m.Name[:]))
	}
	// The only supported version is 2.
	if m.Version != 2 {
		return errors.Errorf("unsupported version: %v", m.Version)
	}
	// Supported states are 0,1,2
	if m.Support6g > 2 {
		return errors.Errorf("unsupported support6g state: %v", m.Support6g)
	}
	// Supported states are 0,1,2
	if m.Support59g > 2 {
		return errors.Errorf("unsupported support59g state: %v", m.Support59g)
	}
	// Reserved values MUST be 0
	if bytes.Compare(m.Reserved[:], []byte{0x00, 0x00}) != 0 {
		return errors.Errorf("unsupported first reserved segment: %v", m.Reserved)
	}
	if bytes.Compare(m.Reserved2[:], []byte{0x00, 0x00}) != 0 {
		return errors.Errorf("unsupported first reserved segment: %v", m.Reserved2)
	}
	return nil
}

// getMTCLString parses the SSDT table (in bytes) into a string that has only the MTCL method definition.
//
// If the MTCL function is not found, the return value is "".
// If the MTCL function is found, the output is expected to be the MTCL ASL function with blank
// space removed.
// Below is an example of the format for the ASL data for an MTCL method.
//
//	     Method(MTCL,0,Serialized)
//		{
//		  Name(LIST, Package(0x13)
//		  {
//		    0x4D, // 'M'
//		    0x54, // 'T'
//		    0x43, // 'C'
//		    0x4C, // 'L'
//		    0x02, // Version 2: 6GHz, 5.9GHz
//		    0x01, // Enable 6GHz if driver and BIOS both support this country.
//		    0x80, // Country List 1. Driver controls other countries.
//		    0x00, // Country List 2. 6GHz operation disabled for these countries.
//		    0x00, // Country List 3. 6GHz operation disabled for these countries.
//		    0x00, // Country List 4. 6GHz operation disabled for these countries.
//		    0x00, // Reserved.
//		    0x00, // Reserved.
//		    0x01, // Enable 5.9GHz if driver and BIOS both support this country.
//		    0x80, // Country List 1. Driver controls other countries.
//		    0x00, // Country List 2. 5.9GHz operation disabled for these countries.
//		    0x00, // Country List 3. 5.9GHz operation disabled for these countries.
//		    0x00, // Country List 4. 5.9GHz operation disabled for these countries.
//		    0x00, // Reserved.
//		    0x00, // Reserved.
//		  })
//		  Return(LIST)
//		}
//
// Example output in the case the method above is in the SSDT input:
// "Method(MTCL,0,Serialized){Name(LIST,Package(0x13){0x4D,0x54,0x43,0x4C,0x02,0x01,0x00,0x00,0x00,0x00,0x00,0x00,0x01,0x00,0x00,0x00,0x00,0x00,0x00})Return(LIST)/*\_SB_.PCI0.RP01.WF00.MTCL.LIST*/}"
func getMTCLString(ssdt []byte) (string, error) {
	m := string(ssdt)

	// Remove spaces and newlines to make processing easier.
	m = strings.Replace(m, "\n", "", -1)
	m = strings.Replace(m, " ", "", -1)

	// Exit early if no MTCL
	i := strings.Index(m, "Method(MTCL")
	if i == -1 {
		return "", nil
	}

	// limit the data string we're manipulating to the specific method we're interested in.
	m = m[i:]
	j := strings.Index(m, "}") + 1
	if j == 0 {
		return "", errors.New("Malformed MTCL method. Missing first closing bracket")
	}

	k := strings.Index(m[j:], "}") + 1
	if k == 0 {
		return "", errors.New("Malformed MTCL method. Missing second closing bracket")
	}

	m = m[:j+k]

	return m, nil
}

// verifyMTCLString converts the MTCL method text as a string into an mtcl struct.
//
// The method returns an error if the string is invalid.
func verifyMTCLString(method string) (mtcl, error) {
	var rv mtcl

	// Verify that the method signature is correct
	if strings.Contains(method, "Method(MTCL,0,Serialized)") == false {
		return rv, errors.New("MTCL exists, but method signature is malformed")
	}

	// Verify that the method returns the correct value
	if strings.Contains(method, "Return(LIST)") == false {
		return rv, errors.New("Method does not return the list value")
	}

	// Verify that the method contains a Package named List that's the appropriate size.
	pi := strings.Index(method, "{Name(LIST,Package(0x13){")
	if pi == -1 {
		return rv, errors.New("Method does not return a package named list that's 19 bytes in size")
	}

	// Parse the data into data values
	dv := strings.Split(method[pi+len("{Name(LIST,Package(0x13){"):strings.Index(method, "})")], ",")

	if len(dv) != 19 {
		return rv, errors.New("Package does not contain 19 elements")
	}

	var bv [19]byte
	for i := 0; i < len(dv); i++ {
		tmp, err := strconv.ParseUint(dv[i], 0, 8)
		if err != nil {
			return rv, err
		}
		bv[i] = byte(tmp)
	}

	err := binary.Read(bytes.NewBuffer(bv[:]), binary.BigEndian, &rv)

	return rv, err
}

func CheckMtkMTCL(ctx context.Context, s *testing.State) {
	// Check to see if the system had an mtcl file configured.
	sp, err := crosconfig.Get(ctx, "/wifi/mtcl-file", "system-path")
	if crosconfig.IsNotFound(err) {
		s.Log("No MTCL file location in configfs")
		return
	} else if err != nil {
		s.Fatal("Failed to execute cros_config : ", err)
	}

	s.Log("MTCL file system-path in configfs: ", sp)

	var pathToSSDT string
	for _, path := range []string{
		// SSDT (Secondary System Description Table) contains MTCL data
		// in encoded binary format. May show up at different paths
		// depending on the platform.
		"/sys/firmware/acpi/tables/SSDT",
		"/sys/firmware/acpi/tables/SSDT1",
	} {
		if _, err := os.Stat(path); err == nil {
			s.Log("Found SSDT at: ", path)
			pathToSSDT = path
			break
		} else if !os.IsNotExist(err) {
			s.Fatalf("Stat(%q) failed: %v", path, err)
		}
	}
	if pathToSSDT == "" {
		s.Fatal("Failed to find SSDT path")
	}
	SSDTRaw, err := ioutil.ReadFile(pathToSSDT)
	if err != nil {
		s.Fatal("Could not read SSDT data: ", err)
	}
	// Write encoded SSDT data to temp file.
	tmpSSDT, err := ioutil.TempFile("", "tempSSDT")
	if err != nil {
		s.Fatal("Could not create temp file for SSDT: ", err)
	}
	defer os.Remove(tmpSSDT.Name())
	if _, err := tmpSSDT.Write(SSDTRaw); err != nil {
		s.Fatal("Could not write to temp SSDT file: ", err)
	}

	// Use iasl to decode the data into ASL format.
	cmd := testexec.CommandContext(ctx, "iasl", "-d", tmpSSDT.Name())
	if err = cmd.Run(); err != nil {
		s.Fatal("Could not run iasl on Dut: ", err)
	}
	// Read in the decoded table.
	pathToDecodedSSDT := tmpSSDT.Name() + ".dsl"
	decodedSSDT, err := ioutil.ReadFile(pathToDecodedSSDT)
	defer os.Remove(pathToDecodedSSDT)
	if err != nil {
		s.Fatal("SSDT decoding failed: ", err)
	}

	// Write the decoded SSDT table to an output file.
	ssdtOut, err := os.Create(filepath.Join(s.OutDir(), "decodedSSDT"))
	defer ssdtOut.Close()
	ssdtOut.Write(decodedSSDT)

	m, err := getMTCLString(decodedSSDT)
	if err != nil {
		s.Fatal("Could not get MTCL method: ", err)
	}
	if m != "" {
		cl, err := verifyMTCLString(m)
		if err != nil {
			s.Fatal("Could not parse MTCL method: ", err)
		}
		err = cl.Validate()
		if err != nil {
			s.Fatal("MTCL data payload is invalid: ", err)
		}
		s.Logf("Found valid MTCL function in SSDT: string: %v, struct: %+v", m, cl)
	} else {
		// Check coreboot logs for errors
		cbl, err := testexec.CommandContext(ctx, "cbmem", "-1").Output()
		if err != nil {
			s.Fatal("Could not get coreboot logs: ", err)
		}
		cbOut, err := os.Create(filepath.Join(s.OutDir(), "coreboot.log"))
		defer cbOut.Close()
		cbOut.Write(cbl)
		s.Fatal("No MTCL function found in SSDT; check coreboot.log for errors")
	}
}
