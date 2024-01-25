// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"github.com/golang/protobuf/proto"

	apb "go.chromium.org/chromiumos/system_api/attestation_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AttestationDeleteKeys,
		Desc:         "Verifies attestation key deleting functionality",
		Attr:         []string{"group:hwsec", "hwsec_nightly"},
		Contacts:     []string{"cros-hwsec@google.com", "yich@google.com"},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm", "endorsement", "no_tpm_dynamic"},
		Fixture:      "attestationFixture",
	})
}

// AttestationDeleteKeys verifies the the key deleting functionality.
func AttestationDeleteKeys(ctx context.Context, s *testing.State) {
	fixtureData := s.FixtValue().(*fixture.AttestFixture)
	username := fixtureData.Username
	attestation := fixtureData.Attestation
	ac := fixtureData.AttestDBus

	getCertificateRequest := apb.GetCertificateRequest{
		Username: proto.String(username),
	}

	// Cleanup the keys before creating them.
	if err := attestation.DeleteKeys(ctx, username, "label"); err != nil {
		s.Fatal("Failed to remove the key group: ", err)
	}

	s.Log("Verifying deletion of keys by prefix")
	for _, label := range []string{"label1", "label2", "label3"} {
		getCertificateRequest.KeyLabel = proto.String(label)
		certReply, err := ac.GetCertificate(ctx, &getCertificateRequest)
		if err != nil {
			s.Fatalf("Failed to create certificate request for label %q: %v", label, err)
		}
		if *certReply.Status != apb.AttestationStatus_STATUS_SUCCESS {
			s.Fatalf("Failed to get certificate for label %q: %v", label, certReply.Status.String())
		}
		_, err = attestation.GetPublicKey(ctx, username, label)
		if err != nil {
			s.Fatalf("Failed to get public key for label %q: %v", label, err)
		}
	}
	s.Log("Deleting keys just created")
	if err := attestation.DeleteKeys(ctx, username, "label"); err != nil {
		s.Fatal("Failed to remove the key group: ", err)
	}
	for _, label := range []string{"label1", "label2", "label3"} {
		_, err := attestation.GetPublicKey(ctx, username, label)
		var ae *hwsec.AttestationError
		if err == nil {
			s.Fatalf("Key with label %q still found", label)
		}
		if !errors.As(err, &ae) {
			s.Fatalf("Failed to get public key with label %q: %v", label, err)
		}
		if ae.AttestationStatus != apb.AttestationStatus_STATUS_INVALID_PARAMETER {
			s.Fatalf("Unexpected error status: got %s; want STATUS_INVALID_PARAMETER", ae.AttestationStatus)
		}
	}
	s.Log("Deletion of keys by prefix verified")
}
