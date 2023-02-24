// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

// All well known gpio straps for different form factors in ti50
const (
	// FfTablet puts ti50 image into Tablet form factor mode via strapping resistors
	FfTablet GpioStrap = "TI50_FF_TABLET"
	// FfClamshell puts ti50 image into Clamshell form factor mode via strapping resistors
	FfClamshell GpioStrap = "TI50_FF_CLAMSHELL"
	// FfBox puts ti50 image into Box form factor mode via strapping resistors
	FfBox GpioStrap = "TI50_FF_BOX"
)

// All well known gpio straps for tpm bus mode
const (
	// TpmSpi boots ti50 image for TPM communication via SPI bus
	TpmSpi GpioStrap = "TI50_TPM_SPI"
	// TpmI2c boots ti50 image for TPM communication via I2C bus
	TpmI2c GpioStrap = "TI50_TPM_I2C"
)

// All well known gpio names for ti50 image
const (
	// GpioTi50ResetL is reset pin to GSC (active low)
	GpioTi50ResetL GpioName = "RESET"
	// GpioTi50PltRstL is the PLT reset signal to GSC (active low)
	GpioTi50PltRstL GpioName = "PLT_RST_L"
	// GpioTi50EcRstL is the EC reset signal (active low)
	GpioTi50EcRstL GpioName = "EC_RST_ODL"
	// GpioTi50EcRstFet is the controls a FET to drive the EC reset signal (high means reset)
	GpioTi50EcRstFet GpioName = "EC_RST_FET_ODL"
	// GpioTi50PowerBtnL is the power signal to GSC (active low)
	GpioTi50PowerBtnL GpioName = "PWR_BTN_L"
	// GpioTi50EcPowerBtnL is the power signal from GSC to EC (active low)
	GpioTi50EcPowerBtnL GpioName = "EC_PWR_BTN_L"
	// GpioTi50Kso2 is the KSO (column) from GSC to KB
	GpioTi50Kso2 GpioName = "KSO_02"
	// GpioTi50EcKso2Inv is the KSO (column) from EC to GSC that should be forward through GSC. This active high though
	GpioTi50EcKso2Inv GpioName = "EC_KSO_02_INV"
	// GpioTi50KsiRefresh is the KSI (row) from KB to GSC that is connected to Refresh
	GpioTi50KsiRefresh GpioName = "KSI_02"
	// GpioTi50KsiVolumeUp is the KSI (row) from KB to GSC that is connected to Volume Up
	GpioTi50KsiVolumeUp GpioName = "KSI_02"
	// GpioTi50KsiRecovery is the KSI (row) from KB to GSC that is connected to the Recovery Button
	GpioTi50KsiRecovery GpioName = "KSI_02"
	// GpioTi50KsiBack is the KSI (row) from KB to GSC that is connected to ChromeOS Back key
	GpioTi50KsiBack GpioName = "KSI_00"
)
