// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

// FormFactorStrap enumerates all of the different form factor strap settings for ti50 fw image
type FormFactorStrap string

const (
	// FfTablet puts ti50 image into Tablet form factor mode via strapping resistors
	FfTablet FormFactorStrap = "TI50_FF_TABLET"
	// FfClamshell puts ti50 image into Clamshell form factor mode via strapping resistors
	FfClamshell FormFactorStrap = "TI50_FF_CLAMSHELL"
	// FfBox puts ti50 image into Box form factor mode via strapping resistors
	FfBox FormFactorStrap = "TI50_FF_BOX"
)

// StrapName returns the string the Open Titan Tool uses to interact with the gpio strap
func (f FormFactorStrap) StrapName() string {
	return string(f)
}

// TpmBusStrap enumerates the two different TPM busses supported: SPI and I2C
type TpmBusStrap string

const (
	// TpmSpi boots ti50 image for TPM communication via SPI bus
	TpmSpi TpmBusStrap = "TI50_TPM_SPI"
	// TpmI2c boots ti50 image for TPM communication via I2C bus
	TpmI2c TpmBusStrap = "TI50_TPM_I2C"
)

// StrapName returns the string the Open Titan Tool uses to interact with the gpio strap
func (f TpmBusStrap) StrapName() string {
	return string(f)
}

// GpioTi50 enumerates Ti50 FW GPIOs that can either be read or written to
type GpioTi50 string

const (
	// GpioTi50ResetL is reset pin to GSC (active low)
	GpioTi50ResetL GpioTi50 = "RESET"
	// GpioTi50PltRstL is the PLT reset signal to GSC (active low)
	GpioTi50PltRstL GpioTi50 = "PLT_RST_L"
	// GpioTi50EcRstL is the EC reset signal (active low)
	GpioTi50EcRstL GpioTi50 = "EC_RST_ODL"
	// GpioTi50EcRstFet is the controls a FET to drive the EC reset signal (high means reset)
	GpioTi50EcRstFet GpioTi50 = "EC_RST_FET_ODL"
	// GpioTi50PowerBtnL is the power signal to GSC (active low)
	GpioTi50PowerBtnL GpioTi50 = "PWR_BTN_L"
	// GpioTi50EcPowerBtnL is the power signal from GSC to EC (active low)
	GpioTi50EcPowerBtnL GpioTi50 = "EC_PWR_BTN_L"
	// GpioTi50Kso2 is the KSO (column) from GSC to KB
	GpioTi50Kso2 GpioTi50 = "KSO_02"
	// GpioTi50EcKso2Inv is the KSO (column) from EC to GSC that should be forward through GSC. This active high though
	GpioTi50EcKso2Inv GpioTi50 = "EC_KSO_02_INV"
	// GpioTi50KsiRefresh is the KSI (row) from KB to GSC that is connected to Refresh
	GpioTi50KsiRefresh GpioTi50 = "KSI_02"
	// GpioTi50KsiVolumeUp is the KSI (row) from KB to GSC that is connected to Volume Up
	GpioTi50KsiVolumeUp GpioTi50 = "KSI_02"
	// GpioTi50KsiRecovery is the KSI (row) from KB to GSC that is connected to the Recovery Button
	GpioTi50KsiRecovery GpioTi50 = "KSI_02"
	// GpioTi50KsiBack is the KSI (row) from KB to GSC that is connected to ChromeOS Back key
	GpioTi50KsiBack GpioTi50 = "KSI_00"
)

// GpioName returns the string that Open Titan Tool uses to interact with the gpio
func (g GpioTi50) GpioName() string {
	return string(g)
}
