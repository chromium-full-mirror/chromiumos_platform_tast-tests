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

// All well known gpio straps for CCD connection status
const (
	// CcdDisconnected is the default, no CCD cable connected
	CcdDisconnected GpioStrap = "CCD_DISCONNECTED"
	// CcdSuzyQ represents a SuzyQ
	CcdSuzyQ GpioStrap = "CCD_SUZYQ"
	// CcdSuzyQFlipped represents a SuzyQ connected upside-down (non-functional)
	CcdSuzyQFlipped GpioStrap = "CCD_SUZYQ_FLIPPED"
	// CcdServo represents a ServoV4 USB-C
	CcdServo GpioStrap = "CCD_SERVO"
	// CcdServoFlipped represents a ServoV4 USB-C connected upside-down (servo may be able to
	// cross the D+/D- signal wires to enable functionality)
	CcdServoFlipped GpioStrap = "CCD_SERVO_FLIPPED"
	// CcdServoSnk1 represents a ServoV4 SUB-C connected while the ChromeOS device acting as
	// the power source.
	CcdServoSnk1 GpioStrap = "CCD_SERVO_SNK1"
	// CcdServoSnk2 represents a ServoV4 SUB-C connected while the ChromeOS device acting as
	// the power source (1.5A).
	CcdServoSnk2 GpioStrap = "CCD_SERVO_SNK2"
	// CcdServoSnk3 represents a ServoV4 SUB-C connected while the ChromeOS device acting as
	// the power source (3A).
	CcdServoSnk3 GpioStrap = "CCD_SERVO_SNK3"
)

const (
	// ServoMicroDisconnected represents simulating that a Servo Micro is not connected.
	ServoMicroDisconnected GpioStrap = "SERVO_MICRO_DISCONNECTED"

	// ServoMicroConnected represents simulating that a Servo Micro is connected.
	ServoMicroConnected GpioStrap = "SERVO_MICRO_CONNECTED"
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
	// GpioTi50VolDownOut from GSC (in Tablet Mode)
	GpioTi50VolDownOut GpioName = "KSO_02"
	// GpioTi50EcKso2Inv is the KSO (column) from EC to GSC that should be forward through GSC.
	// This is active high though.
	GpioTi50EcKso2Inv GpioName = "EC_KSO_02_INV"
	// GpioTi50VolDownIn is volume down input to GSC that should forward through GSC in Tablet Mode
	GpioTi50VolDownIn GpioName = "EC_KSO_02_INV"
	// GpioTi50KsiRefresh is the KSI (row) from KB to GSC that is connected to Refresh
	GpioTi50KsiRefresh GpioName = "KSI_02"
	// GpioTi50VolumeUpIn is volume up input to GSC that should forward through GSC in Tablet Mode
	GpioTi50VolUpIn GpioName = "KSI_02"
	// GpioTi50RecoveryIn is recovery mode switch to GSC that should forward through GSC in Box Mode
	GpioTi50RecoveryIn GpioName = "KSI_02"
	// GpioTi50VolumeUpOut is volume up output from GSC while in Tablet Mode
	GpioTi50VolUpOut GpioName = "EC_KSI_02"
	// GpioTi50RecoveryOut is recovery mode switch output from GSC while in Box Mode
	GpioTi50RecoveryOut GpioName = "EC_KSI_02"
	// GpioTi50KsiBack is the KSI (row) from KB to GSC that is connected to ChromeOS Back key
	GpioTi50KsiBack GpioName = "KSI_00"
	// GpioTi50EcPacketMode is the pin that EC drives high when it is sending packet information
	GpioTi50EcPacketMode GpioName = "EC_GSC_PACKET_MODE"
	// GpioTi50ChassisOpen is the pin that GSC reads to know if end-user has physical access
	GpioTi50ChassisOpen GpioName = "CHASSIS_OPEN"
)
