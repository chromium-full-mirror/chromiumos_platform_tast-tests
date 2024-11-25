// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	exceptionField = "exception"
	errorCodeField = "error_code"
)

type provisionDkCertTestParam struct {
	useNonBlockingTimeout bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ProvisionDkCert,
		Desc:     "Verifies that arc_attestation correctly provisions and retrieves DK certificates",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
		VarDeps:      []string{},
		Params: []testing.Param{{
			Name: "non_blocking",
			Val:  provisionDkCertTestParam{useNonBlockingTimeout: true},
		}, {
			Name: "blocking",
			Val:  provisionDkCertTestParam{useNonBlockingTimeout: false},
		}},
	})
}

func ProvisionDkCert(ctx context.Context, s *testing.State) {
	s.Log("Provisioning Dk Certificate")
	command := "provision"
	useNonBlockingTimeout := s.Param().(provisionDkCertTestParam).useNonBlockingTimeout
	_, err := execArcAttestationCmd(ctx, command, useNonBlockingTimeout)
	if err != nil {
		s.Fatal("Failed to execute provision command: ", err)
	}

	s.Log("Retrieving certificate chain")
	command = "get_cert_chain"
	commandOutput, err := execArcAttestationCmd(ctx, command /* useNonBlockingTimeout*/, false)
	if err != nil {
		s.Fatal("Failed to execute get_cert_chain command: ", err)
	}
	if err = verifyCerts(commandOutput); err != nil {
		s.Fatal("Failed to verify the certificates: ", err)
	}
}

func execArcAttestationCmd(ctx context.Context, command string, useNonBlockingTimeout bool) (string, error) {
	args := []string{command}
	if useNonBlockingTimeout {
		args = append(args, "--non_blocking_timeout=30")
	}

	outBytes, err := testexec.CommandContext(ctx, "arc-attestation-cmd", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	outString := string(outBytes)
	result, err := extractAndroidStatusCodes(outString)
	if err != nil {
		return "", err
	}

	exceptionCode, isExceptionFound := result[exceptionField]
	errorCode, isErrorFound := result[errorCodeField]
	if !isExceptionFound || !isErrorFound {
		return "", errors.Errorf("Command output was not in expected format: %s", outString)
	}
	if exceptionCode != 0 || errorCode != 0 {
		return "", errors.Errorf("Command failed with exception: %d, error: %d", exceptionCode, errorCode)
	}
	return outString, nil
}

// extractAndroidStatusCodes extracts the exception and error codes from command output.
// Example libarc-attestation command output:
//
//	[arc_attestation.ProvisionCmdResult] {
//	  status: [arc_attestation.PrintableAndroidStatus] {
//	    exception: 0
//	    error_code: 0
//	    msg:
//	  }
//	}
func extractAndroidStatusCodes(commandResult string) (map[string]int, error) {
	statusCodes := make(map[string]int)

	androidStatusRegexPattern := "(exception|error_code): (\\d+)"
	re := regexp.MustCompile(androidStatusRegexPattern)
	for _, line := range strings.Split(commandResult, "\n") {
		fields := re.FindStringSubmatch(line)
		if fields == nil {
			continue
		}
		fieldName := fields[1]
		fieldValue, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, errors.Wrapf(err, "error converting value for %s to int", fieldName)
		}
		statusCodes[fieldName] = fieldValue
	}
	return statusCodes, nil
}

// verifyCerts checks that the result contains 2 alphanumeric certificates.
// The format of a successful get_cert_chain command output:
//
//	[arc_attestation.GetCertChainCmdResult] {
//		 status: [arc_attestation.PrintableAndroidStatus] {
//		   exception: 0
//		   error_code: 0
//		   msg:
//		 }
//		 certs: {
//		   <cert1>,
//		   <cert2>
//		 }
//	}
func verifyCerts(commandResult string) error {
	certRegexPattern := "certs: {([\\s\\w,]+)}"
	re := regexp.MustCompile(certRegexPattern)
	certsMatch := re.FindStringSubmatch(commandResult)
	certsArr := strings.Split(certsMatch[1], ",")
	if len(certsArr) != 2 {
		return errors.Errorf("get_cert_chain command should return 2 certificates but returned %d", len(certsArr))
	}
	return nil
}
