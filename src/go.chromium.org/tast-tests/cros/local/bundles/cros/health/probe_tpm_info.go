// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/testing"
)

type tpmVersion struct {
	GscVersion      string           `json:"gsc_version"`
	Family          jsontypes.Uint32 `json:"family"`
	SpecLevel       jsontypes.Uint64 `json:"spec_level"`
	Manufacturer    jsontypes.Uint32 `json:"manufacturer"`
	TpmModel        jsontypes.Uint32 `json:"tpm_model"`
	FirmwareVersion jsontypes.Uint64 `json:"firmware_version"`
	VendorSpecific  *string          `json:"vendor_specific"`
}

type tpmStatus struct {
	Enabled                bool `json:"enabled"`
	Owned                  bool `json:"owned"`
	OwnerPasswordIsPresent bool `json:"owner_password_is_present"`
}

type tpmDictionaryAttack struct {
	Counter                 jsontypes.Uint32 `json:"counter"`
	Threshold               jsontypes.Uint32 `json:"threshold"`
	LockoutInEffect         bool             `json:"lockout_in_effect"`
	LockoutSecondsRemaining jsontypes.Uint32 `json:"lockout_seconds_remaining"`
}

type tpmAttestation struct {
	PreparedForEnrollment bool `json:"prepared_for_enrollment"`
	Enrolled              bool `json:"enrolled"`
}

type tpmSupportedFeatures struct {
	SupportU2f              bool `json:"support_u2f"`
	SupportPinweaver        bool `json:"support_pinweaver"`
	SupportRuntimeSelection bool `json:"support_runtime_selection"`
	IsAllowed               bool `json:"is_allowed"`
}

type tpmInfo struct {
	Version           tpmVersion           `json:"version"`
	Status            tpmStatus            `json:"status"`
	DictionaryAttack  tpmDictionaryAttack  `json:"dictionary_attack"`
	Attestation       tpmAttestation       `json:"attestation"`
	SupportedFeatures tpmSupportedFeatures `json:"supported_features"`
	DidVid            *string              `json:"did_vid"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeTpmInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for TPM info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func ProbeTpmInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryTpm}
	var tpm tpmInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &tpm); err != nil {
		s.Fatal("Failed to get TPM telemetry info: ", err)
	}
}
