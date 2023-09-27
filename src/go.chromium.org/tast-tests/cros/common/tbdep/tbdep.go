// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tbdep

import (
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/chromiumos/config/go/api"
	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
)

// All the TestBedDeps that are simple constants.
const (
	// Arc is the "arc" dependency.
	Arc = "arc"

	// Atrus is the "atrus" dependency.
	Atrus = "atrus"

	// AudioBoard is the "audio_board" dependency.
	AudioBoard = "audio_board"

	// AudioBox is the "audio_box" dependency.
	AudioBox = "audio_box"

	// AudioCable is the "audio_cable" dependency.
	AudioCable = "audio_cable"

	// AudioLoopbackDongle is the "audio_loopback_dongle" dependency.
	AudioLoopbackDongle = "audio_loopback_dongle"

	// Bluetooth is the "bluetooth" dependency.
	Bluetooth = "bluetooth"

	// Callbox is the "callbox" dependency.
	Callbox = "callbox"

	// CameraBox is the "camerabox" dependency.
	CameraBox = "camerabox"

	// Chameleon is the "chameleon" dependency.
	Chameleon = "chameleon"

	// ChaosDUT is the "chaos_dut" dependency.
	ChaosDUT = "chaos_dut"

	// ChaosNightly is the "chaos_nightly" dependency.
	ChaosNightly = "chaos_nightly"

	// ChromeSign is the "chromesign" dependency.
	ChromeSign = "chromesign"

	// Conductive is the "conductive:True" dependency.
	//
	// Note: There are no other values for this one, so it's a constant rather
	// than a function.
	Conductive = "conductive:True"

	// DetachableBase is the "detachablebase" dependency.
	DetachableBase = "detachablebase"

	// Fingerprint is the "fingerprint" dependency.
	Fingerprint = "fingerprint"

	// Flashrom is the "flashrom" dependency.
	Flashrom = "flashrom"

	// HangoutApp is the "hangout_app" dependency.
	HangoutApp = "hangout_app"

	// Hotwording is the "hotwording" dependency.
	Hotwording = "hotwording"

	// Huddly is the "huddly" dependency.
	Huddly = "huddly"

	// InternalDisplay is the "internal_display" dependency.
	InternalDisplay = "internal_display"

	// LucidSleep is the "lucidsleep" dependency.
	LucidSleep = "lucidsleep"

	// MeetApp is the "meet_app" dependency.
	MeetApp = "meet_app"

	// Mimo is the "mimo" dependency.
	Mimo = "mimo"

	// RecoveryTest is the "recovery_test" dependency.
	RecoveryTest = "recovery_test"

	// Router80211ax is the "router_802_11ax" dependency.
	Router80211ax = "router_802_11ax"

	// Servo is the "servo" dependency.
	Servo = "servo"

	// SmartUSBHub is the "smart_usbhub" dependency.
	SmartUSBHub = "smart_usbhub"

	// Stylus is the "stylus" dependency.
	Stylus = "stylus"

	// TestAudioJack is the "test_audiojack" dependency.
	TestAudioJack = "test_audiojack"

	// TestHDMIAudio is the "test_hdmiaudio" dependency.
	TestHDMIAudio = "test_hdmiaudio"

	// TestUSBAudio is the "test_usbaudio" dependency.
	TestUSBAudio = "test_usbaudio"

	// TestUSBPrinting is the "test_usbprinting" dependency.
	TestUSBPrinting = "test_usbprinting"

	// Touchpad is the "touchpad" dependency.
	Touchpad = "touchpad"

	// Touchscreen is the "touchscreen" dependency.
	Touchscreen = "touchscreen"

	// USBDetect is the "usb_detect" dependency.
	USBDetect = "usb_detect"

	// UseLid is the "use_lid" dependency.
	UseLid = "use_lid"

	// Webcam is the "webcam" dependency.
	Webcam = "webcam"

	// Wificell is the "wificell" dependency.
	Wificell = "wificell"
)

// keyValueDep returns a formatted a dependency with key depKey and an optional
// depValue. If depValue is set, the dependency added is in the format
// "<depKey>:<depValue>", matching the syntax as required by swarming.
func keyValueDep(depKey, depValue string) string {
	var dep string
	if depValue != "" {
		dep = fmt.Sprintf("%s:%s", depKey, depValue)
	} else {
		dep = depKey
	}
	return dep
}

// AudioBoxJackPluggerState returns an "audiobox_jackplugger_state" dependency
// with the given state value.
func AudioBoxJackPluggerState(state string) string {
	return keyValueDep("audiobox_jackplugger_state", state)
}

// AudioLatencyToolkitState returns an "audio_latency_toolkit_state" dependency
// with the given PeripheralState value.
func AudioLatencyToolkitState(peripheralState string) string {
	return keyValueDep("audio_latency_toolkit_state", peripheralState)
}

// BatteryState returns a "battery_state" dependency with the given
// HardwareState value.
func BatteryState(hardwareState string) string {
	return keyValueDep("battery_state", hardwareState)
}

// BluetoothState returns a "bluetooth_state" dependency with the given
// HardwareState value.
func BluetoothState(hardwareState string) string {
	return keyValueDep("bluetooth_state", hardwareState)
}

// Board returns a "board" dependency with the given board value.
func Board(board string) string {
	return keyValueDep("board", board)
}

// BrandCode returns a "brand-code" dependency with the given brand value.
func BrandCode(brand string) string {
	return keyValueDep("brand-code", brand)
}

// CameraBoxFacing returns a "camerabox_facing" dependency with the given facing
// value.
func CameraBoxFacing(facing string) string {
	return keyValueDep("camerabox_facing", facing)
}

// CameraBoxLight returns a "camerabox_light" dependency with the given light
// value.
func CameraBoxLight(light string) string {
	return keyValueDep("camerabox_light", light)
}

// Carrier returns a "carrier" dependency with the given carrier value.
func Carrier(carrier string) string {
	return keyValueDep("carrier", carrier)
}

// Cbx returns a "cbx" dependency with the provided cbxState as the value.
func Cbx(cbxState bool) string {
	// Infra parsing of this value is not flexible, so we must set it exactly.
	var depValue string
	if cbxState {
		depValue = "True"
	} else {
		depValue = "False"
	}
	return keyValueDep("cbx", depValue)
}

// CbxBranding returns a "cbx_branding" dependency with the given cbxBranding
// value.
func CbxBranding(cbxBranding string) string {
	return keyValueDep("cbx_branding", cbxBranding)
}

// CellularModemState returns a "cellular_modem_state" dependency with the given
// HardwareState value.
func CellularModemState(hardwareState string) string {
	return keyValueDep("cellular_modem_state", hardwareState)
}

// CellularVariant returns a "cellular_variant" dependency with the specified
// device model cellular sub-variant.
func CellularVariant(modelVariant string) string {
	return keyValueDep("cellular_variant", modelVariant)
}

// ChameleonType returns a "chameleon" dependency with the type specified.
//
// Note: While Chameleon also returns a "chameleon" dependency, it specifies
// a different requirement.
func ChameleonType(chameleonType string) string {
	return keyValueDep("chameleon", chameleonType)
}

// ChameleonState returns a "chameleon_state" dependency with the given PeripheralState value.
func ChameleonState(peripheralState string) string {
	return keyValueDep("chameleon_state", peripheralState)
}

// CR50Phase returns a "cr50" dependency with the given phase value.
func CR50Phase(phase string) string {
	return keyValueDep("cr50", phase)
}

// CR50ROKeyID returns a "cr50-ro-keyid" dependency with the given keyID value.
func CR50ROKeyID(keyID string) string {
	return keyValueDep("cr50-ro-keyid", keyID)
}

// CR50ROVersion returns a "cr50-ro-version" dependency with the given version
// value.
func CR50ROVersion(version string) string {
	return keyValueDep("cr50-ro-version", version)
}

// CR50RWKeyID returns a "cr50-rw-keyid" dependency with the given keyID value.
func CR50RWKeyID(keyID string) string {
	return keyValueDep("cr50-rw-keyid", keyID)
}

// CR50RWVersion returns a "cr50-rw-version" dependency with the given version
// value.
func CR50RWVersion(version string) string {
	return keyValueDep("cr50-rw-version", version)
}

// CtsAbi returns a CTS ABI dependency in the format "cts_abi_<value>".
func CtsAbi(value string) string {
	return "cts_abi_" + value
}

// CtsCPU returns a CTS CPU dependency in the format "cts_cpu_<value>".
func CtsCPU(value string) string {
	return "cts_cpu_" + value
}

// DeviceSKU returns a "device-sku" dependency with the given deviceSku value.
func DeviceSKU(deviceSku string) string {
	return keyValueDep("device-sku", deviceSku)
}

// EC returns an "ec" dependency with the given embeddedControllerType value.
// Note: Only EC_CHROME is supported at this time.
func EC(embeddedControllerType api.HardwareFeatures_EmbeddedController_EmbeddedControllerType) string {
	var depValue string
	switch embeddedControllerType {
	case api.HardwareFeatures_EmbeddedController_EC_CHROME:
		depValue = "cros"
	default:
		panic(fmt.Sprintf("unsupported embeddedControllerType %q", embeddedControllerType.String()))
	}
	return keyValueDep("ec", depValue)
}

// GpuFamily returns a "gpu_family" dependency with the given gpuFamily value.
func GpuFamily(gpuFamily string) string {
	return keyValueDep("gpu_family", gpuFamily)
}

// Graphics returns a "graphics" dependency with the given graphics value.
func Graphics(graphics string) string {
	return keyValueDep("graphics", graphics)
}

// HmrState returns a "hmr_state" dependency with the given PeripheralState
// value.
func HmrState(peripheralState string) string {
	return keyValueDep("hmr_state", peripheralState)
}

// HWVideoAcc returns a video acceleration capability dependency in the format
// "hw_video_acc_<capability>".
func HWVideoAcc(videoAccelerationCapability string) string {
	return "hw_video_acc_" + videoAccelerationCapability
}

// HWIDComponent returns a "hwid_component" dependency with the given
// HWIDComponentName value.
func HWIDComponent(HWIDComponentName string) string {
	return keyValueDep("hwid_component", HWIDComponentName)
}

// LicenseType returns a license type dependency in the format
// "license_<licenseType>".
func LicenseType(licenseType string) string {
	return "license_" + licenseType
}

// Model returns a "model" dependency with the given model value.
func Model(model string) string {
	return keyValueDep("model", model)
}

// Modem returns a "modem" dependency with the given modem value.
func Modem(modem string) string {
	return keyValueDep("modem", modem)
}

// ModemIMEI returns a "modem_imei" dependency with the given modemIMEI value.
func ModemIMEI(modemIMEI string) string {
	return keyValueDep("modem_imei", modemIMEI)
}

// ModemSimCount returns a "modem_sim_count" dependency with the given simCount
// value.
func ModemSimCount(simCount int) string {
	return keyValueDep("modem_sim_count", strconv.Itoa(simCount))
}

// ModemSupportedBands returns a "modem_supported_bands" dependency, whose value
// is a comma-separated list of bands the modem must support.
func ModemSupportedBands(bands ...string) string {
	return keyValueDep("modem_supported_bands", strings.Join(bands, ","))
}

// ModemType returns a "modem_type" dependency with the given modemType value.
func ModemType(modemType string) string {
	return keyValueDep("modem_type", modemType)
}

// OS returns an "os" dependency with the given osType value.
func OS(osType string) string {
	return keyValueDep("os", osType)
}

// PeripheralWifiState returns a "peripheral_wifi_state" dependency with the
// specified state value.
func PeripheralWifiState(peripheralState string) string {
	return keyValueDep("peripheral_wifi_state", peripheralState)
}

// Phase returns a "phase" dependency with the given phase value.
func Phase(phase string) string {
	return keyValueDep("phase", phase)
}

// Platform returns a "platform" dependency with the given platform value.
func Platform(platform string) string {
	return keyValueDep("platform", platform)
}

// Pool returns a "pool" dependency with the given dutPool value.
func Pool(dutPool string) string {
	return keyValueDep("pool", dutPool)
}

// Power returns a "power" dependency with the given power value.
func Power(power string) string {
	return keyValueDep("power", power)
}

// ReferenceDesign returns a "reference_design" dependency with the given
// referenceDesign value.
func ReferenceDesign(referenceDesign string) string {
	return keyValueDep("reference_design", referenceDesign)
}

// RpmState returns a "rpm_state" dependency with the given PeripheralState
// value.
func RpmState(peripheralState string) string {
	return keyValueDep("rpm_state", peripheralState)
}

// ServoComponent returns a "servo_component" dependency with the given
// servoComponent value.
func ServoComponent(servoComponent string) string {
	return keyValueDep("servo_component", servoComponent)
}

// ServoState returns a "servo_state" dependency with the given PeripheralState
// value.
func ServoState(peripheralState string) string {
	return keyValueDep("servo_state", peripheralState)
}

// ServoTopology returns a "servo_topology" dependency. The provided
// servoTopology parameter expected to be a base64-encoded string of a
// ServoTopology marshalled to JSON.
func ServoTopology(servoTopology string) string {
	return keyValueDep("servo_topology", servoTopology)
}

// ServoType returns a "servo_type" dependency with the given servoType value.
func ServoType(servoType string) string {
	return keyValueDep("servo_type", servoType)
}

// ServoUSBState returns a "servo_usb_state" dependency with the given
// HardwareState value.
func ServoUSBState(hardwareState string) string {
	return keyValueDep("servo_usb_state", hardwareState)
}

// SimEID returns a sim dependency with the key in the format
// "sim_<slotID>_eid" and numProfiles as the value.
func SimEID(slotID int, simEID string) string {
	depKey := fmt.Sprintf("sim_%d_eid", slotID)
	return keyValueDep(depKey, simEID)
}

// SimNumProfiles returns a sim dependency with the key in the format
// "sim_<slotID>_num_profiles" and numProfiles as the value.
func SimNumProfiles(slotID, numProfiles int) string {
	depKey := fmt.Sprintf("sim_%d_num_profiles", slotID)
	return keyValueDep(depKey, strconv.Itoa(numProfiles))
}

// SimProfileCarrierName returns a sim dependency with the key in the format
// "sim_<slotID>_<profileID>_carrier_name" and carrierName as the value.
func SimProfileCarrierName(slotID, profileID int, carrierName string) string {
	depKey := fmt.Sprintf("sim_%d_%d_carrier_name", slotID, profileID)
	return keyValueDep(depKey, carrierName)
}

// SimProfileICCID returns a sim dependency with the key in the format
// "sim_<slotID>_<profileID>_iccid" and ICCID as the value.
func SimProfileICCID(slotID, profileID int, ICCID string) string {
	depKey := fmt.Sprintf("sim_%d_%d_iccid", slotID, profileID)
	return keyValueDep(depKey, ICCID)
}

// SimProfileOwnNumber returns a sim dependency with the key in the format
// "sim_<slotID>_<profileID>_own_number" and ownNumber as the value.
func SimProfileOwnNumber(slotID, profileID int, ownNumber string) string {
	depKey := fmt.Sprintf("sim_%d_%d_own_number", slotID, profileID)
	return keyValueDep(depKey, ownNumber)
}

// SimProfilePin returns a sim dependency with the key in the format
// "sim_<slotID>_<profileID>_pin" and pin as the value.
func SimProfilePin(slotID, profileID int, pin string) string {
	depKey := fmt.Sprintf("sim_%d_%d_pin", slotID, profileID)
	return keyValueDep(depKey, pin)
}

// SimProfilePuk returns a sim dependency with the key in the format
// "sim_<slotID>_<profileID>_puk" and puk as the value.
func SimProfilePuk(slotID, profileID int, puk string) string {
	depKey := fmt.Sprintf("sim_%d_%d_puk", slotID, profileID)
	return keyValueDep(depKey, puk)
}

// SimSlotID returns a "sim_slot_id" dependency with the specified slotID value.
func SimSlotID(slotID int) string {
	return keyValueDep("sim_slot_id", strconv.Itoa(slotID))
}

// SimTestEsim returns a sim dependency with the key in the format
// "sim_<slotID>_test_esim" and testEsim as the value.
func SimTestEsim(slotID int, testEsim bool) string {
	depKey := fmt.Sprintf("sim_%d_test_esim", slotID)
	return keyValueDep(depKey, strconv.FormatBool(testEsim))
}

// SimType returns a sim dependency with the key in the format
// "sim_<slotID>_type" and simType as the value.
func SimType(slotID int, simType string) string {
	depKey := fmt.Sprintf("sim_%d_type", slotID)
	return keyValueDep(depKey, simType)
}

// SKU returns a "sku" dependency with the given hwidSku value.
func SKU(hwidSku string) string {
	return keyValueDep("sku", hwidSku)
}

// StarfishSlotMapping returns a "starfish_slot_mapping" dependency with the
// provided mapping of slotID to carrier.
func StarfishSlotMapping(slotIDToCarrier map[int]string) string {
	if len(slotIDToCarrier) == 0 {
		panic("slotIDToCarrier must not be empty")
	}
	var mapEntries []string
	for slotID, carrier := range slotIDToCarrier {
		mapEntries = append(mapEntries, fmt.Sprintf("%d_%s", slotID, carrier))
	}
	return keyValueDep("starfish_slot_mapping", strings.Join(mapEntries, ","))
}

// Storage returns a "storage" dependency with the given storage value.
func Storage(storage string) string {
	return keyValueDep("storage", storage)
}

// StorageState returns a "storage_state" dependency with the given
// HardwareState value.
func StorageState(hardwareState string) string {
	return keyValueDep("storage_state", hardwareState)
}

// Telephony returns a "telephony" dependency with the given telephony value.
func Telephony(telephony string) string {
	return keyValueDep("telephony", telephony)
}

// TRRSType returns a "trrs_type" dependency with the given typeName value.
func TRRSType(typeName string) string {
	return keyValueDep("trrs_type", typeName)
}

// Variant returns a "variant" dependency with the given variant value.
func Variant(variant string) string {
	return keyValueDep("variant", variant)
}

// WifiChip returns a "wifi_chip" dependency with the given wifiChip value.
func WifiChip(wifiChip string) string {
	return keyValueDep("wifi_chip", wifiChip)
}

// WifiRouterFeatures returns a list of "wifi_router_features" dependencies,
// one for each provided required router feature.
//
// All routers in the testbed (including the pcap) must support these features
// to meet this requirement. See the WifiRouterFeature proto enum definition
// for documentation on what each feature means.
func WifiRouterFeatures(features ...labapi.WifiRouterFeature) []string {
	var deps []string
	for _, feature := range features {
		deps = append(deps, keyValueDep("wifi_router_features", feature.String()))
	}
	return deps
}

// WifiRouterModels returns list of a "wifi_router_models" dependencies, one for
// each provided model name.
//
// All listed models must be present in the testbed to match, but it is not
// necessary to repeat the requirements (i.e. just one "gale" is sufficient to
// match a testbed with a gale router and pcap).
func WifiRouterModels(models ...string) []string {
	var deps []string
	for _, model := range models {
		deps = append(deps, keyValueDep("wifi_router_models", model))
	}
	return deps
}

// WifiState returns a "wifi_state" dependency with the given HardwareState
// value.
func WifiState(hardwareState string) string {
	return keyValueDep("wifi_state", hardwareState)
}

// WorkingBluetoothPeers returns a "working_bluetooth_btpeer" dependency with
// the given workingBluetoothPeers value. This specifies the minimum amount of
// btpeers a testbed must have to be able to run the test.
func WorkingBluetoothPeers(workingBluetoothPeers int) string {
	return keyValueDep("working_bluetooth_btpeer", strconv.Itoa(workingBluetoothPeers))
}
