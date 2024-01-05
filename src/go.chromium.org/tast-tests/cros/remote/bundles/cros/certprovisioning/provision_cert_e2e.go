// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package certprovisioning

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// Requirements on the Admin Console setup:
//   - The device management domain is connected to a Certification Authority for client certificate provisioning
//     (Google Cloud Certificate Connector).
//   - There is an Organizational Unit (OU) where the users from the used TAPE Pools are placed.
//   - That OU has
//     "Devices > Chrome > Settings > Device enrollment" = "Place Chrome device in user organization"
//   - That OU has a device SCEP Profile.
//     This SCEP Profile requests certificates with a specific Subject Organization (see subjectOrgForDeviceCert)
//   - That OU has a user SCEP Profile.
//     This SCEP Profile requests certificates with a specific Subject Organization (see subjectOrgForUserCert)

const (
	subjectOrgForDeviceCert = "TestCompanyNameForDevice"
	subjectOrgForUserCert   = "TestCompanyNameForUser"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProvisionCertE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Provision a client certificate with real DMServer",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"pmarko@google.com",
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:tape-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.hwsec.Pkcs11Service",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Timeout: 6 * time.Minute,
		Fixture: fixture.CleanOwnership,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Use Windows infra to provision client certificate (COM_FOUND_CUJ2_TASK4_WF1).
			Value: "screenplay-305c3ff4-9d82-4ebe-b9d2-fc2fbd77f5e8",
		}},
		Params: []testing.Param{
			{
				Name: "alpha",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerAlphaURL,
					PoolID:   tape.BuiltInCertProvisioningTesting,
				},
			},
			{
				Name: "prod",
				Val: gaiaenrollment.TestParams{
					DMServer: policy.DMServerProdURL,
					PoolID:   tape.BuiltInCertProvisioningTesting,
				},
			},
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

func ProvisionCertE2E(ctx context.Context, s *testing.State) {
	param := s.Param().(gaiaenrollment.TestParams)
	dmServerURL := param.DMServer
	poolID := param.PoolID

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	errorHandler := func(errorMsg string) {
		s.Logf("Test failed, capturing screenshot. errorMsg: %s", errorMsg)
		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "error"})
	}
	s.AttachErrorHandlers(errorHandler, errorHandler)

	// Enterprise-enroll and sign-in using TAPE-provided Owned Test Account.
	policyClient := pspb.NewPolicyServiceClient(cl.Conn)
	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	// Create an account manager and lease a test account for the duration of the test.
	tapeTimeout := int32((1 * time.Minute).Seconds())
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, false /*lock*/, tape.WithTimeout(tapeTimeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	// Deprovision the DUT on the server/backend at the end of the test.
	// As devices might get provisioned even when the enrollment fails we need to
	// defer the deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.CustomerID, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())

	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	cryptohome := helper.CryptohomeClient()

	pkcs11Util, err := pkcs11.NewChaps(ctx, cmdRunner, cryptohome)
	if err != nil {
		s.Fatal("Failed to create PKCS#11 Utility: ", err)
	}

	// Wait for device client certificate to be provisioned.
	if err = waitForClientCert(ctx, pkcs11Util, tokenDevice, subjectOrgForDeviceCert); err != nil {
		s.Error("Failed to find device certificate: ", err)
	}

	// Wait for user client certificate to be provisioned.
	if err = waitForClientCert(ctx, pkcs11Util, tokenUser, subjectOrgForUserCert); err != nil {
		s.Error("Failed to find user certificate: ", err)
	}
}

// pkcs11Token describes a mode to use when running emerge.
type pkcs11Token int

const (
	tokenDevice pkcs11Token = iota
	tokenUser
)

func waitForClientCert(ctx context.Context, pkcs11Util *pkcs11.Chaps, token pkcs11Token, subjectOrganization string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		slotInfos, err := pkcs11Util.ListSlots(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to list slots")
		}
		slotIndex, err := findSlot(slotInfos, token)
		if err != nil {
			return err
		}
		certs, err := pkcs11Util.ListCerts(ctx, slotIndex)
		if err != nil {
			return err
		}
		var subjectOrgs []string
		for _, cert := range certs {
			if slices.Contains(cert.Cert().Subject.Organization, subjectOrganization) {
				return nil
			}
			subjectOrgs = append(subjectOrgs, cert.Cert().Subject.Organization...)
		}
		return errors.Errorf("didn't find subjectOrg %s, found %s", subjectOrganization, strings.Join(subjectOrgs, ","))
	}, &testing.PollOptions{
		Interval: 1 * time.Second,
	})
}

func findSlot(slotInfos []pkcs11.SlotInfo, token pkcs11Token) (int, error) {
	for _, slotInfo := range slotInfos {
		if slotMatches(slotInfo, token) {
			return slotInfo.SlotIndex(), nil
		}
	}
	return -1, errors.Errorf("could not find pkcs#11 slot for %d", token)
}

func slotMatches(slotInfo pkcs11.SlotInfo, token pkcs11Token) bool {
	switch token {
	case tokenDevice:
		return strings.Contains(slotInfo.TokenLabel(), "System")
	case tokenUser:
		return strings.Contains(slotInfo.TokenLabel(), "User")
	default:
		return false
	}
}
