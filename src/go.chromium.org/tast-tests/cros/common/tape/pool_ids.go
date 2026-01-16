// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

// PoolIds for managed owned test accounts.
const (
	ArcEnterpriseLoginManaged3ppFalse                   = "arc_enterprise_login_managed_3pp_false"
	ArcEnterpriseLoginManaged3ppTrue                    = "arc_enterprise_login_managed_3pp_true"
	ArcEnterpriseLoginManagedNecktieFalse               = "arc_enterprise_login_managed_necktie_false"
	ArcEnterpriseLoginManagedNecktieTrue                = "arc_enterprise_login_managed_necktie_true"
	ArcDataMigrationManaged                             = "arc_data_migration_managed"
	ArcSnapshot                                         = "arc_snapshot"
	ArcLoggingTest                                      = "arc_logging_test"
	BuiltInCertProvisioningTesting                      = "built_in_cert_provisioning_testing"
	BuiltInCertProvisioningAPITesting                   = "built_in_cert_provisioning_api_testing"
	BuiltInCertProvisioningAPITesting5                  = "built_in_cert_provisioning_api_testing_5"
	BuiltInCertProvisioningAPITesting6                  = "built_in_cert_provisioning_api_testing_6"
	BuiltInCertProvisioningAPITesting7                  = "built_in_cert_provisioning_api_testing_7"
	BuiltInCertProvisioningAPITesting8                  = "built_in_cert_provisioning_api_testing_8"
	BuiltInCertProvisioningAPITesting9                  = "built_in_cert_provisioning_api_testing_9"
	BuiltInCertProvisioningAPITesting10                 = "built_in_cert_provisioning_api_testing_10"
	BuiltInCertProvisioningAPITesting11                 = "built_in_cert_provisioning_api_testing_11"
	BuiltInCertProvisioningAPITesting12                 = "built_in_cert_provisioning_api_testing_12"
	BuiltInCertProvisioningAPITesting13                 = "built_in_cert_provisioning_api_testing_13"
	BuiltInCertProvisioningAPITesting14                 = "built_in_cert_provisioning_api_testing_14"
	ChromeosbytebotCom                                  = "chromeosbytebot_com"
	Crosprqa4Com                                        = "crosprqa4_com"
	DefaultManaged                                      = "default_managed"
	DemoMode                                            = "demo_mode"
	DeviceTrustDisabled                                 = "device_trust_disabled"
	DeviceTrustEnabled                                  = "device_trust_enabled"
	EnterpriseConnectorsMGSAshWebProtectDisabled        = "ash_gaia_mgs_auto_launch_wp_disabled"
	EnterpriseConnectorsMGSAshWebProtectEnabledAllow    = "ash_gaia_mgs_auto_launch_wp_enabled_allow"
	EnterpriseConnectorsMGSAshWebProtectEnabledBlock    = "ash_gaia_mgs_auto_launch_wp_enabled_block"
	EnterpriseConnectorsMGSLacrosWebProtectDisabled     = "lacros_gaia_mgs_auto_launch_wp_disabled"
	EnterpriseConnectorsMGSLacrosWebProtectEnabledAllow = "lacros_gaia_mgs_auto_launch_wp_enabled_allow"
	EnterpriseConnectorsMGSLacrosWebProtectEnabledBlock = "lacros_gaia_mgs_auto_launch_wp_enabled_block"
	Enrollment                                          = "enrollment"
	EnrollmentKiosk                                     = "enrollment_kiosk"
	EnrollmentSAML                                      = "enrollment_saml"
	FREManual                                           = "fre_manual"
	FREAutomated                                        = "fre_automated"
	ImprivataSharedKiosk                                = "imprivata_shared_kiosk"
	ImprivataSingleUser                                 = "imprivata_single_user"
	LogUploadEnabled                                    = "log_upload_enabled"
	Reporting                                           = "reporting"
	ZTETestAutomation                                   = "zte-test-automation"
)

// PoolIds for DMA owned test accounts.
const (
	DmaDefaultManaged                    = "dma_default_managed"
	DmaBuiltInCertProvisioningTesting    = "dma_built_in_cert_provisioning_testing"
	DmaBuiltInCertProvisioningAPITesting = "dma_built_in_cert_provisioning_api_testing"
	DmaDeviceTrustDisabled               = "dma_device_trust_disabled"
	DmaDeviceTrustEnabled                = "dma_device_trust_enabled"
	DmaChromeosbytebotCom                = "dma_chromeosbytebot_com"
	DmaLogUploadEnabled                  = "dma_log_upload_enabled"
)

// PoolIds for unmanaged owned test accounts.
const (
	// For `accountmanager` package.
	AccountManager                          = "account_manager"
	ArcDataMigrationUnmanaged               = "arc_data_migration_unmanaged"
	ArcEnterpriseLoginManagedUnmanagedFalse = "arc_enterprise_login_managed_unmanaged_false"
	ArcEnterpriseLoginManagedUnmanagedTrue  = "arc_enterprise_login_managed_unmanaged_true"
	CrossDeviceFastPair                     = "crossdevicefastpair"
	DefaultUnmanaged                        = "default_unmanaged"
	UIDefault                               = "ui_default"
)

// PoolIds for Citrix accounts.
const (
	Citrix = "citrix"
)
