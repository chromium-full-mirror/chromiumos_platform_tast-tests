// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tbdep

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.chromium.org/chromiumos/config/go/api"
	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
)

func TestSimpleDeps(t *testing.T) {
	type buildDepFunc func() string
	tests := []struct {
		name        string
		buildDep    buildDepFunc
		expectedDep string
	}{
		{
			"AudioBoxJackPluggerState(WORKING)",
			func() string {
				return AudioBoxJackPluggerState("WORKING")
			},
			"audiobox_jackplugger_state:WORKING",
		},
		{
			"AudioLatencyToolkitState(WORKING)",
			func() string {
				return AudioLatencyToolkitState("WORKING")
			},
			"audio_latency_toolkit_state:WORKING",
		},
		{
			"BatteryState(NEED_REPLACEMENT)",
			func() string {
				return BatteryState("NEED_REPLACEMENT")
			},
			"battery_state:NEED_REPLACEMENT",
		},
		{
			"BluetoothState(NEED_REPLACEMENT)",
			func() string {
				return BluetoothState("NEED_REPLACEMENT")
			},
			"bluetooth_state:NEED_REPLACEMENT",
		},
		{
			"Board(boardval)",
			func() string {
				return Board("boardval")
			},
			"board:boardval",
		},
		{
			"BrandCode(HOMH)",
			func() string {
				return BrandCode("HOMH")
			},
			"brand-code:HOMH",
		},
		{
			"CameraBoxFacing(back)",
			func() string {
				return CameraBoxFacing("back")
			},
			"camerabox_facing:back",
		},
		{
			"CameraBoxLight(led)",
			func() string {
				return CameraBoxLight("led")
			},
			"camerabox_light:led",
		},
		{
			"Carrier(tmobile)",
			func() string {
				return Carrier("tmobile")
			},
			"carrier:tmobile",
		},
		{
			"Cbx(true)",
			func() string {
				return Cbx(true)
			},
			"cbx:True",
		},
		{
			"Cbx(false)",
			func() string {
				return Cbx(false)
			},
			"cbx:False",
		},
		{
			"CbxBranding(hard)",
			func() string {
				return CbxBranding("hard")
			},
			"cbx_branding:hard",
		},
		{
			"CellularModemState(NEED_REPLACEMENT)",
			func() string {
				return CellularModemState("NEED_REPLACEMENT")
			},
			"cellular_modem_state:NEED_REPLACEMENT",
		},
		{
			"CellularVariant(somecellularvariant)",
			func() string {
				return CellularVariant("somecellularvariant")
			},
			"cellular_variant:somecellularvariant",
		},
		{
			"ChameleonType(dp)",
			func() string {
				return ChameleonType("dp")
			},
			"chameleon:dp",
		},
		{
			"ChameleonType(hdmi)",
			func() string {
				return ChameleonType("hdmi")
			},
			"chameleon:hdmi",
		},
		{
			"ChameleonState(WORKING)",
			func() string {
				return ChameleonState("WORKING")
			},
			"chameleon_state:WORKING",
		},
		{
			"CR50Phase(pvt)",
			func() string {
				return CR50Phase("pvt")
			},
			"cr50:pvt",
		},
		{
			"CR50ROKeyID(prod)",
			func() string {
				return CR50ROKeyID("prod")
			},
			"cr50-ro-keyid:prod",
		},
		{
			"CR50ROVersion(1.2.3)",
			func() string {
				return CR50ROVersion("1.2.3")
			},
			"cr50-ro-version:1.2.3",
		},
		{
			"CR50RWKeyID(0xde88588d)",
			func() string {
				return CR50RWKeyID("0xde88588d")
			},
			"cr50-rw-keyid:0xde88588d",
		},
		{
			"CR50RWVersion(9.8.7)",
			func() string {
				return CR50RWVersion("9.8.7")
			},
			"cr50-rw-version:9.8.7",
		},
		{
			"CR50RWVersion(9.8.7)",
			func() string {
				return CR50RWVersion("9.8.7")
			},
			"cr50-rw-version:9.8.7",
		},
		{
			"CtsAbi(arm)",
			func() string {
				return CtsAbi("arm")
			},
			"cts_abi_arm",
		},
		{
			"CtsAbi(x86)",
			func() string {
				return CtsAbi("x86")
			},
			"cts_abi_x86",
		},
		{
			"CtsCpu(arm)",
			func() string {
				return CtsCPU("arm")
			},
			"cts_cpu_arm",
		},
		{
			"CtsCpu(x86)",
			func() string {
				return CtsCPU("x86")
			},
			"cts_cpu_x86",
		},
		{
			"DeviceSKU(skuval)",
			func() string {
				return DeviceSKU("skuval")
			},
			"device-sku:skuval",
		},
		{
			"EC(CHROME)",
			func() string {
				return EC(api.HardwareFeatures_EmbeddedController_EC_CHROME)
			},
			"ec:cros",
		},
		{
			"GpuFamily(gpufamilyval)",
			func() string {
				return GpuFamily("gpufamilyval")
			},
			"gpu_family:gpufamilyval",
		},
		{
			"Graphics(graphicsval)",
			func() string {
				return Graphics("graphicsval")
			},
			"graphics:graphicsval",
		},
		{
			"HmrState(WORKING)",
			func() string {
				return HmrState("WORKING")
			},
			"hmr_state:WORKING",
		},
		{
			"HWVideoAcc(enc_vp9)",
			func() string {
				return HWVideoAcc("enc_vp9")
			},
			"hw_video_acc_enc_vp9",
		},
		{
			"HWVideoAcc(enc_vp9_2)",
			func() string {
				return HWVideoAcc("enc_vp9_2")
			},
			"hw_video_acc_enc_vp9_2",
		},
		{
			"HWIDComponent(cellular/fake_cellular)",
			func() string {
				return HWIDComponent("cellular/fake_cellular")
			},
			"hwid_component:cellular/fake_cellular",
		},
		{
			"LicenseType(ms_office_standard)",
			func() string {
				return LicenseType("ms_office_standard")
			},
			"license_ms_office_standard",
		},
		{
			"LicenseType(license_windows_10_pro)",
			func() string {
				return LicenseType("windows_10_pro")
			},
			"license_windows_10_pro",
		},
		{
			"Model(modelval)",
			func() string {
				return Model("modelval")
			},
			"model:modelval",
		},
		{
			"Modem(modemval)",
			func() string {
				return Modem("modemval")
			},
			"modem:modemval",
		},
		{
			"ModemIMEI(imei)",
			func() string {
				return ModemIMEI("imei")
			},
			"modem_imei:imei",
		},
		{
			"ModemSimCount(1)",
			func() string {
				return ModemSimCount(1)
			},
			"modem_sim_count:1",
		},
		{
			"ModemSupportedBands(bands)",
			func() string {
				return ModemSupportedBands("bands")
			},
			"modem_supported_bands:bands",
		},
		{
			"ModemSupportedBands('band1,band2')",
			func() string {
				return ModemSupportedBands("band1,band2")
			},
			"modem_supported_bands:band1,band2",
		},
		{
			"ModemSupportedBands('band1','band2')",
			func() string {
				return ModemSupportedBands("band1", "band2")
			},
			"modem_supported_bands:band1,band2",
		},
		{
			"ModemType(qualcomm_sc7180)",
			func() string {
				return ModemType("qualcomm_sc7180")
			},
			"modem_type:qualcomm_sc7180",
		},
		{
			"OS(cros)",
			func() string {
				return OS("cros")
			},
			"os:cros",
		},
		{
			"PeripheralWifiState(WORKING)",
			func() string {
				return PeripheralWifiState("WORKING")
			},
			"peripheral_wifi_state:WORKING",
		},
		{
			"Phase(DVT2)",
			func() string {
				return Phase("DVT2")
			},
			"phase:DVT2",
		},
		{
			"Platform(platformval)",
			func() string {
				return Platform("platformval")
			},
			"platform:platformval",
		},
		{
			"Pool(bvt)",
			func() string {
				return Pool("bvt")
			},
			"pool:bvt",
		},
		{
			"Power(bvt)",
			func() string {
				return Power("powerval")
			},
			"power:powerval",
		},
		{
			"ReferenceDesign(reef)",
			func() string {
				return ReferenceDesign("reef")
			},
			"reference_design:reef",
		},
		{
			"RpmState(WORKING)",
			func() string {
				return RpmState("WORKING")
			},
			"rpm_state:WORKING",
		},
		{
			"ServoComponent(ccd_cr50)",
			func() string {
				return ServoComponent("ccd_cr50")
			},
			"servo_component:ccd_cr50",
		},
		{
			"ServoComponent(servo_v4)",
			func() string {
				return ServoComponent("servo_v4")
			},
			"servo_component:servo_v4",
		},
		{
			"ServoState(BROKEN)",
			func() string {
				return ServoState("BROKEN")
			},
			"servo_state:BROKEN",
		},
		{
			"ServoTopology(<val>)",
			func() string {
				return ServoState("eyJtYWluIjp7InR5cGUiOiJzZXJ2b192NCIsInN5c2ZzX3Byb2R1Y3QiOiJTZXJ2byBWNCIsInNlcmlhbCI6IkMxOTAzMTQ1NTkxIiwidXNiX2h1Yl9wb3J0IjoiNi40LjEifSwiY2hpbGRyZW4iOlt7InR5cGUiOiJjY2RfY3I1MCIsInN5c2ZzX3Byb2R1Y3QiOiJDcjUwIiwic2VyaWFsIjoiMDY4MUQwM0EtOTJEQ0NENjQiLCJ1c2JfaHViX3BvcnQiOiI2LjQuMiJ9XX0")
			},
			"servo_state:eyJtYWluIjp7InR5cGUiOiJzZXJ2b192NCIsInN5c2ZzX3Byb2R1Y3QiOiJTZXJ2byBWNCIsInNlcmlhbCI6IkMxOTAzMTQ1NTkxIiwidXNiX2h1Yl9wb3J0IjoiNi40LjEifSwiY2hpbGRyZW4iOlt7InR5cGUiOiJjY2RfY3I1MCIsInN5c2ZzX3Byb2R1Y3QiOiJDcjUwIiwic2VyaWFsIjoiMDY4MUQwM0EtOTJEQ0NENjQiLCJ1c2JfaHViX3BvcnQiOiI2LjQuMiJ9XX0",
		},
		{
			"ServoType(servo_v3)",
			func() string {
				return ServoType("servo_v3")
			},
			"servo_type:servo_v3",
		},
		{
			"ServoUSBState(NEED_REPLACEMENT)",
			func() string {
				return ServoUSBState("NEED_REPLACEMENT")
			},
			"servo_usb_state:NEED_REPLACEMENT",
		},
		{
			"SimEID(1,eid)",
			func() string {
				return SimEID(1, "eid1")
			},
			"sim_1_eid:eid1",
		},
		{
			"SimEID(2,eid)",
			func() string {
				return SimEID(2, "eid2")
			},
			"sim_2_eid:eid2",
		},
		{
			"SimNumProfiles(1,2)",
			func() string {
				return SimNumProfiles(1, 2)
			},
			"sim_1_num_profiles:2",
		},
		{
			"SimNumProfiles(3,4)",
			func() string {
				return SimNumProfiles(3, 4)
			},
			"sim_3_num_profiles:4",
		},
		{
			"SimProfileICCID(1,0,iccid1)",
			func() string {
				return SimProfileICCID(1, 0, "iccid1")
			},
			"sim_1_0_iccid:iccid1",
		},
		{
			"SimProfileCarrierName(1,0,NETWORK_TEST)",
			func() string {
				return SimProfileCarrierName(1, 0, "NETWORK_TEST")
			},
			"sim_1_0_carrier_name:NETWORK_TEST",
		},
		{
			"SimProfileCarrierName(2,1,NETWORK_ATT)",
			func() string {
				return SimProfileCarrierName(2, 1, "NETWORK_ATT")
			},
			"sim_2_1_carrier_name:NETWORK_ATT",
		},
		{
			"SimProfileICCID(2,1,iccid2)",
			func() string {
				return SimProfileICCID(2, 1, "iccid2")
			},
			"sim_2_1_iccid:iccid2",
		},
		{
			"SimProfileOwnNumber(1,0,1234567890)",
			func() string {
				return SimProfileOwnNumber(1, 0, "1234567890")
			},
			"sim_1_0_own_number:1234567890",
		},
		{
			"SimProfileOwnNumber(2,1,3456789012)",
			func() string {
				return SimProfileOwnNumber(2, 1, "3456789012")
			},
			"sim_2_1_own_number:3456789012",
		},
		{
			"SimProfilePin(1,0,pin1)",
			func() string {
				return SimProfilePin(1, 0, "pin1")
			},
			"sim_1_0_pin:pin1",
		},
		{
			"SimProfilePin(2,1,pin2)",
			func() string {
				return SimProfilePin(2, 1, "pin2")
			},
			"sim_2_1_pin:pin2",
		},
		{
			"SimProfilePuk(1,0,puk1)",
			func() string {
				return SimProfilePuk(1, 0, "puk1")
			},
			"sim_1_0_puk:puk1",
		},
		{
			"SimProfilePuk(2,1,puk2)",
			func() string {
				return SimProfilePuk(2, 1, "puk2")
			},
			"sim_2_1_puk:puk2",
		},
		{
			"SimSlotID(1)",
			func() string {
				return SimSlotID(1)
			},
			"sim_slot_id:1",
		},
		{
			"SimSlotID(2)",
			func() string {
				return SimSlotID(2)
			},
			"sim_slot_id:2",
		},
		{
			"SimTestEsim(1,true)",
			func() string {
				return SimTestEsim(1, true)
			},
			"sim_1_test_esim:true",
		},
		{
			"SimTestEsim(2,false)",
			func() string {
				return SimTestEsim(2, false)
			},
			"sim_2_test_esim:false",
		},
		{
			"SimType(1,SIM_PHYSICAL)",
			func() string {
				return SimTestEsim(2, false)
			},
			"sim_2_test_esim:false",
		},
		{
			"SimType(1,SIM_PHYSICAL)",
			func() string {
				return SimType(1, "SIM_PHYSICAL")
			},
			"sim_1_type:SIM_PHYSICAL",
		},
		{
			"SKU(eve_IntelR_CoreTM_i7_7Y75_CPU_1_30GHz_16GB)",
			func() string {
				return SKU("eve_IntelR_CoreTM_i7_7Y75_CPU_1_30GHz_16GB")
			},
			"sku:eve_IntelR_CoreTM_i7_7Y75_CPU_1_30GHz_16GB",
		},
		{
			"Storage(storageval)",
			func() string {
				return Storage("storageval")
			},
			"storage:storageval",
		},
		{
			"StorageState(NORMAL)",
			func() string {
				return StorageState("NORMAL")
			},
			"storage_state:NORMAL",
		},
		{
			"Telephony(telephonyval)",
			func() string {
				return Telephony("telephonyval")
			},
			"telephony:telephonyval",
		},
		{
			"TRRSType(CTIA)",
			func() string {
				return TRRSType("CTIA")
			},
			"trrs_type:CTIA",
		},
		{
			"Variant(somevariant)",
			func() string {
				return Variant("somevariant")
			},
			"variant:somevariant",
		},
		{
			"WifiChip(wireless_xxxx)",
			func() string {
				return WifiChip("wireless_xxxx")
			},
			"wifi_chip:wireless_xxxx",
		},
		{
			"WifiState(NEED_REPLACEMENT)",
			func() string {
				return WifiState("NEED_REPLACEMENT")
			},
			"wifi_state:NEED_REPLACEMENT",
		},
		{
			"WorkingBluetoothPeers(3)",
			func() string {
				return WorkingBluetoothPeers(3)
			},
			"working_bluetooth_btpeer:3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.buildDep()
			assert.Equalf(t, actual, tt.expectedDep, "expected %s to return %q, but got %q", tt.name, actual, tt.expectedDep)
		})
	}
}

func TestTestBedDepsBuilder_StarfishSlotMapping(t *testing.T) {
	actualDep := StarfishSlotMapping(map[int]string{
		1: "verizon",
		2: "tmobile",
		4: "att",
	})
	depParts := strings.Split(actualDep, ":")
	assert.Len(t, depParts, 2)
	depKey := depParts[0]
	depValue := depParts[1]
	assert.Equal(t, depKey, "starfish_slot_mapping")
	assert.ElementsMatch(t, strings.Split(depValue, ","), []string{
		"1_verizon",
		"2_tmobile",
		"4_att",
	})
}

func TestTestBedDepsBuilder_WifiRouterFeatures(t *testing.T) {
	actualDeps := WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_A)
	assert.Equal(t, actualDeps, []string{
		"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_A",
	})
	actualDeps = WifiRouterFeatures(
		labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_B,
		labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_G,
		labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_N,
	)
	assert.Equal(t, actualDeps, []string{
		"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_B",
		"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_G",
		"wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_N",
	})
}

func TestTestBedDepsBuilder_WifiRouterModels(t *testing.T) {
	actualDeps := WifiRouterModels("gale")
	assert.Equal(t, actualDeps, []string{
		"wifi_router_models:gale",
	})
	actualDeps = WifiRouterModels(
		"OPENWRT[Ubiquiti_Unifi_6_Lite]",
		"OPENWRT[Another_Model]",
	)
	assert.Equal(t, actualDeps, []string{
		"wifi_router_models:OPENWRT[Ubiquiti_Unifi_6_Lite]",
		"wifi_router_models:OPENWRT[Another_Model]",
	})
}
