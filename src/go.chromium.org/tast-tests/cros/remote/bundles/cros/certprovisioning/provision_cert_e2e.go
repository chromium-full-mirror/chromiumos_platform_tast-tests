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

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	ppb "go.chromium.org/tast-tests/cros/services/cros/policy"
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
	// TODO(b/486823783): Remove the disabling after the problem with OobeAutoEnrollmentCheckForced is fixed.
	commonChromeFlags = "--disable-features=OobeAutoEnrollmentCheckForced"
)

type testParams struct {
	gaiaTestParams  gaiaenrollment.TestParams
	chromeFlags     string
	deviceProfileID string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ProvisionCertE2E,
		Desc: "Provision a client certificate with real DMServer",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"gschwarz@google.com",
			"miersh@google.com",
		},
		BugComponent: "b:1000044",
		Attr: []string{
			"group:tape-daily",
			"group:golden_tier_secondary", // TODO: Keep golden_tier suite until b/321909589 is resolved.
		},
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
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Use Windows infra to provision client certificate (COM_FOUND_CUJ2_TASK4_WF1).
				Value: "screenplay-305c3ff4-9d82-4ebe-b9d2-fc2fbd77f5e8",
			},
			pci.SearchFlag(&policy.RequiredClientCertificateForDevice{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.RequiredClientCertificateForUser{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				// Test that Certificate Provisioning (non-API) works on alpha. See the
				// policy in the managedchrome.com/zzzTape/tape-cert-prov OU. It
				// configures a user and device certificates with VA enabled.
				Name: "alpha", // Static flow.
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningTesting,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "CEA312B8-39AA-4ACE-B7B3-C67D8F3E2FD9",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
				// With "gaia" the test will also be run with DMA enabled.
				ExtraSoftwareDeps: []string{"gaia"},
			},
			{
				// Test that Certificate Provisioning (non-API) works on prod. See the
				// policy in the managedchrome.com/zzzTape/tape-cert-prov OU. It
				// configures a user and device certificates with VA enabled.
				Name: "prod", // Static flow.
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningTesting,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "CEA312B8-39AA-4ACE-B7B3-C67D8F3E2FD9",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
				// With "gaia" the test will also be run with DMA enabled.
				ExtraSoftwareDeps: []string{"gaia"},
			},
			{
				// Test that Certificate Provisioning API works on alpha. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-2 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA enabled and static SCEP challenges.
				Name: "dynamic_alpha",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "3d1c4060-7018-4af6-9240-30d3de469a8b",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
				// With "gaia" the test will also be run with DMA enabled.
				ExtraSoftwareDeps: []string{"gaia"},
			},
			{
				// Test that Certificate Provisioning API works on prod. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-2 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA enabled and static SCEP challenges.
				Name: "dynamic_prod",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "3d1c4060-7018-4af6-9240-30d3de469a8b",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
				// With "gaia" the test will also be run with DMA enabled.
				ExtraSoftwareDeps: []string{"gaia"},
			},
			{
				// Test that Certificate Provisioning API works on alpha. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-5 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA disabled and static SCEP challenges.
				Name: "no_va_dynamic_alpha",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting5,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "be201149-9176-48bc-be86-d12e7cd254e0",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
			},
			{
				// Test that Certificate Provisioning API works on prod. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-5 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA disabled and static SCEP challenges.
				Name: "no_va_dynamic_prod",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting5,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "be201149-9176-48bc-be86-d12e7cd254e0",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
			},
			{
				// Test that Certificate Provisioning API works on alpha. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-6 OU. It configures
				// a user and device certificates that use SCEP profiles with
				// RSA-2048 keys, VA enabled and static SCEP challenges.
				Name: "scep_profile_dynamic_alpha",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting6,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "9fcf3bd8-2fbf-4ac4-9155-7a9cae2f8b24",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
			},
			{
				// Test that Certificate Provisioning API works on prod. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-6 OU. It configures
				// a user and device certificates that use SCEP profiles with
				// RSA-2048 keys, VA enabled and static SCEP challenges.
				Name: "scep_profile_dynamic_prod",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting6,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "9fcf3bd8-2fbf-4ac4-9155-7a9cae2f8b24",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
			},
			{
				// Test that Certificate Provisioning API works on alpha. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-7 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA disabled and static SCEP challenges. ChromeOS is
				// configured to only proceed when it receives an invalidation.
				Name: "required_invalidations_dynamic_alpha",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting7,
					},
					chromeFlags: commonChromeFlags + " --enable-features=CertProvisioningUseOnlyInvalidationsForTesting",

					deviceProfileID: "188adb44-6b18-47b5-86a5-b3f23615d650",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
			},
			{
				// Test that Certificate Provisioning API works on prod. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-7 OU. It configures
				// a user and device certificates that use generic profiles with
				// RSA-2048 keys, VA disabled and static SCEP challenges. ChromeOS is
				// configured to only proceed when it receives an invalidation.
				Name: "required_invalidations_dynamic_prod",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting7,
					},
					chromeFlags:     commonChromeFlags + " --enable-features=CertProvisioningUseOnlyInvalidationsForTesting",
					deviceProfileID: "188adb44-6b18-47b5-86a5-b3f23615d650",
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
			},
			{
				// Test that Certificate Provisioning API works on alpha. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-8 OU. It configures
				// a user and device certificates that use generic profiles with
				// ECC-256 keys, VA disabled and dynamic SCEP challenges.
				Name: "ecc_dscep_dynamic_alpha",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerAlphaURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting8,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "b8712124-d5e8-4025-bca9-c43142773324",
				},

				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
			},
			{
				// Test that Certificate Provisioning API works on prod. See the policy
				// in the managedchrome.com/zzzTape/tape-cert-prov-8 OU. It configures
				// a user and device certificates that use generic profiles with
				// ECC-256 keys, VA disabled and dynamic SCEP challenges.
				Name: "ecc_dscep_dynamic_prod",
				Val: testParams{
					gaiaTestParams: gaiaenrollment.TestParams{
						DMServer: policy.DMServerProdURL,
						PoolID:   tape.BuiltInCertProvisioningAPITesting8,
					},
					chromeFlags:     commonChromeFlags,
					deviceProfileID: "b8712124-d5e8-4025-bca9-c43142773324",
				},

				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerProd",
				}},
			},
		},
		Vars: []string{
			tape.ServiceAccount1,
			tape.ServiceAccount2,
		},
	})
}

func ProvisionCertE2E(ctx context.Context, s *testing.State) {
	param := s.Param().(testParams)
	dmServerURL := param.gaiaTestParams.DMServer
	poolID := dma.TapePool(param.gaiaTestParams.PoolID)

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
	tapeClient, err := tape.GetClientFromLocalCredentials(ctx, []string{s.RequiredVar(tape.ServiceAccount1), s.RequiredVar(tape.ServiceAccount2)})
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

	if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.OrgUnitPath); err != nil {
		s.Fatal("Failed to deprovision device at the beginning: ", err)
	}

	// Deprovision the DUT on the server/backend at the end of the test.
	// As devices might get provisioned even when the enrollment fails we need to
	// defer the deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device at the end: ", err)
		}
	}(cleanupCtx)

	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &pspb.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    acc.Username,
		Password:    acc.Password,
		DmserverURL: dmServerURL,
		ExtraArgs:   param.chromeFlags,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	if err = waitForPolicy(ctx, policyClient, param.deviceProfileID); err != nil {
		s.Error("Failed to wait for policy: ", err)
	}

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

// waitForPolicy continuously refreshes policies until the
// RequiredClientCertificateForDevice policy contains `deviceProfileID`. This is
// useful because shortly after moving to a new OU the device might still get
// device policies from the previous OU.
func waitForPolicy(ctx context.Context, policyClient pspb.PolicyServiceClient, deviceProfileID string) error {
	err := testing.Poll(ctx, func(ctx context.Context) error {
		policy, err := policyClient.GetPolicyValue(ctx, &ppb.GetPolicyValueRequest{
			PolicyName: "RequiredClientCertificateForDevice",
		})
		if err != nil {
			return errors.Wrap(err, "failed to get policy")
		}
		if strings.Contains(policy.JsonValue, deviceProfileID) {
			return nil
		}

		if _, err := policyClient.RefreshPolicies(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to refresh policies")
		}
		return errors.New("still waiting for policies")
	}, &testing.PollOptions{
		Interval: 5 * time.Second,
	})
	return err
}
