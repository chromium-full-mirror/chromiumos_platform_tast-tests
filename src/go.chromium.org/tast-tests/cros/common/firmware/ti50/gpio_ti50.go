// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

// All well known gpio straps for different form factors in ti50.
const (
	// FfTablet puts ti50 image into Tablet form factor mode via strapping
	// resistors.
	FfTablet GpioStrap = "TI50_FF_TABLET"
	// FfClamshell puts ti50 image into Clamshell form factor mode via strapping
	// resistors.
	FfClamshell GpioStrap = "TI50_FF_CLAMSHELL"
	// FfBox puts ti50 image into Box form factor mode via strapping resistors.
	FfBox GpioStrap = "TI50_FF_BOX"
)

// Gpio straps used to setup TPM communication for each type of bus.
const (
	// TpmSpi configures GSC straps to select TPM communication via SPI bus.
	TpmSpi GpioStrap = "TI50_TPM_SPI"
	// TpmI2c configures GSC straps to select TPM communication via I2C bus.
	TpmI2c GpioStrap = "TI50_TPM_I2C"
	// ApOnSpi simulates the AP turning on and sets up the I2C bus for TPM
	// communication.
	ApOnI2c GpioStrap = "AP_ON_I2C"
	// ApOnSpi simulates the AP turning on and sets up the SPI bus for TPM
	// communication.
	ApOnSpi GpioStrap = "AP_ON_SPI"
	// ApOff simulates the AP turning off. Disconnect I2C and SPI.
	ApOff GpioStrap = "AP_OFF"
)

// Combined gpio straps.
const (
	// StrapReset puts and holds the GSC in reset.
	StrapReset GpioStrap = "RESET"
)

// All well known gpio straps for CCD connection status.
const (
	// CcdDisconnected is the default, no CCD cable connected.
	CcdDisconnected GpioStrap = "CCD_DISCONNECTED"
	// CcdSuzyQ represents a SuzyQ.
	CcdSuzyQ GpioStrap = "CCD_SUZYQ"
	// CcdSuzyQFlipped represents a SuzyQ connected upside-down (non-functional).
	CcdSuzyQFlipped GpioStrap = "CCD_SUZYQ_FLIPPED"
	// CcdServo represents a ServoV4 USB-C
	CcdServo GpioStrap = "CCD_SERVO"
	// CcdServoFlipped represents a ServoV4 USB-C connected upside-down (servo may
	// be able to cross the D+/D- signal wires to enable functionality).
	CcdServoFlipped GpioStrap = "CCD_SERVO_FLIPPED"
	// CcdServoSnk1 represents a ServoV4 SUB-C connected while the ChromeOS device
	// acting as the power source.
	CcdServoSnk1 GpioStrap = "CCD_SERVO_SNK1"
	// CcdServoSnk2 represents a ServoV4 SUB-C connected while the ChromeOS device
	// acting as  the power source (1.5A).
	CcdServoSnk2 GpioStrap = "CCD_SERVO_SNK2"
	// CcdServoSnk3 represents a ServoV4 SUB-C connected while the ChromeOS device
	// acting as the power source (3A).
	CcdServoSnk3 GpioStrap = "CCD_SERVO_SNK3"
)

const (
	// ServoMicroDisconnected represents simulating that a Servo Micro is not
	// connected.
	ServoMicroDisconnected GpioStrap = "SERVO_MICRO_DISCONNECTED"

	// ServoMicroConnected represents simulating that a Servo Micro is connected.
	ServoMicroConnected GpioStrap = "SERVO_MICRO_CONNECTED"
)

// All well known gpio names for ti50 image.
const (
	// GpioTi50ResetL is reset pin to GSC (active low).
	GpioTi50ResetL GpioName = "RESET"
	// GpioTi50SysRstL is the SYS reset signal to the AP (active low).
	GpioTi50SysRstL GpioName = "SYS_RST_L"
	// GpioTi50PltRstL is the PLT reset signal to GSC (active low).
	GpioTi50PltRstL GpioName = "PLT_RST_L"
	// GpioTi50EcRstL is the EC reset signal (active low).
	GpioTi50EcRstL GpioName = "EC_RST_ODL"
	// GpioTi50EcRstFet is the controls a FET to drive the EC reset signal
	// (high means reset).
	GpioTi50EcRstFet GpioName = "EC_RST_FET_ODL"
	// GpioTi50PowerBtnL is the power signal to GSC (active low).
	GpioTi50PowerBtnL GpioName = "PWR_BTN_L"
	// GpioTi50EcPowerBtnL is the power signal from GSC to EC (active low).
	GpioTi50EcPowerBtnL GpioName = "EC_PWR_BTN_L"
	// GpioTi50Kso2 is the KSO (column) from GSC to KB (active low).
	GpioTi50Kso2 GpioName = "KSO_02"
	// GpioTi50VolDownOut from GSC in Tablet Mode (active low).
	GpioTi50VolDownOut GpioName = "KSO_02"
	// GpioTi50EcKso2Inv is the KSO (column) from EC to GSC that should be forward
	// through GSC (active high).
	GpioTi50EcKso2Inv GpioName = "EC_KSO_02_INV"
	// GpioTi50VolDownIn is volume down input to GSC that should forward through
	// GSC in Tablet Mode (active low).
	GpioTi50VolDownIn GpioName = "EC_KSO_02_INV"
	// GpioTi50KsiRefresh is the KSI (row) from KB to GSC that is connected to
	// Refresh (active low).
	GpioTi50KsiRefresh GpioName = "KSI_02"
	// GpioTi50VolumeUpIn is volume up input to GSC that should forward through
	// GSC in Tablet Mode (active low).
	GpioTi50VolUpIn GpioName = "KSI_02"
	// GpioTi50RecoveryIn is recovery mode switch to GSC that should forward
	// through GSC in Box Mode (active low).
	GpioTi50RecoveryIn GpioName = "KSI_02"
	// GpioTi50VolumeUpOut is volume up output from GSC while in Tablet Mode
	// (active low).
	GpioTi50VolUpOut GpioName = "EC_KSI_02"
	// GpioTi50RecoveryOut is recovery mode switch output from GSC while in Box
	// Mode (active low).
	GpioTi50RecoveryOut GpioName = "EC_KSI_02"
	// GpioTi50KsiBack is the KSI (row) from KB to GSC that is connected to
	// ChromeOS Back key (active low).
	GpioTi50KsiBack GpioName = "KSI_00"
	// GpioTi50EcPacketMode is the pin that EC drives high when it is sending
	// packet information (active high).
	GpioTi50EcPacketMode GpioName = "EC_GSC_PACKET_MODE"
	// GpioTi50ChassisOpen is the pin that GSC reads to know if end-user has
	// physical access (active high).
	GpioTi50ChassisOpen GpioName = "CHASSIS_OPEN"
	// GpioTi50WriteProtectL is the hardware write protect signal that GSC
	// controls (active low).
	GpioTi50WriteProtectL GpioName = "WP_L"
	// GpioTi50WriteProtectSenseL is the hardware write protect signal that GSC
	// reads (active low).
	GpioTi50WriteProtectSenseL GpioName = "WP_SENSE_L"
	// GpioTi50DeviceI2cSda is the SDA signal for I2C TPM communication with the
	// AP.
	GpioTi50DeviceI2cSda GpioName = "DEVICE_I2C_SDA"
	// GpioTi50DeviceI2cScl is the SCL signal for I2C TPM communication with the
	// AP.
	GpioTi50DeviceI2cScl GpioName = "DEVICE_I2C_SCL"
	// GpioTi50LidOpen is the pin that GSC reads to know if the lid is open
	// (active high).
	GpioTi50LidOpen GpioName = "LID_OPEN"
	// GpioTi50ApIntL is the falling edge "TPM ready" signal from GSC to AP.
	GpioTi50ApIntL GpioName = "GSC_AP_INT_L"
	// GpioTi50LidOpen is the pin that GSC reads to know if the lid is open.
	// GpioTi50CcdModeL is the in/out low-active signal for when CCD cable is
	// detected (active low).
	GpioTi50CcdModeL GpioName = "CCD_MODE_ODL"
	// GpioTi50I2cDbgSda is the SDA signal for I2C communication where the GSC is
	// host.
	GpioTi50I2cDbgSda GpioName = "I2C_DBG_SDA"
	// GpioTi50I2cDbgScl is the SCL signal for I2C communication where the GSC is
	// host.
	GpioTi50I2cDbgScl GpioName = "I2C_DBG_SCL"
	// GpioTi50SmbusSda is the SDA signal for I2C/SM Bus communication where the
	// GSC is host.
	GpioTi50SmbusSda GpioName = "SMBUS_SDA"
	// GpioTi50SmbusScl is the SCL signal for I2C/SM Bus communication where the
	// GSC is host.
	GpioTi50SmbusScl GpioName = "SMBUS_SCL"
	// GpioTi50ACPresent is the signal from PMIC indicating if the ChromeOS device
	// is connected to a power source (active high).
	GpioTi50ACPresent GpioName = "AC_PRESENT"
	// GpioTi50BattDisableL is the signal from GSC to battery pack to cutoff
	// the battery's power (active low).
	GpioTi50BattDisableL GpioName = "BATT_DISABLE_L"
	// GpioTi50UartDbgTxGscRx is the Ti50 console input.
	GpioTi50GscUartRx GpioName = "GSC_UART_RX"
	// GpioTi50UartGscTxDbgRx is the Ti50 console output.
	GpioTi50GscUartTx GpioName = "GSC_UART_TX"
	// GpioTi50UartFpmcuTxDbgRx is the UART signal from FPMCU to Ti50.
	GpioTi50UartFpmcuTxDbgRx GpioName = "UART_FPMCU_TX_DBG_RX"
	// GpioTi50UartDbgTxFpmcuRx is the UART signal from Ti50 to FPMCU.
	GpioTi50UartDbgTxFpmcuRx GpioName = "UART_DBG_TX_FPMCU_RX"
	// GpioTi50UartApTxDbgRx is the UART signal from AP to Ti50.
	GpioTi50UartApTxDbgRx GpioName = "UART_AP_TX_DBG_RX"
	// GpioTi50UartDbgTxApRx is the UART signal from Ti50 to AP.
	GpioTi50UartDbgTxApRx GpioName = "UART_DBG_TX_AP_RX"
	// GpioTi50UartEcTxDbgRx is the UART signal from EC to Ti50.
	GpioTi50UartEcTxDbgRx GpioName = "UART_EC_TX_DBG_RX"
	// GpioTi50UartDbgTxEcRx is the UART signal from Ti50 to EC.
	GpioTi50UartDbgTxEcRx GpioName = "UART_DBG_TX_EC_RX"
)

const (
	// I2cTi50Tpm is the I2C bus on which Ti50 is TPM device.
	I2cTi50Tpm I2cBusName = "TPM"
	// I2cTi50Debug is one of the I2C busses on which Ti50 is host, (should use
	// alias, waiting for OpenTitanTool support).
	I2cTi50Debug I2cBusName = "I2C2"
	// I2cTi50Smbus is one of the I2C busses on which Ti50 is host, (should use
	// alias, waiting for OpenTitanTool support).
	I2cTi50Smbus I2cBusName = "I2C3"
	// I2cPAC1Bus is the I2C bus for the PAC195x chip.
	I2cPAC1Bus I2cBusName = "PAC1"
	// I2cPAC2Bus is the I2C bus for the second PAC195x chip on the
	// OpenTitanShield.
	I2cPAC2Bus I2cBusName = "PAC2"
)
