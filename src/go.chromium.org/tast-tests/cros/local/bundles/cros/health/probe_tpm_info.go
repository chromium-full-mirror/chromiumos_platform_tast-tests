// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"encoding/hex"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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

type probeTpmInfoTestParams struct {
	TpmManagerVerification bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ProbeTpmInfo,
		Desc: "Check that we can probe cros_healthd for TPM info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name:              "",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			Val: probeTpmInfoTestParams{
				TpmManagerVerification: false,
			},
		}, {
			Name:              "no_tpm",
			ExtraHardwareDeps: hwdep.D(hwdep.HasNoTpm()),
			Val: probeTpmInfoTestParams{
				TpmManagerVerification: false,
			},
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "tpm_manager_verification",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			Val: probeTpmInfoTestParams{
				TpmManagerVerification: true,
			},
			ExtraAttr: []string{"informational"},
		}},
	})
}

func verifyGscVersion(tpmManagerGscVersion, healthGscVersion string) bool {
	if healthGscVersion == "Cr50" && tpmManagerGscVersion == "GSC_VERSION_CR50" {
		return true
	}
	if healthGscVersion == "Ti50" && tpmManagerGscVersion == "GSC_VERSION_TI50" {
		return true
	}
	if healthGscVersion == "NotGsc" && tpmManagerGscVersion == "GSC_VERSION_NOT_GSC" {
		return true
	}
	return false
}

func verifyTPMVersion(ctx context.Context, tpmManager *hwsec.TPMManagerClient, version tpmVersion) error {
	tpmManagerVersionInfo, err := tpmManager.GetVersionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get version info from TPMManager")
	}
	if verifyGscVersion(tpmManagerVersionInfo.GscVersion, version.GscVersion) != true {
		return errors.Errorf("GscVersion not matched, %v from healthd, %v from TPMManager", version.GscVersion, tpmManagerVersionInfo.GscVersion)
	}
	if tpmManagerVersionInfo.Family != int(version.Family) {
		return errors.Errorf("Family not matched, %v from healthd, %v from TPMManager", version.Family, tpmManagerVersionInfo.Family)
	}
	if tpmManagerVersionInfo.SpecLevel != uint64(version.SpecLevel) {
		return errors.Errorf("SpecLevel not matched, %v from healthd, %v from TPMManager", version.SpecLevel, tpmManagerVersionInfo.SpecLevel)
	}
	if tpmManagerVersionInfo.Manufacturer != int(version.Manufacturer) {
		return errors.Errorf("Manufacturer not matched, %v from healthd, %v from TPMManager", version.Manufacturer, tpmManagerVersionInfo.Manufacturer)
	}
	if tpmManagerVersionInfo.TpmModel != int(version.TpmModel) {
		return errors.Errorf("TpmModel not matched, %v from healthd, %v from TPMManager", version.TpmModel, tpmManagerVersionInfo.TpmModel)
	}
	if tpmManagerVersionInfo.FirmwareVersion != uint64(version.FirmwareVersion) {
		return errors.Errorf("FirmwareVersion not matched, %v from healthd, %v from TPMManager", version.FirmwareVersion, tpmManagerVersionInfo.FirmwareVersion)
	}
	tpmManagerVendorSpecificStringByteStream, err := hex.DecodeString(tpmManagerVersionInfo.VendorSpecific)
	if err != nil {
		return errors.Wrap(err, "failed to decode VendorSpecific from TPMManager")
	}
	tpmManagerVendorSpecificString := string(tpmManagerVendorSpecificStringByteStream)

	// `VendorSpecific` would be nil if the `vendor_specfic` string is an empty string.
	if version.VendorSpecific == nil {
		if tpmManagerVendorSpecificString != "" {
			return errors.Errorf("VendorSpecific not matched, empty string from healthd, %v from TPMManager", tpmManagerVendorSpecificString)
		}
	} else if tpmManagerVendorSpecificString != *version.VendorSpecific {
		return errors.Errorf("VendorSpecific not matched, %v from healthd, %v from TPMManager", version.VendorSpecific, tpmManagerVendorSpecificString)
	}
	return nil
}

func verifyTPMStatus(ctx context.Context, tpmManager *hwsec.TPMManagerClient, status tpmStatus) error {
	tpmManagerStatus, err := tpmManager.GetNonsensitiveStatusIgnoreCache(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get status info from TPMManager")
	}
	if tpmManagerStatus.IsEnabled != status.Enabled {
		return errors.Errorf("Enabled not matched, %v from healthd, %v from TPMManager", status.Enabled, tpmManagerStatus.IsEnabled)
	}
	if tpmManagerStatus.IsOwned != status.Owned {
		return errors.Errorf("Owned not matched, %v from healthd, %v from TPMManager", status.Owned, tpmManagerStatus.IsOwned)
	}
	if tpmManagerStatus.IsOwnerPasswordPresent != status.OwnerPasswordIsPresent {
		return errors.Errorf("OwnerPasswordIsPresent not matched, %v from healthd, %v from TPMManager", status.OwnerPasswordIsPresent, tpmManagerStatus.IsOwnerPasswordPresent)
	}
	return nil
}

func verifyTPMDictionaryAttack(ctx context.Context, tpmManager *hwsec.TPMManagerClient, tpmDA tpmDictionaryAttack) error {
	tpmManagerDAInfo, err := tpmManager.GetDAInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get dictionary attack from TPMManager")
	}
	if tpmManagerDAInfo.Counter != int(tpmDA.Counter) {
		return errors.Errorf("Counter not matched, %v from healthd, %v from TPMManager", tpmDA.Counter, tpmManagerDAInfo.Counter)
	}
	if tpmManagerDAInfo.Threshold != int(tpmDA.Threshold) {
		return errors.Errorf("Threshold not matched, %v from healthd, %v from TPMManager", tpmDA.Threshold, tpmManagerDAInfo.Threshold)
	}
	if tpmManagerDAInfo.InEffect != tpmDA.LockoutInEffect {
		return errors.Errorf("LockoutInEffect not matched, %v from healthd, %v from TPMManager", tpmDA.LockoutInEffect, tpmManagerDAInfo.InEffect)
	}
	if tpmManagerDAInfo.Remaining != int(tpmDA.LockoutSecondsRemaining) {
		return errors.Errorf("LockoutSecondsRemaining not matched, %v from healthd, %v from TPMManager", tpmDA.LockoutSecondsRemaining, tpmManagerDAInfo.Remaining)
	}
	return nil
}

func verifyTPMAttestation(ctx context.Context, attestation tpmAttestation) error {
	ac, err := hwseclocal.NewAttestationDBus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create attestation client")
	}
	attestationClient := hwsec.NewAttestationClient(ac)

	acIsPreparedForEnrollment, err := attestationClient.IsPreparedForEnrollment(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get IsPreparedForEnrollment")
	}
	if acIsPreparedForEnrollment != attestation.PreparedForEnrollment {
		return errors.Errorf("PreparedForEnrollment not matched, %v from healthd, %v from attestationClient", attestation.PreparedForEnrollment, acIsPreparedForEnrollment)
	}
	acIsEnrolled, err := attestationClient.IsEnrolled(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get IsEnrolled")
	}
	if acIsEnrolled != attestation.Enrolled {
		return errors.Errorf("Enrolled not matched, %v from healthd, %v from attestationClient", attestation.Enrolled, acIsEnrolled)
	}

	return nil
}

func verifyTPMSupportedFeatures(ctx context.Context, tpmManager *hwsec.TPMManagerClient, supportedFeatures tpmSupportedFeatures) error {
	tpmManagerSupportedFeatures, err := tpmManager.GetSupportedFeatures(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get supported features from TPMManager")
	}
	if tpmManagerSupportedFeatures.SupportU2F != supportedFeatures.SupportU2f {
		return errors.Errorf("SupportU2F not matched, %v from healthd, %v from TPMManager", supportedFeatures.SupportU2f, tpmManagerSupportedFeatures.SupportU2F)
	}
	if tpmManagerSupportedFeatures.SupportPinweaver != supportedFeatures.SupportPinweaver {
		return errors.Errorf("SupportPinweaver not matched, %v from healthd, %v from TPMManager", supportedFeatures.SupportPinweaver, tpmManagerSupportedFeatures.SupportPinweaver)
	}
	if tpmManagerSupportedFeatures.SupportRuntimeSelection != supportedFeatures.SupportRuntimeSelection {
		return errors.Errorf("SupportRuntimeSelection not matched, %v from healthd, %v from TPMManager", supportedFeatures.SupportRuntimeSelection, tpmManagerSupportedFeatures.SupportRuntimeSelection)
	}
	if tpmManagerSupportedFeatures.IsAllowed != supportedFeatures.IsAllowed {
		return errors.Errorf("IsAllowed not matched, %v from healthd, %v from TPMManager", supportedFeatures.IsAllowed, tpmManagerSupportedFeatures.IsAllowed)
	}

	return nil
}

func ProbeTpmInfo(ctx context.Context, s *testing.State) {
	tpmInfoVerification := s.Param().(probeTpmInfoTestParams).TpmManagerVerification

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryTpm}
	var tpm tpmInfo

	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &tpm); err != nil {
		s.Fatal("Failed to get TPM telemetry info: ", err)
	}

	if tpmInfoVerification {
		cmdRunner := hwseclocal.NewLoglessCmdRunner()
		tpmManager := hwsec.NewTPMManagerClient(cmdRunner)
		if err := verifyTPMVersion(ctx, tpmManager, tpm.Version); err != nil {
			s.Fatal("Failed to verify Version: ", err)
		}
		if err := verifyTPMStatus(ctx, tpmManager, tpm.Status); err != nil {
			s.Fatal("Failed to verify Status: ", err)
		}
		if err := verifyTPMDictionaryAttack(ctx, tpmManager, tpm.DictionaryAttack); err != nil {
			s.Fatal("Failed to verify DictionaryAttack: ", err)
		}
		if err := verifyTPMAttestation(ctx, tpm.Attestation); err != nil {
			s.Fatal("Failed to verify Attestation: ", err)
		}
		if err := verifyTPMSupportedFeatures(ctx, tpmManager, tpm.SupportedFeatures); err != nil {
			s.Fatal("Failed to verify SupportedFeatures: ", err)
		}
	}

}
