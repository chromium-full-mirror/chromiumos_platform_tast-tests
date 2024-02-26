// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"

	apb "go.chromium.org/chromiumos/system_api/attestation_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/fixture"
	"go.chromium.org/tast/core/testing"
)

type attestParam struct {
	profile    apb.CertificateProfile
	keyType    apb.KeyType
	systemCert bool
	cannotSign bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Attestation,
		Desc:         "Verifies attestation-related functionality",
		Attr:         []string{"group:hwsec", "hwsec_nightly"},
		Contacts:     []string{"cros-hwsec@google.com", "chingkang@google.com"},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm", "endorsement", "no_tpm_dynamic"},
		Fixture:      "attestationFixture",
		Params: []testing.Param{{
			Name:      "enterprise_machine_certificate_rsa_system",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_MACHINE_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			Name:      "enterprise_machine_certificate_rsa_user",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_MACHINE_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			Name:              "enterprise_machine_certificate_ecc_system",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_MACHINE_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			Name:              "enterprise_machine_certificate_ecc_user",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_MACHINE_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			Name:      "enterprise_user_certificate_rsa_system_rsa_system",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			Name:      "enterprise_user_certificate_rsa_system_rsa_user",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			Name:              "enterprise_user_certificate_rsa_system_ecc_system",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			Name:              "enterprise_user_certificate_rsa_system_ecc_user",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "content_protection_certificate_rsa_system",
			Val: attestParam{
				profile:    apb.CertificateProfile_CONTENT_PROTECTION_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "content_protection_certificate_rsa_user",
			Val: attestParam{
				profile:    apb.CertificateProfile_CONTENT_PROTECTION_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "content_protection_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_CONTENT_PROTECTION_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "content_protection_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_CONTENT_PROTECTION_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			// TODO(b/324441573): Enable this test after we fix the server side code.
			Name: "enterprise_enrollment_certificate_rsa_system",
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_ENROLLMENT_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			// TODO(b/324441573): Enable this test after we fix the server side code.
			Name: "enterprise_enrollment_certificate_rsa_user",
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_ENROLLMENT_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			// TODO(b/324441573): Enable this test after we fix the server side code.
			Name:              "enterprise_enrollment_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_ENROLLMENT_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			// TODO(b/324441573): Enable this test after we fix the server side code.
			Name:              "enterprise_enrollment_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_ENROLLMENT_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			// This test requires the device ADID.
			Name:              "enterprise_vtpm_ek_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_VTPM_EK_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
				cannotSign: true,
			},
		}, {
			// This test requires the device ADID.
			Name:              "enterprise_vtpm_ek_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ENTERPRISE_VTPM_EK_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
				cannotSign: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "soft_bind_certificate_rsa_system",
			Val: attestParam{
				profile:    apb.CertificateProfile_SOFT_BIND_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "soft_bind_certificate_rsa_user",
			Val: attestParam{
				profile:    apb.CertificateProfile_SOFT_BIND_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "soft_bind_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_SOFT_BIND_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "soft_bind_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_SOFT_BIND_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "device_setup_certificate_rsa_system",
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_SETUP_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name: "device_setup_certificate_rsa_user",
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_SETUP_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "device_setup_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_SETUP_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			// This test requires the device in verified mode.
			Name:              "device_setup_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_SETUP_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			// TODO(b/256992149): Waiting for crrev.com/c/5179676
			Name:              "arc_tpm_certifying_key_certificate_ecc_system",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ARC_TPM_CERTIFYING_KEY_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
				cannotSign: true,
			},
		}, {
			// TODO(b/256992149): Waiting for crrev.com/c/5179676
			Name:              "arc_tpm_certifying_key_certificate_ecc_user",
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ARC_TPM_CERTIFYING_KEY_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
				cannotSign: true,
			},
		}, {
			Name:              "arc_attestation_device_key_certificate_ecc_system",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ARC_ATTESTATION_DEVICE_KEY_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			Name:              "arc_attestation_device_key_certificate_ecc_user",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_ARC_ATTESTATION_DEVICE_KEY_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}, {
			Name:      "device_trust_user_certificate_rsa_system",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_TRUST_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: true,
			},
		}, {
			Name:      "device_trust_user_certificate_rsa_user",
			ExtraAttr: []string{"firmware_cr50", "group:firmware", "group:attestation"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_TRUST_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_RSA,
				systemCert: false,
			},
		}, {
			Name:              "device_trust_user_certificate_ecc_system",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_TRUST_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: true,
			},
		}, {
			Name:              "device_trust_user_certificate_ecc_user",
			ExtraAttr:         []string{"firmware_cr50", "group:firmware", "group:attestation"},
			ExtraSoftwareDeps: []string{"tpm2"},
			Val: attestParam{
				profile:    apb.CertificateProfile_DEVICE_TRUST_USER_CERTIFICATE,
				keyType:    apb.KeyType_KEY_TYPE_ECC,
				systemCert: false,
			},
		}},
	})
}

// Attestation runs through the attestation flow, including enrollment, cert, sign challenge.
// Also, it verifies the the key access functionality.
func Attestation(ctx context.Context, s *testing.State) {
	fixtureData := s.FixtValue().(*fixture.AttestFixture)
	username := fixtureData.Username
	attestation := fixtureData.Attestation
	at := fixtureData.AttestTest
	ac := fixtureData.AttestDBus

	param := s.Param().(attestParam)
	profile := param.profile
	keyType := param.keyType

	if param.systemCert {
		username = ""
	}

	getCertificateRequest := apb.GetCertificateRequest{
		CertificateProfile: &profile,
		KeyType:            &keyType,
		Username:           proto.String(username),
		KeyLabel:           proto.String(hwsec.DefaultCertLabel),
	}

	if profile == apb.CertificateProfile_DEVICE_SETUP_CERTIFICATE {
		id, err := uuid.NewRandom()
		if err != nil {
			s.Fatal("Failed to generate uuid: ", err)
		}

		getCertificateRequest.Metadata = &apb.GetCertificateRequest_DeviceSetupCertificateRequestMetadata{
			DeviceSetupCertificateRequestMetadata: &apb.DeviceSetupCertificateRequestMetadata{
				Id:             proto.String(id.String()),
				ContentBinding: proto.String(id.String()),
			},
		}
	}

	// Cleanup the keys before creating them.
	if err := attestation.DeleteKeys(ctx, username, hwsec.DefaultCertLabel); err != nil {
		s.Fatal("Failed to remove the default key: ", err)
	}

	certReply, err := ac.GetCertificate(ctx, &getCertificateRequest)
	if err != nil {
		s.Fatal("Failed to call D-Bus API to get certificate: ", err)
	}
	if *certReply.Status != apb.AttestationStatus_STATUS_SUCCESS {
		s.Fatal("Failed to get certificate: ", certReply.Status.String())
	}

	if !param.cannotSign {
		// TODO(b/165426637): Enable it after we inject the fake device policy with customer ID.
		if username != "" {
			if err := at.SignEnterpriseChallenge(ctx, apb.VerifiedAccessFlow_ENTERPRISE_USER, username, hwsec.DefaultCertLabel); err != nil {
				s.Fatal("Failed to sign enterprise challenge: ", err)
			}
		}

		if err := at.SignSimpleChallenge(ctx, username, hwsec.DefaultCertLabel); err != nil {
			s.Fatal("Failed to sign simple challenge: ", err)
		}
	}

	s.Log("Start key payload closed-loop testing")
	s.Log("Setting key payload")
	expectedPayload := hwsec.DefaultKeyPayload
	_, err = attestation.SetKeyPayload(ctx, username, hwsec.DefaultCertLabel, expectedPayload)
	if err != nil {
		s.Fatal("Failed to set key payload: ", err)
	}
	s.Log("Getting key payload")
	resultPayload, err := attestation.GetKeyPayload(ctx, username, hwsec.DefaultCertLabel)
	if err != nil {
		s.Fatal("Failed to get key payload: ", err)
	}
	if resultPayload != expectedPayload {
		s.Fatalf("Inconsistent paylaod -- result: %s / expected: %s", resultPayload, expectedPayload)
	}
	s.Log("Start key payload closed-loop done")

	s.Log("Start verifying key registration")
	isSuccessful, err := attestation.RegisterKeyWithChapsToken(ctx, username, hwsec.DefaultCertLabel)
	if err != nil {
		s.Fatal("Failed to register key with chaps token due to error: ", err)
	}
	if !isSuccessful {
		s.Fatal("Failed to register key with chaps token")
	}
	// Now the key has been registered and remove from the key store
	_, err = attestation.GetPublicKey(ctx, username, hwsec.DefaultCertLabel)
	if err == nil {
		s.Fatal("Undesired successful operation -- key should be removed after registration")
	}
	// Well, actually we need more on system key so the key registration is validated.
	s.Log("Key registration verified")
}
