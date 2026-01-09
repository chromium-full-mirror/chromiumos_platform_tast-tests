// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/xmlrpc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// A StringControl contains the name of a gettable/settable Control which takes a string value.
type StringControl string

// These are the Servo controls which can be get/set with a string value.
const (
	ActiveChgPort         StringControl = "active_chg_port"
	ActiveDUTController   StringControl = "active_dut_controller"
	ArbKey                StringControl = "arb_key"
	ArbKeyConfig          StringControl = "arb_key_config"
	ArbKeysConfig         StringControl = "arb_keys_config"
	BottomUSBKeyMux       StringControl = "bottom_usbkey_mux"
	ColdResetSelect       StringControl = "cold_reset_select"
	DUTUSB3EnV4p1         StringControl = "servo_v4p1_dut_usb3_en"
	DUTVoltageMV          StringControl = "dut_voltage_mv"
	Devices               StringControl = "devices"
	DownloadImageToUSBDev StringControl = "download_image_to_usb_dev"
	ECActiveCopy          StringControl = "ec_active_copy"
	FWWPState             StringControl = "fw_wp_state"
	GSCFWWPAtBootState    StringControl = "gsc_fw_wp_atboot_state"
	ImageUSBKeyDev        StringControl = "image_usbkey_dev"
	ImageUSBKeyDirection  StringControl = "image_usbkey_direction"
	ImageUSBKeyPwr        StringControl = "image_usbkey_pwr"
	LidOpen               StringControl = "lid_open"
	PDCommunication       StringControl = "servo_pd_comm"
	PowerState            StringControl = "power_state"
	PwrButtonCtrl         StringControl = "pwr_button"
	SecondUSBKeyDirection StringControl = "second_usbkey_direction"
	ServoDUTSBU1MV        StringControl = "servo_dut_sbu1_mv"
	ServoDUTSBU2MV        StringControl = "servo_dut_sbu2_mv"
	ServoDUTCC1MV         StringControl = "servo_dut_cc1_mv"
	ServoDUTCC2MV         StringControl = "servo_dut_cc2_mv"
	SupportCrosECComm     StringControl = "supports_cros_ec_communication"
	TopUSBKeyMux          StringControl = "top_usbkey_mux"
	Type                  StringControl = "servo_type"
	UARTCmd               StringControl = "servo_uart_cmd"
	UARTRegexp            StringControl = "servo_uart_regexp"
	UARTStream            StringControl = "servo_uart_stream"
	USBArbKey             StringControl = "usb_arb_key"
	USBArbKeyConfig       StringControl = "usb_arb_key_config"
	USBCPolarity          StringControl = "usbc_polarity"
	WarmReset             StringControl = "warm_reset"
	Watchdog              StringControl = "watchdog"
	WatchdogAdd           StringControl = "watchdog_add"
	WatchdogRemove        StringControl = "watchdog_remove"
	PDAdapterSrcCaps      StringControl = "ada_srccaps"

	// DUTConnectionType was previously known as V4Type ("servo_v4_type")
	DUTConnectionType StringControl = "root.dut_connection_type"

	// PDRole was previously known as V4Role ("servo_v4_role")
	PDRole StringControl = "servo_pd_role"
)

// These controls accept only "on" and "off" as values.
const (
	UARTCapture OnOffControl = "servo_uart_capture"
)

// A BoolControl contains the name of a gettable/settable Control which takes a boolean value.
type BoolControl string

// These are the Servo controls which can be get/set with a boolean value.
const (
	ChargerAttached BoolControl = "charger_attached"
)

// An IntControl contains the name of a gettable/settable Control which takes an integer value.
type IntControl string

// These are the Servo controls which can be get/set with an integer value.
const (
	BatteryChargeMAH             IntControl = "battery_charge_mah"
	BatteryCurrentMA             IntControl = "ppvar_vbat_ma"
	BatteryDesignVoltageDesignMV IntControl = "battery_voltage_design_mv"
	BatteryFullDesignMAH         IntControl = "battery_full_design_mah"
	BatteryFullChargeMAH         IntControl = "battery_full_charge_mah"
	BatteryVoltageMV             IntControl = "ppvar_vbat_mv"
	UsbcPr                       IntControl = "usbc_pr"
	VolumeDownHold               IntControl = "volume_down_hold"    // Integer represents a number of milliseconds.
	VolumeUpHold                 IntControl = "volume_up_hold"      // Integer represents a number of milliseconds.
	VolumeUpDownHold             IntControl = "volume_up_down_hold" // Integer represents a number of milliseconds.
)

// A FloatControl contains the name of a gettable/settable Control which takes a floating-point value.
type FloatControl string

// These are the Servo controls with floating-point values.
const (
	ChargerVoltageMV          FloatControl = "ppchg5_mv"
	BatteryTemperatureCelsius FloatControl = "battery_tempc"
	VBusVoltage               FloatControl = "vbus_voltage"
	VBusPower                 FloatControl = "vbus_power"
	UARTCmdTimeout            FloatControl = "servo_uart_timeout"
)

// DefaultUARTCmdTimeout is the value servod uses for UARTCmdTimeout on startup.
const DefaultUARTCmdTimeout float64 = 3

// A OnOffControl accepts either "on" or "off" as a value.
type OnOffControl string

// These controls accept only "on" and "off" as values.
const (
	BottomUSBKeyPwr OnOffControl = "bottom_usbkey_pwr"
	CCDCPUFWSPI     OnOffControl = "ccd_cpu_fw_spi"
	CCDKeepaliveEn  OnOffControl = "ccd_keepalive_en"
	CCDState        OnOffControl = "ccd_state"
	CPUFWSPI        OnOffControl = "cpu_fw_spi"
	ColdReset       OnOffControl = "cold_reset"
	DTSMode         OnOffControl = "servo_dts_mode"
	DutEthPwrEn     OnOffControl = "dut_eth_pwr_en"
	I2CMuxEn        OnOffControl = "i2c_mux_en"
	InitKeyboard    OnOffControl = "init_keyboard"
	RecMode         OnOffControl = "rec_mode"
	TopUSBKeyPwr    OnOffControl = "top_usbkey_pwr"
	USBKeyboard     OnOffControl = "init_usb_keyboard"
)

// An OnOffValue is a string value that would be accepted by an OnOffControl.
type OnOffValue string

// These are the values used by OnOff controls.
const (
	Off OnOffValue = "off"
	On  OnOffValue = "on"
)

// A KeypressControl is a special type of Control which can take either a numerical value or a KeypressDuration.
type KeypressControl StringControl

// These are the Servo controls which can be set with either a numerical value or a KeypressDuration.
const (
	CtrlD        KeypressControl = "ctrl_d"
	CtrlR        KeypressControl = "ctrl_r"
	CtrlS        KeypressControl = "ctrl_s"
	CtrlU        KeypressControl = "ctrl_u"
	CtrlEnter    KeypressControl = "ctrl_enter"
	Ctrl         KeypressControl = "ctrl_key"
	Enter        KeypressControl = "enter_key"
	Refresh      KeypressControl = "refresh_key"
	CtrlRefresh  KeypressControl = "ctrl_refresh_key"
	ImaginaryKey KeypressControl = "imaginary_key"
	SysRQX       KeypressControl = "sysrq_x"
	SysRQR       KeypressControl = "sysrq_r"
	PowerKey     KeypressControl = "power_key"
	Pwrbutton    KeypressControl = "pwr_button"
	USBEnter     KeypressControl = "usb_keyboard_enter_key"
	ArrowUp      KeypressControl = "arrow_up"
	ArrowDown    KeypressControl = "arrow_down"
)

// A KeypressDuration is a string accepted by a KeypressControl.
type KeypressDuration string

// These are string values that can be passed to a KeypressControl.
const (
	DurTab        KeypressDuration = "tab"
	DurPress      KeypressDuration = "press"
	DurShortPress KeypressDuration = "short_press"
	DurLongPress  KeypressDuration = "long_press"
)

// Dur returns a custom duration that can be passed to KeypressWithDuration
func Dur(dur time.Duration) KeypressDuration {
	return KeypressDuration(fmt.Sprintf("%f", dur.Seconds()))
}

// A FWWPStateValue is a string accepted by the FWWPState control.
type FWWPStateValue string

// These are the string values that can be passed to the FWWPState control.
const (
	FWWPStateOff FWWPStateValue = "force_off"
	FWWPStateOn  FWWPStateValue = "force_on"
)

// A LidOpenValue is a string accepted by the LidOpen control.
type LidOpenValue string

// These are the string values that can be passed to the LidOpen control.
const (
	LidOpenYes LidOpenValue = "yes"
	LidOpenNo  LidOpenValue = "no"
)

// A PowerStateValue is a string accepted by the PowerState control.
type PowerStateValue string

// These are the string values that can be passed to the PowerState control.
const (
	PowerStateCR50Reset   PowerStateValue = "cr50_reset"
	PowerStateOff         PowerStateValue = "off"
	PowerStateOn          PowerStateValue = "on"
	PowerStateRec         PowerStateValue = "rec"
	PowerStateRecForceMRC PowerStateValue = "rec_force_mrc"
	PowerStateReset       PowerStateValue = "reset"
	PowerStateWarmReset   PowerStateValue = "warm_reset"
)

// A USBMuxState indicates whether the servo's USB mux is on, and if so, which direction it is powering.
type USBMuxState string

// These are the possible states of the USB mux.
const (
	USBMuxOff  USBMuxState = "off"
	USBMuxDUT  USBMuxState = "dut_sees_usbkey"
	USBMuxHost USBMuxState = "servo_sees_usbkey"
)

// A PDRoleValue is a string that would be accepted by the PDRole control.
type PDRoleValue string

// These are the string values that can be passed to PDRole.
const (
	PDRoleSnk PDRoleValue = "snk"
	PDRoleSrc PDRoleValue = "src"

	// PDRoleNA indicates a non-v4 servo.
	PDRoleNA PDRoleValue = "n/a"
)

// A DUTConnTypeValue is a string that would be returned by the DUTConnectionType control.
type DUTConnTypeValue string

// These are the string values that can be returned by DUTConnectionType
const (
	DUTConnTypeA DUTConnTypeValue = "type-a"
	DUTConnTypeC DUTConnTypeValue = "type-c"

	// DUTConnTypeNA indicates a non-v4 servo.
	DUTConnTypeNA DUTConnTypeValue = "n/a"
)

// A WatchdogValue is a string that would be accepted by WatchdogAdd & WatchdogRemove control.
type WatchdogValue string

// These are the string watchdog type values that can be passed to WatchdogAdd & WatchdogRemove.
const (
	WatchdogCCD  WatchdogValue = "ccd"
	WatchdogMain WatchdogValue = "main"
)

// DUTController is the active controller on a dual mode servo.
type DUTController string

// Parameters that can be passed to SetActiveDUTController().
const (
	DUTControllerC2D2       DUTController = "c2d2"
	DUTControllerCCDCr50    DUTController = "ccd_cr50"
	DUTControllerCCD        DUTController = "ccd_gsc"
	DUTControllerCCDTi50    DUTController = "ccd_ti50"
	DUTControllerServoMicro DUTController = "servo_micro"
)

// A DUTUSB3Value is a string accepted by the dut_usb3 control.
type DUTUSB3Value string

// These are the string values that can be passed to the dut_usb3 control.
const (
	DUTUSB3Enable  DUTUSB3Value = "enable"
	DUTUSB3Disable DUTUSB3Value = "disable"
)

// ServoKeypressDelay comes from hdctools/servo/drv/keyboard_handlers.py.
// It is the minimum time interval between 'press' and 'release' keyboard events.
const ServoKeypressDelay = 100 * time.Millisecond

// SrcCap stores the voltage and current values of a PDO.
type SrcCap struct {
	Voltage int // Voltage in mV
	Current int // Current in mA
}

// HasControl determines whether the Servo being used supports the given control.
func (s *Servo) HasControl(ctx context.Context, ctrl string) (bool, error) {
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("doc", ctrl))
	// If the control exists, doc() should return with no issue.
	if err == nil {
		return true, nil
	}
	// If the control doesn't exist, then doc() should return a fault.
	if _, isFault := err.(xmlrpc.FaultError); isFault {
		return false, nil
	}
	// A non-fault error indicates that something went wrong.
	return false, err
}

// Echo calls the Servo echo method.
func (s *Servo) Echo(ctx context.Context, message string) (string, error) {
	var val string
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("echo", message), &val)
	return val, err
}

// PowerNormalPress calls the Servo power_normal_press method.
func (s *Servo) PowerNormalPress(ctx context.Context) (bool, error) {
	var val bool
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("power_normal_press"), &val)
	return val, err
}

// SetActChgPort enables a charge port on fluffy.
func (s *Servo) SetActChgPort(ctx context.Context, port string) error {
	return s.SetString(ctx, ActiveChgPort, port)
}

// DUTVoltageMV reads the voltage present on the DUT port on fluffy.
func (s *Servo) DUTVoltageMV(ctx context.Context) (string, error) {
	return s.GetString(ctx, DUTVoltageMV)
}

// DUTCCVoltageMV reads the voltage present on the cc lines.
func (s *Servo) DUTCCVoltageMV(ctx context.Context) (string, string, error) {
	var err error = nil
	var cc1V, cc2V string

	if cc1V, err = s.GetString(ctx, ServoDUTCC1MV); err != nil {
		return cc1V, cc2V, err
	}

	if cc2V, err = s.GetString(ctx, ServoDUTCC2MV); err != nil {
		return cc1V, cc2V, err
	}

	return cc1V, cc2V, nil
}

// GetServoVersion gets the version of Servo being used.
func (s *Servo) GetServoVersion(ctx context.Context) (string, error) {
	if s.version != "" {
		return s.version, nil
	}
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get_version"), &s.version)
	return s.version, err
}

// IsServoV4 determines whether the Servo being used is v4.
func (s *Servo) IsServoV4(ctx context.Context) (bool, error) {
	version, err := s.GetServoVersion(ctx)
	if err != nil {
		return false, errors.Wrap(err, "determining servo version")
	}
	return strings.HasPrefix(version, "servo_v4"), nil
}

// GetDUTConnectionType gets the type of connection between the Servo and the DUT.
// If Servo is not V4, returns DUTConnTypeNA.
func (s *Servo) GetDUTConnectionType(ctx context.Context) (DUTConnTypeValue, error) {
	if s.dutConnType != "" {
		return s.dutConnType, nil
	}
	if isV4, err := s.IsServoV4(ctx); err != nil {
		return "", errors.Wrap(err, "determining whether servo is v4")
	} else if !isV4 {
		s.dutConnType = DUTConnTypeNA
		return s.dutConnType, nil
	}
	connType, err := s.GetString(ctx, DUTConnectionType)
	if err != nil {
		return "", err
	}
	s.dutConnType = DUTConnTypeValue(connType)
	return s.dutConnType, nil
}

// GetString returns the value of a specified control.
func (s *Servo) GetString(ctx context.Context, control StringControl) (string, error) {
	var value string
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return "", errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// GetStringTimeout returns the value of a specified control with a custom timeout.
func (s *Servo) GetStringTimeout(ctx context.Context, control StringControl, timeout time.Duration) (string, error) {
	var value string
	if err := s.xmlrpc.Run(
		ctx,
		xmlrpc.NewCallTimeout(
			"get",
			timeout,
			string(control)),
		&value); err != nil {
		return "", errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// GetStringArray returns a result array of strings from a specified control string.
func (s *Servo) GetStringArray(ctx context.Context, control string) ([]string, error) {
	values := make([]string, 0)
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", control), &values); err != nil {
		return []string{}, errors.Wrapf(err, "getting values for servo control %q", control)
	}
	return values, nil
}

// GetServoSerials returns a map of servo serial numbers. Interesting map keys are "ccd", "main", "servo_micro", but there are others also.
func (s *Servo) GetServoSerials(ctx context.Context) (map[string]string, error) {
	value := make(map[string]string)
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get_servo_serials"), &value); err != nil {
		return map[string]string{}, errors.Wrap(err, "getting servo serials")
	}
	return value, nil
}

// GetCCDSerial returns the serial number of the CCD interface.
func (s *Servo) GetCCDSerial(ctx context.Context) (string, error) {
	value, err := s.GetServoSerials(ctx)
	if err != nil {
		return "", err
	}
	ccdSerial, ok := value["ccd"]
	if ok {
		return ccdSerial, nil
	}
	ccdSerial, ok = value["ccd_gsc"]
	if ok {
		return ccdSerial, nil
	}
	ccdSerial, ok = value["ccd_cr50"]
	if ok {
		return ccdSerial, nil
	}
	servoType, err := s.GetServoType(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get servo type")
	}
	// SuzyQ reports as ccd_cr50, and doesn't have an interface named ccd.
	if servoType == "ccd_cr50" {
		ccdSerial, ok := value["main"]
		if ok {
			return ccdSerial, nil
		}
	}
	return "", errors.Errorf("no ccd serial in %q", value)
}

// RemoveCCDWatchdogs enumerates over all servo devices, removes the watchdogs and enables CCDKeepaliveEn for any CCD devices.
func (s *Servo) RemoveCCDWatchdogs(ctx context.Context) error {
	devices, err := s.GetStringListTimeout(ctx, Devices, 20*time.Second)
	if err != nil {
		return err
	}
	didRemove := false
	for _, device := range devices {
		stringType, err := PropertyToString(device, "type")
		if err != nil {
			return err
		}
		testing.ContextLog(ctx, "Removing watchdog: ", stringType)
		if err := s.SetString(ctx, WatchdogRemove, stringType); err != nil {
			return err
		}
		if strings.HasPrefix(stringType, "ccd") {
			if err := s.SetOnOff(ctx, CCDKeepaliveEn, On); err != nil {
				return err
			}
		}
		didRemove = true
	}

	if didRemove {
		// GoBigSleepLint: Removing the watchdog seems to take some time before it works.
		if err := testing.Sleep(ctx, 4*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// GetBool returns the boolean value of a specified control.
func (s *Servo) GetBool(ctx context.Context, control BoolControl) (bool, error) {
	var value bool
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return false, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// GetChargerAttached returns the boolean value to indicate whether charger is attached.
func (s *Servo) GetChargerAttached(ctx context.Context) (bool, error) {
	return s.GetBool(ctx, ChargerAttached)
}

// LidOpenState checks whether DUT's lid is open or closed, and returns yes/no.
func (s *Servo) LidOpenState(ctx context.Context) (string, error) {
	return s.GetString(ctx, LidOpen)
}

// parseUint extracts a hex number from `value` at `*index+1` that is exactly `bits` in length.
// `bits` must be power of 2.
// `*index` will be moved to the end of the extracted runes.
func parseUint(value []rune, index *int, bits int) (rune, error) {
	chars := bits / 4
	endIndex := *index + chars
	if endIndex >= len(value) {
		return 0, errors.Errorf("unparsable escape sequence `\\%s`", string(value[*index:]))
	}
	char, err := strconv.ParseUint(string(value[*index+1:endIndex+1]), 16, bits)
	if err != nil {
		return 0, errors.Wrapf(err, "unparsable escape sequence `\\%s`", string(value[*index:endIndex+1]))
	}
	*index += chars
	return rune(char), nil
}

// parseInt extracts a decimal number from `value` at `*index`.
// `*index` will be moved to the end of the extracted runes.
func parseInt(value []rune, index *int) (int, error) {
	intVal := 0
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		if c >= '0' && c <= '9' {
			intVal *= 10
			intVal += int(c - '0')
		} else {
			(*index)--
			break
		}
	}
	return intVal, nil
}

// parseQuotedStringInternal returns a new string with the quotes and escaped chars from `value` removed, moves `*index` to the index of the closing quote rune.
func parseQuotedStringInternal(value []rune, index *int) (string, error) {
	if *index >= len(value) {
		return "", errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	// The first char should always be a ' or "
	quoteChar := value[*index]
	if quoteChar != '\'' && quoteChar != '"' {
		return "", errors.Errorf("unexpected string char %c at index %d in %s", quoteChar, *index, string(value))
	}
	(*index)++
	var current strings.Builder
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		if c == quoteChar {
			break
		} else if c == '\\' {
			(*index)++
			if *index >= len(value) {
				return "", errors.New("unparsable escape sequence \\")
			}
			switch value[*index] {
			case '"', '\'', '\\':
				current.WriteRune(value[*index])
			case 'r':
				current.WriteRune('\r')
			case 'n':
				current.WriteRune('\n')
			case 't':
				current.WriteRune('\t')
			case 'x':
				r, err := parseUint(value, index, 8)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			case 'u':
				r, err := parseUint(value, index, 16)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			case 'U':
				r, err := parseUint(value, index, 32)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			default:
				return "", errors.Errorf("unexpected escape sequence \\%c at index %d in %s", value[*index], *index, string(value))
			}
		} else {
			current.WriteRune(c)
		}
	}
	return current.String(), nil
}

// parseValueInternal parses `value` as a string, list, map, or integer. Moves `*index` to the end of the value.
func parseValueInternal(value []rune, index *int) (interface{}, error) {
	if *index >= len(value) {
		return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		switch c {
		case '[', '(':
			sublist, err := parseStringListInternal(value, index)
			if err != nil {
				return nil, err
			}
			return sublist, nil
		case '{':
			submap, err := parseStringMapInternal(value, index)
			if err != nil {
				return nil, err
			}
			return submap, nil
		case '\'', '"':
			substr, err := parseQuotedStringInternal(value, index)
			if err != nil {
				return nil, err
			}
			return substr, nil
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			substr, err := parseInt(value, index)
			if err != nil {
				return nil, err
			}
			return substr, nil
		case 'N':
			if string(value[*index:*index+4]) == "None" {
				*index += 3
				return nil, nil
			}
			fallthrough
		default:
			return nil, errors.Errorf("unexpected value char %q at index %d in %s", c, *index, string(value))
		}
	}
	return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
}

// parseStringListInternal parses `value` as a possibly nested list of strings, each quoted and separated by commas. Moves `*index` to the index of the closing ] rune.
func parseStringListInternal(value []rune, index *int) ([]interface{}, error) {
	var result []interface{}
	if *index >= len(value) {
		return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	// The first char should always be a [ or (, as it might be a list or a tuple.
	if value[*index] != '[' && value[*index] != '(' {
		return nil, errors.Errorf("unexpected list char %q at index %d in %s", value[*index], *index, string(value))
	}
	(*index)++
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		switch c {
		case '[', '(', '{', '\'', '"', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'N':
			subval, err := parseValueInternal(value, index)
			if err != nil {
				return nil, err
			}
			result = append(result, subval)
		case ',', ' ', '\n', '\t':
			// Ignore this char
		case ']', ')':
			return result, nil
		default:
			return nil, errors.Errorf("unexpected list member char %q at index %d in %s", c, *index, string(value))
		}
	}
	return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
}

// parseStringMapInternal parses `value` as a map of string:value, each quoted and separated by commas. Moves `*index` to the index of the closing } rune.
func parseStringMapInternal(value []rune, index *int) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if *index >= len(value) {
		return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	// The first char should always be a {
	if value[*index] != '{' {
		return nil, errors.Errorf("unexpected map char %q at index %d in %s", value[*index], *index, string(value))
	}
	(*index)++
	for *index < len(value) {
		// Parse key
		key := ""
	keyLoop:
		for ; *index < len(value); (*index)++ {
			c := value[*index]
			switch c {
			case '\'', '"':
				var err error
				key, err = parseQuotedStringInternal(value, index)
				if err != nil {
					return nil, err
				}
				break keyLoop
			case ' ', '\n', '\t':
				// Ignore this char
			default:
				return nil, errors.Errorf("unexpected map key char %q at index %d in %s", c, *index, string(value))
			}
		}
		(*index)++
		// Parse colon
	colonLoop:
		for ; *index < len(value); (*index)++ {
			c := value[*index]
			switch c {
			case ':':
				break colonLoop
			case ' ', '\n':
				// Ignore this char
			default:
				return nil, errors.Errorf("unexpected map key colon char %q at index %d in %s", c, *index, string(value))
			}
		}
		(*index)++
		// Parse value
	valueLoop:
		for ; *index < len(value); (*index)++ {
			c := value[*index]
			switch c {
			case ' ', '\n', '\t':
				// Ignore this char
			default:
				break valueLoop
			}
		}
		var subVal interface{}
		subVal, err := parseValueInternal(value, index)
		if err != nil {
			return nil, err
		}
		(*index)++
		result[key] = subVal
		// Parse comma
	commaLoop:
		for ; *index < len(value); (*index)++ {
			c := value[*index]
			switch c {
			case ',':
				break commaLoop
			case ' ', '\n', '\t':
				// Ignore this char
			case '}':
				return result, nil
			default:
				return nil, errors.Errorf("unexpected map key comma char %q at index %d in %s", c, *index, string(value))
			}
		}
		(*index)++
	}
	return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
}

// PropertyToString returns the string value assigned to a property
func PropertyToString(property interface{}, name string) (string, error) {
	propertyMap, ok := property.(map[string]interface{})
	if !ok {
		return "", errors.Errorf("failed to cast %+v to map", property)
	}
	propertyRaw, ok := propertyMap[name]
	if !ok {
		return "", errors.Errorf("device has no property %s: %+v", name, property)
	}
	propertyString, ok := propertyRaw.(string)
	if !ok {
		return "", errors.Errorf("failed to cast %+v to string", propertyString)
	}

	return propertyString, nil
}

// ParseStringList parses `value` as a possibly nested list of strings, each quoted and separated by commas.
func ParseStringList(value string) ([]interface{}, error) {
	index := 0
	// Skip over newlines
	for ; index < len(value) && value[index] == '\n'; index++ {
	}
	return parseStringListInternal([]rune(value), &index)
}

// ParseQuotedString returns a new string with the quotes and escaped chars from `value` removed.
func ParseQuotedString(value string) (string, error) {
	index := 0
	return parseQuotedStringInternal([]rune(value), &index)
}

// GetStringList parses the value of a control as an encoded list
func (s *Servo) GetStringList(ctx context.Context, control StringControl) ([]interface{}, error) {
	v, err := s.GetString(ctx, control)
	if err != nil {
		return nil, err
	}
	return ParseStringList(v)
}

// GetStringListTimeout parses the value of a control as an encoded list with a timeout
func (s *Servo) GetStringListTimeout(ctx context.Context, control StringControl, timeout time.Duration) ([]interface{}, error) {
	v, err := s.GetStringTimeout(ctx, control, timeout)
	if err != nil {
		return nil, err
	}
	return ParseStringList(v)
}

// ConvertToStringArrayArray takes a stringList from GetStringList and converts it to [][]string
func ConvertToStringArrayArray(ctx context.Context, stringList []interface{}) ([][]string, error) {
	var ret [][]string
	for i, x := range stringList {
		switch t := x.(type) {
		case string:
			ret = append(ret, []string{t})
		case []string:
			ret = append(ret, t)
		case []interface{}:
			var strings []string
			for _, y := range t {
				strings = append(strings, fmt.Sprint(y))
			}
			ret = append(ret, strings)
		default:
			return nil, errors.Errorf("unexpected type %T at index %d", x, i)
		}
	}
	return ret, nil
}

// GetQuotedString parses the value of a control as a quoted string
func (s *Servo) GetQuotedString(ctx context.Context, control StringControl) (string, error) {
	v, err := s.GetString(ctx, control)
	if err != nil {
		return "", err
	}
	return ParseQuotedString(v)
}

// SetString sets a Servo control to a string value.
func (s *Servo) SetString(ctx context.Context, control StringControl, value string) error {
	// Servo's Set method returns a bool stating whether the call succeeded or not.
	// This is redundant, because a failed call will return an error anyway.
	// So, we can skip unpacking the output.
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("set", string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %q", control, value)
	}
	return nil
}

// SetStringTimeout sets a Servo control to a string value.
func (s *Servo) SetStringTimeout(ctx context.Context, control StringControl, value string, timeout time.Duration) error {
	// Servo's Set method returns a bool stating whether the call succeeded or not.
	// This is redundant, because a failed call will return an error anyway.
	// So, we can skip unpacking the output.
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCallTimeout("set", timeout, string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %q", control, value)
	}
	return nil
}

// SetStringList sets a Servo control to a list of string values.
func (s *Servo) SetStringList(ctx context.Context, control StringControl, values []string) error {
	value := "["
	for i, part := range values {
		if i > 0 {
			value += ", "
		}

		// Escape \ and '
		part = strings.ReplaceAll(part, `\`, `\\`)
		part = strings.ReplaceAll(part, `'`, `\'`)

		// Surround by '
		value += "'" + part + "'"
	}
	value += "]"
	return s.SetString(ctx, control, value)
}

// SetInt sets a Servo control to an integer value.
func (s *Servo) SetInt(ctx context.Context, control IntControl, value int) error {
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("set", string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %d", control, value)
	}
	return nil
}

// GetInt returns the integer value of a specified control.
func (s *Servo) GetInt(ctx context.Context, control IntControl) (int, error) {
	var value int
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return 0, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// GetBatteryChargeMAH returns the battery's charge in mAh.
func (s *Servo) GetBatteryChargeMAH(ctx context.Context) (int, error) {
	return s.GetInt(ctx, BatteryChargeMAH)
}

// GetBatteryFullChargeMAH returns the battery's last full charge in mAh.
func (s *Servo) GetBatteryFullChargeMAH(ctx context.Context) (int, error) {
	return s.GetInt(ctx, BatteryFullChargeMAH)
}

// GetBatteryDesignVoltageDesignMV returns the battery's design voltage in mV
func (s *Servo) GetBatteryDesignVoltageDesignMV(ctx context.Context) (int, error) {
	return s.GetInt(ctx, BatteryDesignVoltageDesignMV)
}

// GetBatteryFullDesignMAH returns the battery's full design capacity in mAh
func (s *Servo) GetBatteryFullDesignMAH(ctx context.Context) (int, error) {
	return s.GetInt(ctx, BatteryFullDesignMAH)
}

// SetFloat sets a Servo control to a float value.
func (s *Servo) SetFloat(ctx context.Context, control FloatControl, value float64) error {
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("set", string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %f", control, value)
	}
	return nil
}

// GetFloat returns the floating-point value of a specified control.
func (s *Servo) GetFloat(ctx context.Context, control FloatControl) (float64, error) {
	var value float64
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return 0, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// SetStringAndCheck sets a control to a specified value, and then verifies that it was set correctly.
// Unless you have a good reason to check, just use SetString.
func (s *Servo) SetStringAndCheck(ctx context.Context, control StringControl, value string) error {
	if err := s.SetString(ctx, control, value); err != nil {
		return err
	}
	if checkedValue, err := s.GetString(ctx, control); err != nil {
		return err
	} else if checkedValue != value {
		return errors.Errorf("after attempting to set %s to %s, checked value was %s", control, value, checkedValue)
	}
	return nil
}

func parseKeypressDuration(value KeypressDuration) (time.Duration, error) {
	// The default duration with SetString is 10s.
	timeout := 10 * time.Second
	// If duration is string (e.g. press, tab, long_press) don't parse as duration string.
	if match := regexp.MustCompile(`\d+(.|\.\d+)?`).FindStringSubmatch(string(value)); match != nil {
		out, err := time.ParseDuration(fmt.Sprintf("%ss", string(value)))
		if err != nil {
			return timeout, errors.Wrap(err, "parsing duration")
		}
		timeout = out + 1*time.Second
	}
	return timeout, nil
}

// KeypressWithDuration sets a KeypressControl to a KeypressDuration value.
func (s *Servo) KeypressWithDuration(ctx context.Context, control KeypressControl, value KeypressDuration) error {
	timeout, err := parseKeypressDuration(value)
	if err != nil {
		return err
	}
	// Set timeout of keypress to make it doesn't timeout before keypress is complete.
	return s.SetStringTimeout(ctx, StringControl(control), string(value), timeout)
}

// PressKey presses an arbitrary key for a KeypressDuration.
// This either uses EC keyboard emulation or the servo's emulated USB keyboard depending on the setting of USBKeyboard.
func (s *Servo) PressKey(ctx context.Context, key string, value KeypressDuration) error {
	timeout, err := parseKeypressDuration(value)
	if err != nil {
		return err
	}
	if err := s.SetString(ctx, ArbKeyConfig, key); err != nil {
		return errors.Wrapf(err, "failed to press key %q", key)
	}
	// Set timeout of keypress so that it doesn't timeout before keypress is complete.
	return s.SetStringTimeout(ctx, ArbKey, string(value), timeout)
}

// PressKeys presses mutiple arbitrary keys for a KeypressDuration.
// This either uses EC keyboard emulation or the servo's emulated USB keyboard depending on the setting of USBKeyboard.
func (s *Servo) PressKeys(ctx context.Context, keys []string, value KeypressDuration) error {
	timeout, err := parseKeypressDuration(value)
	if err != nil {
		return err
	}
	arbKeys := "[\"" + strings.Join(keys, "\",\"") + "\"]"
	if err := s.SetString(ctx, ArbKeysConfig, arbKeys); err != nil {
		return errors.Wrapf(err, "failed to press keys %q", keys)
	}
	// Set timeout of keypress so that it doesn't timeout before keypress is complete.
	return s.SetStringTimeout(ctx, ArbKey, string(value), timeout)
}

// PressUSBKey sends an arbitrary USB KB key for a KeypressDuration.
func (s *Servo) PressUSBKey(ctx context.Context, key string, value KeypressDuration) error {
	timeout, err := parseKeypressDuration(value)
	if err != nil {
		return err
	}
	if err := s.SetString(ctx, USBArbKeyConfig, key); err != nil {
		return errors.Wrapf(err, "failed to press key %q", key)
	}
	// Set timeout of keypress so that it doesn't time out before keypress is complete.
	return s.SetStringTimeout(ctx, USBArbKey, string(value), timeout)
}

// GetUSBMuxState determines whether the servo USB mux is on, and if so, which direction it is pointed.
func (s *Servo) GetUSBMuxState(ctx context.Context) (USBMuxState, error) {
	pwr, err := s.GetString(ctx, ImageUSBKeyPwr)
	if err != nil {
		return "", err
	}
	if pwr == string(USBMuxOff) {
		return USBMuxOff, nil
	}
	direction, err := s.GetString(ctx, ImageUSBKeyDirection)
	if err != nil {
		return "", err
	}
	if direction != string(USBMuxDUT) && direction != string(USBMuxHost) {
		return "", errors.Errorf("%q has an unknown value: %q", ImageUSBKeyDirection, direction)
	}
	return USBMuxState(direction), nil
}

// SetUSBMuxState switches the servo's USB mux to the specified power/direction state.
func (s *Servo) SetUSBMuxState(ctx context.Context, value USBMuxState) error {
	if value == USBMuxOff {
		return s.SetString(ctx, ImageUSBKeyPwr, string(value))
	}
	// Servod ensures the following:
	// * The port is power cycled if it is changing directions
	// * The port ends up in a powered state after this call
	// * If facing the host side, the call only returns once a USB device is detected, or after a generous timeout (10s)
	if err := s.SetStringTimeout(ctx, ImageUSBKeyDirection, string(value), 90*time.Second); err != nil {
		return err
	}
	return nil
}

// SetPowerState sets the PowerState control.
// Because this is particularly disruptive, it is always logged.
// It can be slow, because some boards are configured to hold down the power button for 12 seconds.
func (s *Servo) SetPowerState(ctx context.Context, value PowerStateValue) (retErr error) {
	// Power states that reboot the EC can make servod exit or fail if the CCD watchdog is enabled.
	switch value {
	case PowerStateRec, PowerStateRecForceMRC, PowerStateReset:
		// TODO(b/302370064) C2D2 has a terrible habit of rebooting the GSC during recovery and cold resets
		if _, err := s.GetServoType(ctx); err != nil {
			return errors.Wrap(err, "get servo types")
		}
		if s.hasC2D2 {
			oldVal, err := s.GetString(ctx, ColdResetSelect)
			if err != nil {
				return errors.Wrapf(err, "check %s", ColdResetSelect)
			}
			if err := s.SetString(ctx, ColdResetSelect, string(GSCECResetPulse)); err != nil {
				return errors.Wrapf(err, "set %s", ColdResetSelect)
			}
			defer func() {
				err := s.SetString(ctx, ColdResetSelect, oldVal)
				if err != nil {
					retErr = errors.Join(retErr, errors.Wrapf(err, "restore %s", ColdResetSelect))
				}
			}()
		}
		fallthrough
	case PowerStateCR50Reset, PowerStateWarmReset:
		if err := s.RemoveCCDWatchdogs(ctx); err != nil {
			return errors.Wrap(err, "remove ccd watchdog")
		}
	default:
		// Do nothing
	}

	// TODO:(b/474614687) - servod power_state:cold_reset is flaky on ti50 if the WP has changed
	if s.hasCCD && value == PowerStateReset {
		testing.ContextLog(ctx, "Resetting EC via GSC console")
		if err := s.RunGSCCommand(ctx, "ecrst pulse"); err != nil {
			return errors.Wrap(err, "ecrst pulse failed")
		}
		if err := s.WaitForCCDState(ctx, true, 30*time.Second); err != nil {
			return errors.Wrap(err, "wait for ccd failed")
		}
		return nil
	}
	testing.ContextLogf(ctx, "Setting %q to %q", PowerState, value)
	return s.SetStringTimeout(ctx, PowerState, string(value), 30*time.Second)
}

var srcCapsRe = regexp.MustCompile(`([\d]+)mV\/([\d]+)mA`)

// GetPDAdapterSrcCaps gets the attached charger's source caps
func (s *Servo) GetPDAdapterSrcCaps(ctx context.Context) ([]SrcCap, error) {
	// The ada_srccaps servo control is flaky, just make the console command ourselves.
	var value []SrcCap

	if err := s.RunServoCommand(ctx, "chan save"); err != nil {
		return nil, errors.Wrap(err, "servo console command failed")
	}
	if err := s.RunServoCommand(ctx, "chan 0"); err != nil {
		return nil, errors.Wrap(err, "servo console command failed")
	}
	defer s.RunServoCommand(ctx, "chan restore")
	// Run command on the servo console
	cmdOutput, err := s.RunServoCommandGetOutput(ctx, "ada_srccaps", []string{`ada_srccaps.*> `})
	if err != nil {
		return nil, errors.Wrap(err, "ada_srccaps failed")
	}

	matches := srcCapsRe.FindAllStringSubmatch(cmdOutput[0][0], -1)
	for _, cap := range matches {
		mV, err := strconv.Atoi(cap[1])
		if err != nil {
			return nil, errors.Wrap(err, "voltage capability is not a integer value")
		}
		mA, err := strconv.Atoi(cap[2])
		if err != nil {
			return nil, errors.Wrap(err, "current capability is not a integer value")
		}
		value = append(value, SrcCap{mV, mA})
	}

	if len(value) == 0 {
		return nil, errors.Errorf("ada_srccaps returned no srccaps: %q", cmdOutput[0][0])
	}

	return value, nil
}

// GetDUTSrcCaps gets the attached DUT's source caps
func (s *Servo) GetDUTSrcCaps(ctx context.Context) ([]SrcCap, error) {
	var value []SrcCap

	if err := s.RunServoCommand(ctx, "chan save"); err != nil {
		return nil, errors.Wrap(err, "servo console command failed")
	}
	if err := s.RunServoCommand(ctx, "chan 0"); err != nil {
		return nil, errors.Wrap(err, "servo console command failed")
	}
	defer s.RunServoCommand(ctx, "chan restore")
	// Run command on the servo console
	cmdOutput, err := s.RunServoCommandGetOutput(ctx, "dut_srccaps", []string{`dut_srccaps.*> `})
	if err != nil {
		testing.ContextLog(ctx, "Could not run servo command, are you on the right version?")
		testing.ContextLog(ctx, "Try updating servo \"servo_updater --updater_channel latest -- -b servo_v4p1 -c alpha\"")
		return nil, errors.Wrap(err, "could not run command to retrieve DUT src caps, try verifying servo FW version")
	}

	matches := srcCapsRe.FindAllStringSubmatch(cmdOutput[0][0], -1)
	for _, cap := range matches {
		mV, err := strconv.Atoi(cap[1])
		if err != nil {
			return nil, errors.Wrap(err, "voltage capability is not a integer value")
		}
		mA, err := strconv.Atoi(cap[2])
		if err != nil {
			return nil, errors.Wrap(err, "current capability is not a integer value")
		}
		value = append(value, SrcCap{mV, mA})
	}

	return value, nil
}

// SetFWWPState sets the FWWPState control.
// Because this is particularly disruptive, it is always logged.
func (s *Servo) SetFWWPState(ctx context.Context, value FWWPStateValue) error {
	ti50HasWPRebootFeature, err := s.GSCHasFeature(ctx, GSCAPROVerificationWPReboot)
	if err != nil {
		return errors.Wrap(err, "failed to determine if ti50-based device")
	}
	// Make sure the AllowUnverifiedRo setting is Always before changing the
	// WP state. If it's not Always, setting WP can brick the device.
	if ti50HasWPRebootFeature {
		if _, accessible, err := s.GetCCDCapability(ctx, AllowUnverifiedRo); err != nil {
			return errors.Wrap(err, "failed to get AllowUnverifiedRo state")
		} else if accessible != "Y" {
			return errors.Wrap(err, "cannot set WP unless AllowUnverifiedRo is Always")
		}
	}

	testing.ContextLogf(ctx, "Setting %q to %q", FWWPState, value)
	if err := s.SetString(ctx, FWWPState, string(value)); err != nil {
		return err
	}
	// For devices with Ti50-based GSCs, we also need to set the `atboot` WP
	// value since we expect the GSC to reboot itself. Note it is important that
	// we first set the normal FWWPState otherwise we can get into a mismatch
	// between servo_micro asserting WP externally and what Ti50 things WP should
	// be.
	if ti50HasWPRebootFeature {
		if err := s.setGSCFWWPAtBootState(ctx, value); err != nil {
			return errors.Wrapf(err, "failed to %q at boot firmware write protect", value)
		}

		if err := s.Ti50CheckPendingWPEvent(ctx, value); err != nil {
			return errors.Wrap(err, "failed to check Ti50 WP event")
		}
	}
	return nil
}

// setGSCFWWPAtBootState sets the GSCFWWPAtBootState control.
// Because this is particularly disruptive, it is always logged.
func (s *Servo) setGSCFWWPAtBootState(ctx context.Context, value FWWPStateValue) error {
	testing.ContextLogf(ctx, "Setting %q to %q", GSCFWWPAtBootState, value)
	return s.SetString(ctx, GSCFWWPAtBootState, string(value))
}

// GetPDRole returns the servo's current PDRole (SNK or SRC), or PDRoleNA if Servo is not V4.
func (s *Servo) GetPDRole(ctx context.Context) (PDRoleValue, error) {
	isV4, err := s.IsServoV4(ctx)
	if err != nil {
		return "", errors.Wrap(err, "determining whether servo is v4")
	}
	if !isV4 {
		return PDRoleNA, nil
	}
	role, err := s.GetString(ctx, PDRole)
	if err != nil {
		return "", err
	}
	return PDRoleValue(role), nil
}

// SetPDRole sets the PDRole control for a servo v4.
// On a Servo version other than v4, this does nothing.
func (s *Servo) SetPDRole(ctx context.Context, newRole PDRoleValue) error {
	// Determine the current PD role
	currentRole, err := s.GetPDRole(ctx)
	if err != nil {
		return errors.Wrap(err, "getting current PD role")
	}

	// If not using a servo V4, then we can't set the PD Role
	if currentRole == PDRoleNA {
		testing.ContextLogf(ctx, "Skipping setting %q to %q on non-v4 servo", PDRole, newRole)
		return nil
	}

	return s.SetString(ctx, PDRole, string(newRole))
}

// SetOnOff sets an OnOffControl setting to the specified value.
func (s *Servo) SetOnOff(ctx context.Context, ctrl OnOffControl, value OnOffValue) error {
	return s.SetString(ctx, StringControl(ctrl), string(value))
}

// ToggleOffOn turns a switch off and on again.
func (s *Servo) ToggleOffOn(ctx context.Context, ctrl OnOffControl) error {
	if err := s.SetString(ctx, StringControl(ctrl), string(Off)); err != nil {
		return err
	}
	// GoBigSleepLint: Simulating a specific speed of keypress
	if err := testing.Sleep(ctx, ServoKeypressDelay); err != nil {
		return err
	}
	if err := s.SetString(ctx, StringControl(ctrl), string(On)); err != nil {
		return err
	}
	return nil
}

// ToggleOnOff turns a switch on and off again.
func (s *Servo) ToggleOnOff(ctx context.Context, ctrl OnOffControl) error {
	if err := s.SetString(ctx, StringControl(ctrl), string(On)); err != nil {
		return err
	}
	// GoBigSleepLint: Simulating a specific speed of keypress
	if err := testing.Sleep(ctx, ServoKeypressDelay); err != nil {
		return err
	}
	if err := s.SetString(ctx, StringControl(ctrl), string(Off)); err != nil {
		return err
	}
	return nil
}

// GetOnOff gets an OnOffControl as a bool.
func (s *Servo) GetOnOff(ctx context.Context, ctrl OnOffControl) (bool, error) {
	str, err := s.GetString(ctx, StringControl(ctrl))
	if err != nil {
		return false, err
	}
	switch str {
	case string(On):
		return true, nil
	case string(Off):
		return false, nil
	}
	return false, errors.Errorf("cannot convert %q to boolean", str)
}

// WatchdogAdd adds the specified watchdog to the servod instance.
func (s *Servo) WatchdogAdd(ctx context.Context, val WatchdogValue) error {
	return s.SetString(ctx, WatchdogAdd, string(val))
}

// WatchdogRemove removes the specified watchdog from the servod instance.
func (s *Servo) WatchdogRemove(ctx context.Context, val WatchdogValue) error {
	if val == WatchdogCCD {
		// SuzyQ reports as ccd_cr50, and doesn't have a watchdog named CCD.
		servoType, err := s.GetServoType(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get servo type")
		}
		// No need to remove CCD watchdog if there is no CCD.
		if !s.hasCCD {
			testing.ContextLog(ctx, "Skipping watchdog remove CCD, because there is no CCD")
			return nil
		}
		if servoType == "ccd_cr50" {
			val = WatchdogMain
		}
	}
	testing.ContextLog(ctx, "Removing watchdog: ", val)
	if err := s.SetString(ctx, WatchdogRemove, string(val)); err != nil {
		return err
	}
	// GoBigSleepLint: Removing the watchdog seems to take some time before it works.
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		return err
	}
	return nil
}

// RunServoCommand runs the given command on the servo console.
func (s *Servo) RunServoCommand(ctx context.Context, cmd string) error {
	if err := s.SetString(ctx, UARTRegexp, "None"); err != nil {
		return errors.Wrap(err, "Clearing Servo UART Regexp")
	}
	return s.SetString(ctx, UARTCmd, cmd)
}

// RunServoCommandGetOutput runs the given command on the servo v4 UART and
// returns the output matching patterns.
func (s *Servo) RunServoCommandGetOutput(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	if err := s.SetStringList(ctx, UARTRegexp, patterns); err != nil {
		return nil, errors.Wrapf(err, "setting %s to %s", UARTRegexp, patterns)
	}

	defer s.SetString(ctx, UARTRegexp, "None")

	if err := s.SetString(ctx, UARTCmd, cmd); err != nil {
		return nil, errors.Wrapf(err, "setting %s to %s", string(UARTCmd), cmd)
	}

	iList, err := s.GetStringList(ctx, UARTCmd)
	if err != nil {
		return nil, errors.Wrap(err, "decoding string list")
	}

	return ConvertToStringArrayArray(ctx, iList)
}

// CaptureServoCommand runs the specified command on the servo console, capturing all output
// for the specified amount of time.
func (s *Servo) CaptureServoCommand(ctx context.Context, cmd string, timeout time.Duration) (retOutput string, retErr error) {
	closeUART, err := s.EnableUARTCapture(ctx, UARTCapture)
	if err != nil {
		return "", errors.Wrap(err, "failed to enable UART capture")
	}
	defer func() {
		retErr = errors.Join(retErr, closeUART(ctx))
	}()

	uartCtx, uartCtxCancel := context.WithTimeout(ctx, timeout)
	defer uartCtxCancel()
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	// Drain the UART output prior to running the command.
	if _, err := s.GetQuotedString(ctx, UARTStream); err != nil {
		return "", errors.Wrap(err, "failed to drain servo UART")
	}

	if err := s.SetString(ctx, UARTCmd, cmd); err != nil {
		return "", errors.Wrapf(err, "setting %s to %s", string(UARTCmd), cmd)
	}

	var output string
	var lines string
Loop:
	for {
		select {
		case <-uartCtx.Done():
			break Loop
		case <-ticker.C:
			lines, err = s.GetQuotedString(ctx, UARTStream)
			if err != nil {
				return "", errors.Wrap(err, "failed to read servo UART")
			}
			output += lines
		}
	}

	return output, nil
}

// RunUSBCDPConfigCommand executes the "usbc_action dp" command with the specified args on the servo
// console.
func (s *Servo) RunUSBCDPConfigCommand(ctx context.Context, args ...string) error {
	args = append([]string{"usbc_action dp"}, args...)
	cmd := strings.Join(args, " ")
	return s.RunServoCommand(ctx, cmd)
}

// SetCC sets the CC line to the specified value.
func (s *Servo) SetCC(ctx context.Context, val OnOffValue) error {
	cmd := "cc " + string(val)
	return s.RunServoCommand(ctx, cmd)
}

// SetActiveDUTController sets the active controller on a dual mode v4 servo
func (s *Servo) SetActiveDUTController(ctx context.Context, adc DUTController) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		err := s.SetStringTimeout(ctx, ActiveDUTController, string(adc), 30*time.Second)
		if err != nil && !strings.Contains(err.Error(), "activeV4DeviceError") {
			return testing.PollBreak(err)
		}
		return err
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 1 * time.Second})
}

// GetServoType gets the type of the servo.
func (s *Servo) GetServoType(ctx context.Context) (string, error) {
	if s.servoType != "" {
		return s.servoType, nil
	}
	servoType, err := s.GetString(ctx, Type)
	if err != nil {
		return "", err
	}

	devices, err := s.GetStringList(ctx, Devices)
	if err != nil {
		return "", err
	}

	var dutCCDController DUTController
	var dutDebugController DUTController
	hasCCD := false
	hasServoMicro := false
	hasC2D2 := false
	hasPDPartner := false
	hasDebugController := false

	for _, device := range devices {
		servoDeviceType, err := PropertyToString(device, "type")
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(servoDeviceType, "ccd") {
			hasCCD = true
			dutCCDController = DUTController(servoDeviceType)
		}
		if strings.HasPrefix(servoDeviceType, "servo_v4") {
			hasPDPartner = true
		}

		if strings.Compare(servoDeviceType, string(DUTControllerServoMicro)) == 0 {
			hasServoMicro = true
			hasDebugController = true
			dutDebugController = DUTController(servoDeviceType)
		} else if strings.Compare(servoDeviceType, string(DUTControllerC2D2)) == 0 {
			hasC2D2 = true
			hasDebugController = true
			dutDebugController = DUTController(servoDeviceType)
		}
	}

	if !hasCCD {
		if hasCCDState, err := s.HasControl(ctx, string(CCDState)); err != nil {
			return "", errors.Wrap(err, "failed to check ccd_state control")
		} else if hasCCDState {
			ccdState, err := s.GetOnOff(ctx, CCDState)
			if err != nil {
				return "", errors.Wrap(err, "failed to get ccd_state")
			}
			hasCCD = ccdState
		}
	}

	isDualV4 := hasCCD && (hasServoMicro || hasC2D2)
	isPDTester := hasDebugController && hasPDPartner

	if !hasCCD && !hasServoMicro && !hasC2D2 {
		testing.ContextLogf(ctx, "Assuming %s is equivalent to servo_micro", servoType)
		hasServoMicro = true
	}
	s.servoType = servoType
	s.hasCCD = hasCCD
	s.hasServoMicro = hasServoMicro
	s.hasC2D2 = hasC2D2
	s.isDualV4 = isDualV4
	s.isPDTester = isPDTester
	s.dutCCDController = dutCCDController
	s.dutDebugController = dutDebugController

	return s.servoType, nil
}

// GetPDCommunication returns the current value from servo_v4_PD_comm.
func (s *Servo) GetPDCommunication(ctx context.Context) (string, error) {
	pdComm, err := s.GetString(ctx, PDCommunication)
	if err != nil {
		return "", errors.Wrap(err, "failed to get PD communication status")
	}
	return pdComm, nil
}

// SetPDCommunication sets servo_pd_comm to the specified value, if it is not
// set to that value already.
func (s *Servo) SetPDCommunication(ctx context.Context, value OnOffValue) error {
	if err := s.SetString(ctx, PDCommunication, string(value)); err != nil {
		return errors.Wrap(err, "failed to set PD communication status")
	}
	return nil
}

// RequireCCD verifies that the servo has a CCD connection, and switches to it for dual v4 servos.
func (s *Servo) RequireCCD(ctx context.Context) error {
	servoType, err := s.GetServoType(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get servo type")
	}

	if !s.hasCCD {
		return errors.Wrapf(err, "servo %s is not CCD", servoType)
	}
	if s.isDualV4 {
		if err = s.SetActiveDUTController(ctx, s.dutCCDController); err != nil {
			return errors.Wrap(err, "failed to set active dut controller")
		}
	}
	return nil
}

// RequirePDTester verifies that the servo has both sides of the PD connection.
func (s *Servo) RequirePDTester(ctx context.Context) error {
	servoType, err := s.GetServoType(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get servo type")
	}

	if !s.isPDTester {
		return errors.Wrapf(err, "servo %s is not a PD tester", servoType)
	}
	return nil
}

// HasCCD checks if the servo has a CCD connection.
func (s *Servo) HasCCD(ctx context.Context) (bool, error) {
	_, err := s.GetServoType(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get servo type")
	}

	return s.hasCCD, nil
}

// HasC2D2 checks if the servo has a C2D2 connection.
func (s *Servo) HasC2D2(ctx context.Context) (bool, error) {
	_, err := s.GetServoType(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get servo type")
	}

	return s.hasC2D2, nil
}

// HasServoMicro checks if the servo has a servo_micro connection.
func (s *Servo) HasServoMicro(ctx context.Context) (bool, error) {
	_, err := s.GetServoType(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get servo type")
	}

	return s.hasServoMicro, nil
}

// PreferDebugHeader switches to the servo_micro or C2D2 for dual v4 servos, but doesn't fail on CCD only servos.
// Returns true if the servo has a debug header connection, false if it only has CCD.
func (s *Servo) PreferDebugHeader(ctx context.Context) (bool, error) {
	_, err := s.GetServoType(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get servo type")
	}
	if s.isDualV4 {
		if err = s.SetActiveDUTController(ctx, s.dutDebugController); err != nil {
			return false, errors.Wrap(err, "failed to set active dut controller")
		}
		return true, nil
	}
	return s.hasServoMicro || s.hasC2D2, nil
}

// RequireDebugHeader verifies that the servo has a servo_micro or C2D2 connection, and switches to it for dual v4 servos.
func (s *Servo) RequireDebugHeader(ctx context.Context) error {
	servoType, err := s.GetServoType(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get servo type")
	}
	if !s.hasServoMicro && !s.hasC2D2 {
		return errors.Wrapf(err, "servo %s doesn't have debug header", servoType)
	}
	if s.isDualV4 {
		if err = s.SetActiveDUTController(ctx, s.dutDebugController); err != nil {
			return errors.Wrap(err, "failed to set active dut controller")
		}
	}
	return nil
}

// OpenLid sends the ec command to simulate DUT lid opening
func (s *Servo) OpenLid(ctx context.Context) error {
	testing.ContextLog(ctx, "Opening DUT lid")
	return s.SetString(ctx, LidOpen, string(LidOpenYes))
}

// CloseLid sends the ec command to simulate DUT lid closing
func (s *Servo) CloseLid(ctx context.Context) error {
	testing.ContextLog(ctx, "Closing DUT lid")
	return s.SetString(ctx, LidOpen, string(LidOpenNo))
}

// EnableUARTCapture enables capture on the specified UART control, unless it is already enabled.
// Make sure you call the close function in a deferred call. I.e.
// closeUART, err := s.EnableUARTCapture(ctx, ECUARTCapture)
// if err != nil { ... }
// defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }
// ... Do a reboot or something ...
// err = s.PollForRegexp(ctx, ECUARTStream, regex, time.Minute)
// if err != nil { ... }
func (s *Servo) EnableUARTCapture(ctx context.Context, uart OnOffControl) (closeUART func(ctx context.Context) error, err error) {
	closeUART = func(ctx context.Context) error { return nil }
	if on, err := s.GetOnOff(ctx, uart); err != nil {
		return closeUART, errors.Wrapf(err, "failed to query capture %s", uart)
	} else if !on {
		testing.ContextLogf(ctx, "Clearing %s buffer before capturing", uart)
		lines := "placeholder"
		var err error
		for lines != "" {
			if lines, err = s.GetQuotedString(ctx, ECUARTStream); err != nil {
				return closeUART, errors.Wrapf(err, "failed to clear buffer before capturing %s", uart)
			}
		}

		testing.ContextLogf(ctx, "Capturing %s", uart)
		if err := s.SetOnOff(ctx, uart, On); err != nil {
			return closeUART, errors.Wrapf(err, "failed to capture %s", uart)
		}
		closeUART = func(ctx context.Context) error {
			testing.ContextLogf(ctx, "Disable capture %s", uart)
			if err := s.SetOnOff(ctx, uart, Off); err != nil {
				return errors.Wrapf(err, "failed to disable capture %s", uart)
			}
			return nil
		}
	}
	return
}

// PollForRegexp polls a UART for one or more strings for up to timeout, returning nil if found. You may need to call EnableUARTCapture() before rebooting to capture boot time logs.
func (s *Servo) PollForRegexp(ctx context.Context, uart StringControl, toFind *regexp.Regexp, timeout time.Duration) (bool, error) {
	var leftoverLines string
	type regexpNotFoundErr struct {
		*errors.E
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if lines, err := s.GetQuotedString(ctx, uart); err != nil {
			return errors.Wrap(err, "failed to read UART")
		} else if lines != "" {
			// It is possible to read partial lines, so save the part after newline for later
			lines = leftoverLines + lines
			if crlfIdx := strings.LastIndex(lines, "\r\n"); crlfIdx < 0 {
				leftoverLines = lines
				lines = ""
			} else {
				leftoverLines = lines[crlfIdx+2:]
				lines = lines[:crlfIdx+2]
			}

			for _, l := range strings.Split(lines, "\r\n") {
				if toFind.MatchString(l) {
					return nil
				}
			}
		}
		return &regexpNotFoundErr{E: errors.Errorf("failed to find %q", toFind)}
	}, &testing.PollOptions{Interval: time.Millisecond * 200, Timeout: timeout}); err != nil {
		if _, ok := errors.Unwrap(err).(*regexpNotFoundErr); ok {
			return false, nil
		}
		return false, errors.Wrap(err, "UART output-parsing failed")
	}
	return true, nil
}

// IsServoTypeC checks if the dut connection type is type-c.
func (s *Servo) IsServoTypeC(ctx context.Context) (bool, error) {
	// The servo is slightly evil, and will report that it has the
	// servo_pd_role control even for Type-A.
	connectionType := ""
	hasControl, err := s.HasControl(ctx, string(PDRole))
	if err != nil {
		return false, errors.Wrap(err, "checking for control")
	}
	if hasControl {
		connectionType, err = s.GetString(ctx, DUTConnectionType)
		if err != nil {
			return false, errors.Wrap(err, "getting connection type")
		}
	}
	return connectionType == string(DUTConnTypeC), nil
}

// CheckECActiveCopyMatch polls to check if the EC active copy matches the expected one.
func (s *Servo) CheckECActiveCopyMatch(ctx context.Context, expectedCopy string) error {
	activeCopy := ""
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		activeCopy, err = s.GetString(ctx, ECActiveCopy)
		return err
	}, &testing.PollOptions{
		Timeout: 20 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "EC active copy failed")
	}
	if !strings.HasPrefix(activeCopy, expectedCopy) {
		return errors.Errorf("EC active copy incorrect, got %q want %q", activeCopy, expectedCopy)
	}
	return nil
}

// GetServodVersion gets the version of servod being used.
func (s *Servo) GetServodVersion(ctx context.Context) (string, error) {
	var v string
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("servod_version"), &v)
	return v, err
}
