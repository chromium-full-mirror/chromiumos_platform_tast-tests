// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50TCG,
		Desc:    "Run TCG Compliance tests against a remote Ti50",
		Timeout: 60 * time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"granaghan@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "se__test_simulated_smart_card",
			Val:  "SE_TestSimulatedSmartCard",
		}, {
			Name: "test_activate",
			Val:  "TestActivate",
		}, {
			Name: "test_activate_ecdh",
			Val:  "TestActivateEcdh",
		}, {
			Name: "test_activate_hmac",
			Val:  "TestActivateHmac",
		}, {
			Name: "test_auth_dec_session",
			Val:  "TestAuthDecSession",
		}, {
			Name: "test_auth_enc_dec_session",
			Val:  "TestAuthEncDecSession",
		}, {
			Name: "test_bad_handles",
			Val:  "TestBadHandles",
		}, {
			Name: "test_bad_sessions",
			Val:  "TestBadSessions",
		}, {
			Name: "test_bogus_hash_ticket",
			Val:  "TestBogusHashTicket",
		}, {
			Name: "test_bound_audit_session",
			Val:  "TestBoundAuditSession",
		}, {
			Name: "test_bound_auth_enc_dec_session",
			Val:  "TestBoundAuthEncDecSession",
		}, {
			Name: "test_cancel",
			Val:  "TestCancel",
		}, {
			Name: "test_certify_creation",
			Val:  "TestCertifyCreation",
		}, {
			Name: "test_certify_creation_primary",
			Val:  "TestCertifyCreationPrimary",
		}, {
			Name: "test_certify_key",
			Val:  "TestCertifyKey",
		}, {
			Name: "test_certify_key_without_nv__certify",
			Val:  "TestCertifyKeyWithoutNv_Certify",
		}, {
			Name: "test_continue_session_attr",
			Val:  "TestContinueSessionAttr",
		}, {
			Name: "test_create_ecc_primary",
			Val:  "TestCreateEccPrimary",
		}, {
			Name: "test_create_errors",
			Val:  "TestCreateErrors",
		}, {
			Name: "test_create_primary_faults",
			Val:  "TestCreatePrimaryFaults",
		}, {
			Name: "test_create_sign_quote",
			Val:  "TestCreateSignQuote",
		}, {
			Name: "test_da_exemption",
			Val:  "TestDAExemption",
		}, {
			Name: "test_duplicate_errors",
			Val:  "TestDuplicateErrors",
		}, {
			Name: "test_duplicate_import",
			Val:  "TestDuplicateImport",
		}, {
			Name: "test_ecc",
			Val:  "TestEcc",
		}, {
			Name: "test_ecc_sig",
			Val:  "TestEccSig",
		}, {
			Name: "test_ecc_sig2",
			Val:  "TestEccSig2",
		}, {
			Name: "test_ec_schnorr_coverage",
			Val:  "TestEcSchnorrCoverage",
		}, {
			Name: "test_empty_sensitive",
			Val:  "TestEmptySensitive",
		}, {
			Name: "test_evict_control",
			Val:  "TestEvictControl",
		}, {
			Name: "test_evict_control_transient",
			Val:  "TestEvictControlTransient",
		}, {
			Name: "test_export_attribs",
			Val:  "TestExportAttribs",
		}, {
			Name: "test_external_key_import",
			Val:  "TestExternalKeyImport",
		}, {
			Name: "test_firmware_read",
			Val:  "TestFirmwareRead",
		}, {
			Name: "test_gen_new_symm",
			Val:  "TestGenNewSymm",
		}, {
			Name: "test_get_capability_context",
			Val:  "TestGetCapabilityContext",
		}, {
			Name: "test_get_capability_pcr",
			Val:  "TestGetCapabilityPCR",
		}, {
			Name: "test_get_cap_commands",
			Val:  "TestGetCapCommands",
		}, {
			Name: "test_get_cap_coverage",
			Val:  "TestGetCapCoverage",
		}, {
			Name: "test_get_random",
			Val:  "TestGetRandom",
		}, {
			Name: "test_get_test_results",
			Val:  "TestGetTestResults",
		}, {
			Name: "test_gratuitous_auth",
			Val:  "TestGratuitousAuth",
		}, {
			Name: "test_hash",
			Val:  "TestHash",
		}, {
			Name: "test_hierarchy_control_owner_access",
			Val:  "TestHierarchyControlOwnerAccess",
		}, {
			Name: "test_hmac2",
			Val:  "TestHmac2",
		}, {
			Name: "test_hmac_errors",
			Val:  "TestHmacErrors",
		}, {
			Name: "test_hmac_pub_priv_mismatch",
			Val:  "TestHmacPubPrivMismatch",
		}, {
			Name: "test_hmac_quote",
			Val:  "TestHmacQuote",
		}, {
			Name: "test_hmac_session",
			Val:  "TestHmacSession",
		}, {
			Name: "test_hmac_signing",
			Val:  "TestHmacSigning",
		}, {
			Name: "test_import",
			Val:  "TestImport",
		}, {
			Name: "test_import_data_object",
			Val:  "TestImportDataObject",
		}, {
			Name: "test_import_ecdh",
			Val:  "TestImportEcdh",
		}, {
			Name: "test_import_export",
			Val:  "TestImportExport",
		}, {
			Name: "test_inner_wrapper_dup",
			Val:  "TestInnerWrapperDup",
		}, {
			Name: "test_key_creation",
			Val:  "TestKeyCreation",
		}, {
			Name: "test_key_gen",
			Val:  "TestKeyGen",
		}, {
			Name: "test_load_errors",
			Val:  "TestLoadErrors",
		}, {
			Name: "test_normalized_policies",
			Val:  "TestNormalizedPolicies",
		}, {
			Name: "test_nv",
			Val:  "TestNv",
		}, {
			Name: "test_nv2",
			Val:  "TestNv2",
		}, {
			Name: "test_nv3",
			Val:  "TestNv3",
		}, {
			Name: "test_nv_counter",
			Val:  "TestNvCounter",
		}, {
			Name: "test_nv_counter2",
			Val:  "TestNvCounter2",
		}, {
			Name: "test_nv_define_space",
			Val:  "TestNvDefineSpace",
		}, {
			Name: "test_nv_extend",
			Val:  "TestNvExtend",
		}, {
			Name: "test_nv_owner_clear",
			Val:  "TestNvOwnerClear",
		}, {
			Name: "test_nv_partial_write",
			Val:  "TestNvPartialWrite",
		}, {
			Name: "test_nv_set_bits",
			Val:  "TestNvSetBits",
		}, {
			Name: "test_nv_stress",
			Val:  "TestNvStress",
		}, {
			Name: "test_nv_with_policy",
			Val:  "TestNvWithPolicy",
		}, {
			Name: "test_patch_410162",
			Val:  "TestPatch_410162",
		}, {
			Name: "test_patch_411165",
			Val:  "TestPatch_411165",
		}, {
			Name: "test_patch__r88_336067",
			Val:  "TestPatch_R88_336067",
		}, {
			Name: "test_patch__r88_374194",
			Val:  "TestPatch_R88_374194",
		}, {
			Name: "test_patch__r88_391785",
			Val:  "TestPatch_R88_391785",
		}, {
			Name: "test_patch__r88_392304",
			Val:  "TestPatch_R88_392304",
		}, {
			Name: "test_patch__r88_398735",
			Val:  "TestPatch_R88_398735",
		}, {
			Name: "test_patch__r88_405740",
			Val:  "TestPatch_R88_405740",
		}, {
			Name: "test_patch__r88_898154",
			Val:  "TestPatch_R88_898154",
		}, {
			Name: "test_persistent_transient_object",
			Val:  "TestPersistentTransientObject",
		}, {
			Name: "test_pin",
			Val:  "TestPin",
		}, {
			Name: "test_plaintext_import_export",
			Val:  "TestPlaintextImportExport",
		}, {
			Name: "test_policy_auth",
			Val:  "TestPolicyAuth",
		}, {
			Name: "test_policy_auth_bound",
			Val:  "TestPolicyAuthBound",
		}, {
			Name: "test_policy_auth_bound_da",
			Val:  "TestPolicyAuthBoundDA",
		}, {
			Name: "test_policy_pcr_raw_commands",
			Val:  "TestPolicyPCRRawCommands",
		}, {
			Name: "test_policy_secret",
			Val:  "TestPolicySecret",
		}, {
			Name: "test_policy_secret_bad_auth",
			Val:  "TestPolicySecretBadAuth",
		}, {
			Name: "test_policy_signed",
			Val:  "TestPolicySigned",
		}, {
			Name: "test_public_private_mismatch_ecc",
			Val:  "TestPublicPrivateMismatchEcc",
		}, {
			Name: "test_public_private_mismatch_rsa",
			Val:  "TestPublicPrivateMismatchRsa",
		}, {
			Name: "test_public_private_mismatch_sym",
			Val:  "TestPublicPrivateMismatchSym",
		}, {
			Name: "test_pub_only_bind_object",
			Val:  "TestPubOnlyBindObject",
		}, {
			Name: "test_random",
			Val:  "TestRandom",
		}, {
			Name: "test_raw_certify_creation",
			Val:  "TestRawCertifyCreation",
		}, {
			Name: "test_raw_nv_define_space",
			Val:  "TestRawNVDefineSpace",
		}, {
			Name: "test_raw_nv_increment",
			Val:  "TestRawNVIncrement",
		}, {
			Name: "test_raw_nv_read",
			Val:  "TestRawNVRead",
		}, {
			Name: "test_raw_nv_read_lock",
			Val:  "TestRawNVReadLock",
		}, {
			Name: "test_raw_nv_read_public",
			Val:  "TestRawNVReadPublic",
		}, {
			Name: "test_raw_nv_undefine_space",
			Val:  "TestRawNVUndefineSpace",
		}, {
			Name: "test_raw_nv_write",
			Val:  "TestRawNVWrite",
		}, {
			Name: "test_raw_sign",
			Val:  "TestRawSign",
		}, {
			Name: "test_reserved_bits",
			Val:  "TestReservedBits",
		}, {
			Name: "test_rsaes",
			Val:  "TestRsaes",
		}, {
			Name: "test_rsa_exponents",
			Val:  "TestRsaExponents",
		}, {
			Name: "test_rsa_extrema",
			Val:  "TestRsaExtrema",
		}, {
			Name: "test_rsa_sign_pss",
			Val:  "TestRsaSignPss",
		}, {
			Name: "test_salted_auth_session_errors",
			Val:  "TestSaltedAuthSessionErrors",
		}, {
			Name: "test_salted_auth_session_sym",
			Val:  "TestSaltedAuthSessionSym",
		}, {
			Name: "test_salted_auth_session_xor",
			Val:  "TestSaltedAuthSessionXor",
		}, {
			Name: "test_salted_bound_auth_session",
			Val:  "TestSaltedBoundAuthSession",
		}, {
			Name: "test_seal_unseal",
			Val:  "TestSealUnseal",
		}, {
			Name: "test_seal_unseal_persisted_object",
			Val:  "TestSealUnsealPersistedObject",
		}, {
			Name: "test_self_test",
			Val:  "TestSelfTest",
		}, {
			Name: "test_serialization",
			Val:  "TestSerialization",
		}, {
			Name: "test_session_encryption",
			Val:  "TestSessionEncryption",
		}, {
			Name: "test_session_encryption_other_algs",
			Val:  "TestSessionEncryptionOtherAlgs",
		}, {
			Name: "test_sign",
			Val:  "TestSign",
		}, {
			Name: "test_sign_object_attributes",
			Val:  "TestSignObjectAttributes",
		}, {
			Name: "test_sign_restricted",
			Val:  "TestSignRestricted",
		}, {
			Name: "test_sign_verify",
			Val:  "TestSignVerify",
		}, {
			Name: "test_simple_duplicate_import",
			Val:  "TestSimpleDuplicateImport",
		}, {
			Name: "test_simple_duplicate_import_rsa",
			Val:  "TestSimpleDuplicateImportRsa",
		}, {
			Name: "test_simple_sign",
			Val:  "TestSimpleSign",
		}, {
			Name: "test_start_auth_session_bound",
			Val:  "TestStartAuthSessionBound",
		}, {
			Name: "test_start_auth_session_hmac",
			Val:  "TestStartAuthSessionHMAC",
		}, {
			Name: "test_start_auth_session_max_session",
			Val:  "TestStartAuthSessionMaxSession",
		}, {
			Name: "test_start_auth_session_policy",
			Val:  "TestStartAuthSessionPolicy",
		}, {
			Name: "test_start_auth_session_seed",
			Val:  "TestStartAuthSessionSeed",
		}, {
			Name: "test_sw_signing",
			Val:  "TestSWSigning",
		}, {
			Name: "test_sym_parent",
			Val:  "TestSymParent",
		}, {
			Name: "test_sym_primary",
			Val:  "TestSymPrimary",
		}, {
			Name: "test_temporary_object",
			Val:  "TestTemporaryObject",
		}, {
			Name: "test_test_parms",
			Val:  "TestTestParms",
		}, {
			Name: "test_vendor_specific",
			Val:  "TestVendorSpecific",
		},
		},
	})
}

func Ti50TCG(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting ti50 with SPI straps")
	b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusSpi, ti50.CcdDisconnected, ti50.FfClamshell)

	s.Log("Starting TCG tests")
	testSuite := s.Param().(string)

	err := b.RunTcgTests(ctx, s.OutDir(), testSuite)

	// The test starts with a GSC power-on reset. If sysinfo shows GSC did a
	// hard reset, that means GSC crashed during the test.
	sysinfo, sysinfoErr := i.Sysinfo(ctx)
	if sysinfoErr != nil {
		s.Error("Failed to get sysinfo", sysinfoErr)
	} else if sysinfo.ResetFlags&ti50.GscResetFlagHard != 0 {
		s.Error("GSC crashed during test")
	}

	th.MustSucceed(err, "Tests failed.")
}
