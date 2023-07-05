// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testdevicerequirements provides a way to define ChromeOS
// software and device requirements used for mapping test cases to
// requirements
//
// # Usage
//
// import tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirement"
//
// testing.AddTest(&testing.Test{
//
//	Func:         ExampleTest,
//	LacrosStatus: testing.LacrosVariantNeeded,
//	Desc:         "Fake example test",
//	...
//	Requirements: []string{tdreq.BootPerfKernel, tdreq.BootPerfLogin},
//	...
//
// )
//
// Contact cros-test-xfn-requirements@google.com with any questions.
package testdevicerequirements

const (
	// BootPerfKernel is a ChromeOS Platform requirement. Please contact cros-test-xfn-requirements@google.com with any questions.
	BootPerfKernel = "boot-perf-0001-v01"

	// BootPerfLogin is a ChromeOS Platform requirement. Please contact cros-test-xfn-requirements@google.com with any questions.
	BootPerfLogin = "boot-perf-0002-v01"

	// Connectivity Wi-Fi Requirements, Please contact chromeos-wifi-team@google.com with any questions.
	// Only requirements that map to existing tests are listed here. Please refer to this link for full list
	// of Connectivity WiFi requirements:
	// link: https://chromeos.google.com/partner/dlm/docs/latest-requirements/chromebook.html#wi-fi
	// The mapping: go/pvs_wifi_tests_mapping

	// [[ WiFi coexistence ]]

	// WiFiCoexSupportBT the Wi-Fi controller MUST support the coexistence with Bluetooth.
	WiFiCoexSupportBT = "wifi-coex-0001-v01"

	// [[ WiFi driver ]]

	// WiFiDrvSupportCrOS the Wi-Fi controller MUST support Chrome OS platforms.
	WiFiDrvSupportCrOS = "wifi-drv-0001-v01"

	// WiFiDrvSupportCfg80211 the Linux driver for the Wi-Fi controller MUST support cfg80211, the Linux 802.11 configuration API.
	WiFiDrvSupportCfg80211 = "wifi-drv-0005-v01"

	// [[ WiFi generic ]]

	// WiFiGenSupportWiFi the device MUST support Wi-Fi connectivity.
	WiFiGenSupportWiFi = "wifi-gen-0001-v01"

	// WiFiGenSupport80211ax the Wi-Fi controller MUST support 802.11ax (Wifi 6).
	WiFiGenSupport80211ax = "wifi-gen-0002-v01"

	// WiFiGenSupport6E the Wi-Fi controller SHOULD support Wifi 6E.
	WiFiGenSupport6E = "wifi-gen-0003-v01"

	// WiFiGenSupportLegacy the Wi-Fi controller MUST support legacy WiFi standards (802.11a/b/g/n/ac).
	WiFiGenSupportLegacy = "wifi-gen-0004-v01"

	// WiFiGenSupport2x2MIMO the Wi-Fi controller MUST support 2x2 MIMO.
	WiFiGenSupport2x2MIMO = "wifi-gen-0005-v01"

	// WiFiGenSupportPMF the Wi-Fi controller MUST support PMF (Protected Management Frames).
	WiFiGenSupportPMF = "wifi-gen-0006-v01"

	// WiFiGenSupportLegacyBands the device MUST support 2.4 and 5Ghz for STA.
	WiFiGenSupportLegacyBands = "wifi-gen-0007-v02"

	// WiFiGenSupport6GHz the device MAY support 6Ghz for STA and Mobile AP.
	WiFiGenSupport6GHz = "wifi-gen-0013-v01"

	// WiFiGenSupport6GHzV2 the device MAY support 6Ghz for STA.
	WiFiGenSupport6GHzV2 = "wifi-gen-0013-v02"

	// WiFiGenSupportMARScan the Linux driver for the Wi-Fi controller MUST support MAC address randomization for scans.
	WiFiGenSupportMARScan = "wifi-gen-0014-v01"

	// WiFiGenSupportMARConn the Linux driver for the Wi-Fi controller MUST support MAC address randomization for connections.
	WiFiGenSupportMARConn = "wifi-gen-0015-v01"

	// WiFiGenSupportMBO the Wi-Fi controller MUST support MBO / Agile Multiband.
	WiFiGenSupportMBO = "wifi-gen-0016-v01"

	// WiFiGenSupportPasspoint the Wi-Fi controller MUST support Passpoint / Hotspot 2.0.
	WiFiGenSupportPasspoint = "wifi-gen-0017-v01"

	// WiFiGenSupportWFD the Wi-Fi controller SHOULD support WiFi Direct.
	WiFiGenSupportWFD = "wifi-gen-0019-v01"

	// [[ WiFi process ]]

	// WiFiProcPassFW the new FW releases MUST pass all Chrome OS validation test cases.
	WiFiProcPassFW = "wifi-proc-0009-v01"

	// WiFiProcPassAVL the AVL test suite MUST pass in order to grant an AVL certification for the Wi-Fi controller.
	WiFiProcPassAVL = "wifi-proc-0012-v01"

	// WiFiProcPassAVLBeforeUpdates the AVL test suite MUST pass before Wi-Fi controller firmware updates are released to ChromeOS.
	WiFiProcPassAVLBeforeUpdates = "wifi-proc-0013-v01"

	// WiFiProcPassMatfunc the wifi_matfunc test suite MUST pass in order to grant an AVL certification for the Wi-Fi controller.
	WiFiProcPassMatfunc = "wifi-proc-0014-v01"

	// WiFiProcPassStress the wifi_stress test suite MUST pass in order to grant an AVL certification for the Wi-Fi controller.
	WiFiProcPassStress = "wifi-proc-0015-v01"

	// WiFiProcPassPerf the wifi_perf test suite MUST pass in order to grant an AVL certification for the Wi-Fi controller.
	WiFiProcPassPerf = "wifi-proc-0016-v01"

	// WiFiProcPassAttenPerf the wifi_atten_perf test suite MUST pass in order to grant an AVL certification for the Wi-Fi controller.
	WiFiProcPassAttenPerf = "wifi-proc-0017-v01"

	// WiFiProcPassMatfuncBeforeUpdates the wifi_matfunc test suite MUST pass before Wi-Fi controller firmware updates are released to ChromeOS.
	WiFiProcPassMatfuncBeforeUpdates = "wifi-proc-0018-v01"

	// WiFiProcPassStressBeforeUpdates the wifi_stress test suite MUST pass before Wi-Fi controller firmware updates are released to ChromeOS.
	WiFiProcPassStressBeforeUpdates = "wifi-proc-0019-v01"

	// WiFiProcPassPerfBeforeUpdates the wifi_perf test suite MUST pass before Wi-Fi controller firmware updates are released to ChromeOS.
	WiFiProcPassPerfBeforeUpdates = "wifi-proc-0020-v01"

	// WiFiProcPassAttenPerfBeforeUpdates the wifi_atten_perf test suite MUST pass before Wi-Fi controller firmware updates are released to ChromeOS.
	WiFiProcPassAttenPerfBeforeUpdates = "wifi-proc-0021-v01"

	// [[ WiFi power & timing ]]

	// WiFiPwrTimingWoW the Wi-Fi controller MUST support Wake-On-Wifi.
	WiFiPwrTimingWoW = "wifi-pwrTiming-0001-v01"

	// [[ WiFi regulation ]]

	// WiFiRegSupportDynamicPowerTable If the device is targeting multiple regulatory domains with different output power limits, the Wi-Fi controller MUST support dynamic power table.
	WiFiRegSupportDynamicPowerTable = "wifi-reg-0002-v01"

	// WiFiRegSupportStaticSAR the Wi-Fi controller MUST support Static SAR support.
	WiFiRegSupportStaticSAR = "wifi-reg-0003-v01"

	// WiFiRegSupportDynamicSAR the Wi-Fi controller MUST support Dynamic SAR support.
	WiFiRegSupportDynamicSAR = "wifi-reg-0004-v01"

	// WiFiRegSupportGeoSAR the Wi-Fi controller MUST support Geo SAR support.
	WiFiRegSupportGeoSAR = "wifi-reg-0005-v01"

	// WiFiRegSupportNL80211CMD the Wi-Fi controller MUST accept regulatory domain indication via NL80211_CMD_REQ_SET_REG.
	WiFiRegSupportNL80211CMD = "wifi-reg-0008-v01"

	// [[ WiFi radio ]]

	// WiFiRf6E160MHz if the Wi-Fi controller supports Wifi 6E, the controller MUST support a transmission frequency bandwidth of 160 MHz.
	WiFiRf6E160MHz = "wifi-rf-0016-v01"

	// [[ WiFi security ]]

	// WiFiSecSupportOpen the Wi-Fi controller MUST support connecting to AP without security protocol (open security).
	WiFiSecSupportOpen = "wifi-sec-0001-v01"

	// WiFiSecSupportWPA3Personal the device MUST support WPA3-Personal as defined in the WiFi Alliance specification.
	WiFiSecSupportWPA3Personal = "wifi-sec-0002-v01"

	// WiFiSecSupportWPA3Enterprise the device MUST support WPA3-Enterprise as defined in the WiFi Alliance specification.
	WiFiSecSupportWPA3Enterprise = "wifi-sec-0003-v01"

	// WiFiSecSupportWPA2Personal the device MUST support WPA2-Personal as defined in the WiFi Alliance specification.
	WiFiSecSupportWPA2Personal = "wifi-sec-0005-v01"

	// WiFiSecSupportWPA2Enterprise the device MUST support WPA2-Enterprise as defined in the WiFi Alliance specification.
	WiFiSecSupportWPA2Enterprise = "wifi-sec-0006-v01"

	// WiFiSecSupportWEP the device MUST support WEP as defined in the WiFi Alliance specification.
	WiFiSecSupportWEP = "wifi-sec-0007-v01"

	// [[ WiFi standard specifications ]]

	// WiFiSpecUlMIMO the Wi-Fi controller MAY support UL MU-MIMO.
	WiFiSpecUlMIMO = "wifi-spec-0003-v01"

	// [[ WiFi throughput ]]

	// WiFiTputRvR the vendor MUST provide RvR results, demonstrating the througphut/PHY rate vs range/attenuation.
	WiFiTputRvR = "wifi-tput-0001-v01"

	// WiFiTputHT20UDPTx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 72 mbps.
	WiFiTputHT20UDPTx = "wifi-tput-0002-v01"

	// WiFiTputHT20UDPRx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 72 mbps.
	WiFiTputHT20UDPRx = "wifi-tput-0003-v01"

	// WiFiTputHT20UDPRxTx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 72 mbps.
	WiFiTputHT20UDPRxTx = "wifi-tput-0004-v01"

	// WiFiTputHT40UDPTx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 135 mbps.
	WiFiTputHT40UDPTx = "wifi-tput-0007-v01"

	// WiFiTputHT40UDPRx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 135 mbps.
	WiFiTputHT40UDPRx = "wifi-tput-0008-v01"

	// WiFiTputHT40UDPRxTx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 135 mbps.
	WiFiTputHT40UDPRxTx = "wifi-tput-0009-v01"

	// WiFiTputVHT20UDPTx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 87 mbps.
	WiFiTputVHT20UDPTx = "wifi-tput-0010-v01"

	// WiFiTputVHT20UDPRx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 87 mbps.
	WiFiTputVHT20UDPRx = "wifi-tput-0011-v01"

	// WiFiTputVHT20UDPRxTx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 87 mbps.
	WiFiTputVHT20UDPRxTx = "wifi-tput-0012-v01"

	// WiFiTputVHT40UDPTx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 180 mbps.
	WiFiTputVHT40UDPTx = "wifi-tput-0013-v01"

	// WiFiTputVHT40UDPRx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 180 mbps.
	WiFiTputVHT40UDPRx = "wifi-tput-0014-v01"

	// WiFiTputVHT40UDPRxTx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 180 mbps.
	WiFiTputVHT40UDPRxTx = "wifi-tput-0015-v01"

	// WiFiTputVHT80UDPTx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 347 mbps.
	WiFiTputVHT80UDPTx = "wifi-tput-0016-v01"

	// WiFiTputVHT80UDPRx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 347 mbps.
	WiFiTputVHT80UDPRx = "wifi-tput-0017-v01"

	// WiFiTputVHT80UDPRxTx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum UDP throughput of 347 mbps.
	WiFiTputVHT80UDPRxTx = "wifi-tput-0018-v01"

	// WiFiTputHT20TCPTx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 61 mpbs.
	WiFiTputHT20TCPTx = "wifi-tput-0031-v01"

	// WiFiTputHT20TCPRx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 61 mpbs.
	WiFiTputHT20TCPRx = "wifi-tput-0032-v01"

	// WiFiTputHT20TCPRxTx with Wi-Fi connection of 802.11n 20Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 61 mpbs.
	WiFiTputHT20TCPRxTx = "wifi-tput-0033-v01"

	// WiFiTputHT40TCPTx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 115 mpbs.
	WiFiTputHT40TCPTx = "wifi-tput-0036-v01"

	// WiFiTputHT40TCPRx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 115 mpbs.
	WiFiTputHT40TCPRx = "wifi-tput-0037-v01"

	// WiFiTputHT40TCPRxTx with Wi-Fi connection of 802.11n 40Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 115 mpbs.
	WiFiTputHT40TCPRxTx = "wifi-tput-0038-v01"

	// WiFiTputVHT20TCPTx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 74 mpbs.
	WiFiTputVHT20TCPTx = "wifi-tput-0039-v01"

	// WiFiTputVHT20TCPRx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 74 mpbs.
	WiFiTputVHT20TCPRx = "wifi-tput-0040-v01"

	// WiFiTputVHT20TCPRxTx with Wi-Fi connection of 802.11ac 20Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 74 mpbs.
	WiFiTputVHT20TCPRxTx = "wifi-tput-0041-v01"

	// WiFiTputVHT40TCPTx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 153 mpbs.
	WiFiTputVHT40TCPTx = "wifi-tput-0042-v01"

	// WiFiTputVHT40TCPRx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 153 mpbs.
	WiFiTputVHT40TCPRx = "wifi-tput-0043-v01"

	// WiFiTputVHT40TCPRxTx with Wi-Fi connection of 802.11ac 40Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 153 mpbs.
	WiFiTputVHT40TCPRxTx = "wifi-tput-0044-v01"

	// WiFiTputVHT80TCPTx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 200 mpbs.
	WiFiTputVHT80TCPTx = "wifi-tput-0045-v01"

	// WiFiTputVHT80TCPRx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES RX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 200 mpbs.
	WiFiTputVHT80TCPRx = "wifi-tput-0046-v01"

	// WiFiTputVHT80TCPRxTx with Wi-Fi connection of 802.11ac 80Mhz, 2 streams in AES RX+TX mode, the Wi-Fi controller SHOULD provide a minimum TCP throughput of 200 mpbs.
	WiFiTputVHT80TCPRxTx = "wifi-tput-0047-v01"

	// Storage Requirements

	// InternalStorageGeneral with The ChromeOS device MUST provide storage with a non-volatile solid-state storage device.
	// This is a catch-all requirement for all tests that don't have a dedicated requirement at the moment.
	InternalStorageGeneral = "store-general-0001-v01"

	// InternalStorageInterface with The ChromeOS device MUST provide non-volatile storage via one or more of the following interfaces:
	// * eMMC
	// * NVMe
	// * UFS
	InternalStorageInterface = "store-motherbrd-0001-v01"

	// EmmcInterface with The ChromeOS device MAY provide non-volatile storage via eMMC interface.
	EmmcInterface = "store-motherbrd-0002-v01"

	// NvmeInterface with The ChromeOS device MAY provide non-volatile storage via NVMe interface.
	NvmeInterface = "store-motherbrd-0003-v01"

	// UfsInterface with The ChromeOS device MAY provide non-volatile storage via NVMe interface.
	UfsInterface = "store-motherbrd-0004-v01"

	// NvmeStorageSeqReadTp with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device MUST support >= 250 MB/second sequential read operations.
	NvmeStorageSeqReadTp = "store-nvme-0007-v01"

	// NvmeStorageSeqWriteTp with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device MUST support >= 100 MB/second sequential write operations.
	NvmeStorageSeqWriteTp = "store-nvme-0009-v01"

	// NvmeStorageSeqReadLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 512k sequential reads, with iodepth=1 workload.
	NvmeStorageSeqReadLatency = "store-nvme-0017-v01"

	// NvmeStorageSeqWriteLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 512k sequential writes, with iodepth=1 workload.
	NvmeStorageSeqWriteLatency = "store-nvme-0018-v01"

	// EmmcStorageSeqReadTp with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST support >= 50 MB/second sequential read operations.
	EmmcStorageSeqReadTp = "store-emmc-0011-v01"

	// EmmcStorageSeqWriteTp with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST support >= 20 MB/second sequential write operations.
	EmmcStorageSeqWriteTp = "store-emmc-0013-v01"

	// EmmcStorageSeqReadLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 512k sequential reads, with iodepth=1 workload.
	EmmcStorageSeqReadLatency = "store-emmc-0021-v01"

	// EmmcStorageSeqWriteLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 512k sequential writes, with iodepth=1 workload.
	EmmcStorageSeqWriteLatency = "store-emmc-0022-v01"

	// UfsStorageSeqReadTp with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device MUST support >= 250 MB/second sequential read operations.
	UfsStorageSeqReadTp = "store-ufs-0014-v01"

	// UfsStorageSeqWriteTp with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device MUST support >= 100 MB/second sequential write operations.
	UfsStorageSeqWriteTp = "store-ufs-0016-v01"

	// UfsStorageSeqReadLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 512k sequential reads, with iodepth=1 workload.
	UfsStorageSeqReadLatency = "store-ufs-0032-v01"

	// UfsStorageSeqWriteLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 512k sequential writes, with iodepth=1 workload.
	UfsStorageSeqWriteLatency = "store-ufs-0033-v01"

	// RemovableStorageSeqTp with The ChromeOS device MUST support >= 40 MBps throughput to external storage devices.
	RemovableStorageSeqTp = "rmvbl-general-0001-v01"

	// Storage16kReadIOPs with The ChromeOS device non-volatile storage MUST support >= 1500 16kB random read IOPS.
	Storage16kReadIOPs = "store-performance-0001-v01"

	// Storage16kWriteIOPs with The ChromeOS device non-volatile storage MUST support >= 150 16kB random write IOPS.
	Storage16kWriteIOPs = "store-performance-0003-v01"

	// EmmcStorageControllerRevision with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC controller MUST support eMMC5.1
	EmmcStorageControllerRevision = "store-emmc-0003-v01"

	// EmmcStorageControllerHS with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC controller MUST support HS400-ES mode.
	EmmcStorageControllerHS = "store-emmc-0004-v01"

	// EmmcStorageControllerCqeSupport with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC controller SHOULD support CQE
	EmmcStorageControllerCqeSupport = "store-emmc-0005-v01"

	// EmmcStorageControllerCqeActive with If the ChromeOS device uses eMMC to provide non-volatile storage, and if its eMMC controller supports CQE, the ChromeOS device MUST enable CQE.
	EmmcStorageControllerCqeActive = "store-emmc-0006-v01"

	// EmmcStorageDeviceRevision with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST support eMMC5.1
	EmmcStorageDeviceRevision = "store-emmc-0007-v01"

	// EmmcStorageDeviceHS with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST support HS400-ES mode.
	EmmcStorageDeviceHS = "store-emmc-0008-v01"

	// EmmcStorageDeviceFFU with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST support FFU according to sections 6.6.18 and A.12 of JESD84-B51
	EmmcStorageDeviceFFU = "store-emmc-0009-v01"

	// EmmcStorageDeviceExtCsd with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device MUST provide health info via ExtCsd.
	EmmcStorageDeviceExtCsd = "store-emmc-0010-v01"

	// EmmcStorage16kReadLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 16k random reads, with iodepth=4 workload.
	EmmcStorage16kReadLatency = "store-emmc-0019-v01"

	// EmmcStorage16kWriteLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 16k random writes, with iodepth=4 workload.
	EmmcStorage16kWriteLatency = "store-emmc-0020-v01"

	// StorageTBWReport with If the ChromeOS device provides non-volatile storage via an interface whose standard specifies reporting written/read bytes, its non-volatile storage MUST support reporting written/read bytes.
	StorageTBWReport = "store-storagedev-0007-v01"

	// NvmeStorageOnPcie with If the ChromeOS device uses NVMe to provide non-volatile storage, it MUST provide NVMe over PCIe physical interface.
	NvmeStorageOnPcie = "store-nvme-0001-v01"

	// NvmeStoragePcieGen with If the ChromeOS device uses NVMe to provide non-volatile storage, it MUST provide PCie gen3 or higher for NVMe non-volatile storage.
	NvmeStoragePcieGen = "store-nvme-0002-v02"

	// NvmeStorageProtocolVersion with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device MUST support NVMe 1.3 or higher.
	NvmeStorageProtocolVersion = "store-nvme-0004-v01"

	// NvmeStorageHealthInfo with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device MUST provide health info via SMART / Health Information Log.
	NvmeStorageHealthInfo = "store-nvme-0006-v01"

	// NvmeStorage16kReadLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 16k random reads, with iodepth=4 workload.
	NvmeStorage16kReadLatency = "store-nvme-0015-v01"

	// NvmeStorage16kWriteLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 16k random writes, with iodepth=4 workload.
	NvmeStorage16kWriteLatency = "store-nvme-0016-v01"

	// UfsStorageControllerVersion with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS controller MUST support UFS2.1 or higher.
	UfsStorageControllerVersion = "store-ufs-0002-v01"

	// UfsStorageControllerGear with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS controller MUST support Rate-Series-B HS-Gear3 or higher.
	UfsStorageControllerGear = "store-ufs-0004-v01"

	// UfsStorageControllerTxLanes with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS controller SHOULD support 2 Tx lanes.
	UfsStorageControllerTxLanes = "store-ufs-0005-v01"

	// UfsStorageControllerRxLanes with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS controller SHOULD support 2 Rx lanes.
	UfsStorageControllerRxLanes = "store-ufs-0006-v01"

	// UfsStorageDeviceVersion with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device MUST support UFS2.1 or higher.
	UfsStorageDeviceVersion = "store-ufs-0007-v01"

	// UfsStorageDeviceGear with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device MUST support Rate-Series-B HS-Gear3 or higher.
	UfsStorageDeviceGear = "store-ufs-0009-v01"

	// UfsStorageDeviceTxLanes with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device SHOULD support 2 Tx lanes.
	UfsStorageDeviceTxLanes = "store-ufs-0010-v01"

	// UfsStorageDeviceRxLanes with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device SHOULD support 2 Rx lanes.
	UfsStorageDeviceRxLanes = "store-ufs-0011-v01"

	// UfsStorageHealthDescriptor with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device MUST provide health info via Health Descriptor
	UfsStorageHealthDescriptor = "store-ufs-0013-v01"

	// UfsStorageProvisioningType If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage MUST support LUNs bProvisioningType == 03h - thin-provisioning with TPRZ == 1 according to JESD220C 12.2.3.5
	UfsStorageProvisioningType = "store-ufs-0018-v01"

	// UfsWriteBoosterSupport with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage SHOULD support Write Booster feacture according to UFS-3.1 13.4.17
	UfsWriteBoosterSupport = "store-ufs-0020-v01"

	// UfsWriteBoosterSpaceMode with If the ChromeOS device uses UFS to provide non-volatile storage, and its UFS storage supports Write Booster, it MUST support bWriteBoosterBufferPreserveUserSpaceEn == 1 - ‘Preserve user space’ configuration mode for Write Booster Buffer.
	UfsWriteBoosterSpaceMode = "store-ufs-0021-v01"

	// UfsWriteBoosterType with If the ChromeOS device uses UFS to provide non-volatile storage, and its UFS storage supports Write Booster, it MUST support bWriteBoosterBufferType == 1 - ‘Single shared buffer type’ configuration mode for Write Booster Buffer.
	UfsWriteBoosterType = "store-ufs-0022-v01"

	// UfsMinWriteBoosterSize with If the ChromeOS device uses UFS to provide non-volatile storage, and its UFS storage supports Write Booster, it MUST support allocating at least 5% of device’s total capacity as Write Booster buffer.
	UfsMinWriteBoosterSize = "store-ufs-0023-v01"

	// UfsRecomendedWriteBoosterSize with If the ChromeOS device uses UFS to provide non-volatile storage, and its UFS storage supports Write Booster, it SHOULD support allocating at least 10% of device’s total capacity as Write Booster buffer.
	UfsRecomendedWriteBoosterSize = "store-ufs-0024-v01"

	// UfsStorage16kReadLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 16k random reads, with iodepth=4 workload.
	UfsStorage16kReadLatency = "store-ufs-0030-v01"

	// UfsStorage16kWriteLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 16k random writes, with iodepth=4 workload.
	UfsStorage16kWriteLatency = "store-ufs-0031-v01"

	// EmmcStorage4kReadLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 4k random reads, with iodepth=1 workload.
	EmmcStorage4kReadLatency = "store-emmc-0015-v01"

	// EmmcStorage4kWriteLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 4k random writes, with iodepth=1 workload.
	EmmcStorage4kWriteLatency = "store-emmc-0016-v01"

	// EmmcStorage4kQD4ReadLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 4k random reads, with iodepth=4 workload.
	EmmcStorage4kQD4ReadLatency = "store-emmc-0017-v01"

	// EmmcStorage4kQD4WriteLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile of 4k random writes, with iodepth=4 workload.
	EmmcStorage4kQD4WriteLatency = "store-emmc-0018-v01"

	// NvmeStorage4kReadLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random reads, with iodepth=1 workload.
	NvmeStorage4kReadLatency = "store-nvme-0011-v01"

	// NvmeStorage4kWriteLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random writes, with iodepth=1 workload.
	NvmeStorage4kWriteLatency = "store-nvme-0012-v01"

	// NvmeStorage4kQD4ReadLatency with  If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random reads, with iodepth=4 workload.
	NvmeStorage4kQD4ReadLatency = "store-nvme-0013-v01"

	// NvmeStorage4kQD4WriteLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random writes, with iodepth=4 workload.
	NvmeStorage4kQD4WriteLatency = "store-nvme-0014-v01"

	// UfsStorage4kReadLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random reads, with iodepth=1 workload.
	UfsStorage4kReadLatency = "store-ufs-0026-v01"

	// UfsStorage4kWriteLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random writes, with iodepth=1 workload.
	UfsStorage4kWriteLatency = "store-ufs-0027-v01"

	// UfsStorage4kQD4ReadLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random reads, with iodepth=4 workload.
	UfsStorage4kQD4ReadLatency = "store-ufs-0028-v01"

	// UfsStorage4kQD4WriteLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile of 4k random writes, with iodepth=4 workload.
	UfsStorage4kQD4WriteLatency = "store-ufs-0029-v01"

	// NvmeStorageUserSimReadLatency with If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile reads of the user simulating workload.
	NvmeStorageUserSimReadLatency = "store-nvme-0019-v01"

	// NvmeStorageUserSimWriteLatency with  If the ChromeOS device uses NVMe to provide non-volatile storage, its NVMe storage device latency MUST NOT exceed 12ms for the 99th percentile writes of the user simulating workload.
	NvmeStorageUserSimWriteLatency = "store-nvme-0020-v01"

	// EmmcStorageUserSimReadLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile reads of the user simulating workload.
	EmmcStorageUserSimReadLatency = "store-emmc-0023-v01"

	// EmmcStorageUserSimWriteLatency with If the ChromeOS device uses eMMC to provide non-volatile storage, its eMMC storage device latency MUST NOT exceed 35ms for the 99th percentile writes of the user simulating workload.
	EmmcStorageUserSimWriteLatency = "store-emmc-0024-v01"

	// UfsStorageUserSimReadLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile reads of the user simulating workload.
	UfsStorageUserSimReadLatency = "store-ufs-0034-v01"

	// UfsStorageUserSimWriteLatency with If the ChromeOS device uses UFS to provide non-volatile storage, its UFS storage device latency MUST NOT exceed 12ms for the 99th percentile writes of the user simulating workload.
	UfsStorageUserSimWriteLatency = "store-ufs-0035-v01"
)
