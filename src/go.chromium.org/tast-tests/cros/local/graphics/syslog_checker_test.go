// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"io/ioutil"
	"strings"
	"testing"
)

// newReader writes messages to a tempfile and return the reader.
func newReader(t *testing.T, messages []string) *syslog.Reader {
	tf, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal("TempFile failed: ", err)
	}
	defer tf.Close()
	opts := append([]syslog.Option{syslog.SourcePath(tf.Name())})
	r, err := syslog.NewReader(context.Background(), opts...)
	if err != nil {
		t.Fatal("NewReader failed: ", err)
	}
	tf.WriteString(strings.Join(messages, "\n") + "\n")
	return r
}

var (
	goodSysLog            = `2019-12-10T11:18:33.123456Z INFO kernel[2133]: hello`
	gpuHangsSysLog        = `2019-12-10T11:18:33.123456Z ERROR kernel[2133]: drm:i915_hangcheck_elapsed`
	amdGpuErrorSysLog     = `2023-06-26T16:23:40.034742Z ERR kernel: [ 1042.139813] [drm:amdgpu_job_run] *ERROR* Error scheduling IBs (-22)`
	kernelSplatsX86SysLog = `2023-11-26T18:33:44.190459Z WARNING kernel: [  123.556283] ------------[ cut here ]------------
2023-11-26T18:33:44.190460Z WARNING kernel: [  123.556292] i915 0000:00:02.0: drm_WARN_ON(common_len <= 0)
2023-11-26T18:33:44.190461Z WARNING kernel: [  123.556319] WARNING: CPU: 1 PID: 6922 at drivers/gpu/drm/i915/display/intel_dp.c:2338 intel_dp_compute_config+0x373/0xd4e
2023-11-26T18:33:44.190461Z WARNING kernel: [  123.556424] RIP: 0010:intel_dp_compute_config+0x373/0xd4e`
	kernelSplatsARMSysLog = `2024-01-31T03:51:48.156587Z INFO tast[5305]: video.PlaybackStress.h264_1080p_30fps: Connecting to Chrome target 21E37A2A3C2915806B22B7F8FF58B590
2024-01-31T03:51:48.159947Z INFO tast[5305]: video.PlaybackStress.h264_1080p_30fps: Browser connection is established
2024-01-31T03:51:48.174867Z WARNING kernel: [  171.878706] ------------[ cut here ]------------
2024-01-31T03:51:48.174892Z WARNING kernel: [  171.878713] list_del corruption, ffffffb9b0cfbba8->next is LIST_POISON1 (dead000000000100)
2024-01-31T03:51:48.174895Z WARNING kernel: [  171.878728] WARNING: CPU: 5 PID: 17548 at lib/list_debug.c:55 __list_del_entry_valid+0xb0/0xfc
2024-01-31T03:51:48.174896Z WARNING kernel: [  171.878729] Modules linked in: 8021q rfcomm algif_hash veth algif_skcipher af_alg lzo_rle lzo_compress zram xt_cgroup uinput xt_MASQUERADE cros_ec_rpmsg mtk_vcodec_dec_hw mtk_vcodec_dec v4l2_h264 mtk_vcodec_enc v4l2_vp9 mtk_vcodec_dbgfs mtk_vcodec_common mtk_vpu uvcvideo videobuf2_vmalloc btusb btmtk btintel btbcm btrtl cros_ec_typec typec mtk_mdp3 videobuf2_dma_contig videobuf2_memops v4l2_mem2mem videobuf2_v4l2 videobuf2_common mtk_scp mtk_rpmsg rpmsg_core mtk_scp_ipi snd_sof_mt8195 snd_sof_xtensa_dsp mtk_adsp_common adsp_pcm snd_sof_of snd_sof snd_sof_utils ip6table_nat fuse mt7921e mt7921_common mt76_connac_lib mt76 mac80211 iio_trig_sysfs cfg80211 bluetooth ecdh_generic ecc cros_ec_lid_angle cros_ec_sensors cros_ec_sensors_core industrialio_triggered_buffer kfifo_buf cros_ec_sensorhub r8153_ecm cdc_ether usbnet r8152 mii joydev
2024-01-31T03:51:48.174899Z WARNING kernel: [  171.878781] CPU: 5 PID: 17548 Comm: ThreadPoolSingl Tainted: G        W         5.10.209-24328-g37ba563d1b4c #1 ab36ef3aa625c90e3eb9500161e4e2515df92c2d
2024-01-31T03:51:48.174901Z WARNING kernel: [  171.878783] Hardware name: MediaTek Tomato board (DT)
2024-01-31T03:51:48.174903Z WARNING kernel: [  171.878786] pstate: 60400089 (nZCv daIf +PAN -UAO -TCO BTYPE=--)
2024-01-31T03:51:48.174904Z WARNING kernel: [  171.878788] pc : __list_del_entry_valid+0xb0/0xfc
2024-01-31T03:51:48.174906Z WARNING kernel: [  171.878790] lr : __list_del_entry_valid+0xac/0xfc
2024-01-31T03:51:48.174908Z WARNING kernel: [  171.878791] sp : ffffffc016c0ba40`
	kernelSplatsVeryLong = `2024-02-06T04:26:01.376714Z WARNING kernel: [   77.983433] ------------[ cut here ]------------
2024-02-06T04:26:01.376715Z WARNING kernel: [   77.983437] i2c i2c-3: Transfer while suspended
2024-02-06T04:26:01.376716Z WARNING kernel: [   77.983454] WARNING: CPU: 6 PID: 129 at drivers/i2c/i2c-core.h:56 __i2c_check_suspended+0x63/0x67
2024-02-06T04:26:01.376719Z WARNING kernel: [   77.983457] Modules linked in: 8021q lzo_rle zram uinput snd_acp_sof_mach snd_acp_mach snd_soc_dmic rfcomm cmac algif_hash algif_skcipher veth af_alg xt_cgroup xt_MASQUERADE btusb snd_sof_amd_renoir snd_sof_xtensa_dsp btrtl snd_sof_amd_acp btintel snd_sof_pci snd_sof btbcm snd_sof_utils snd_acp_pci cros_ec_typec snd_acp_config snd_soc_acpi roles i2c_piix4 typec ip6table_nat snd_pci_acp3x snd_hda_codec_hdmi snd_soc_rt5682s snd_hda_intel snd_intel_dspcfg snd_hda_codec snd_soc_max98357a snd_hwdep snd_hda_core fuse ath11k_pci ath11k qmi_helpers mac80211 cfg80211 bluetooth
2024-02-06T04:26:01.376720Z INFO kernel: [   77.983503] i2c_designware AMDI0019:00: calling acpi_subsys_resume_early+0x0/0x6b @ 5286, parent: platform
2024-02-06T04:26:01.376722Z INFO kernel: [   77.983510] i2c_designware AMDI0010:01: calling acpi_subsys_resume_early+0x0/0x6b @ 5289, parent: platform
2024-02-06T04:26:01.376724Z INFO kernel: [   77.983514] i2c_designware AMDI0010:02: calling acpi_subsys_resume_early+0x0/0x6b @ 5287, parent: platform
2024-02-06T04:26:01.376725Z INFO kernel: [   77.983517] pci 0000:00:00.2: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376762Z INFO kernel: [   77.983521] i2c_designware AMDI0010:00: calling acpi_subsys_resume_early+0x0/0x6b @ 5290, parent: platform
2024-02-06T04:26:01.376763Z INFO kernel: [   77.983524] pci 0000:00:00.0: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376765Z INFO kernel: [   77.983528] pci 0000:00:00.2: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376766Z INFO kernel: [   77.983531] pci 0000:00:00.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376768Z INFO kernel: [   77.983534] chromeos_pstore GOOG9999:00: calling acpi_subsys_resume_early+0x0/0x6b @ 5236, parent: platform
2024-02-06T04:26:01.376769Z INFO kernel: [   77.983537] pci 0000:00:01.0: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376771Z INFO kernel: [   77.983540] pci 0000:00:02.0: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376772Z INFO kernel: [   77.983543] chromeos_pstore GOOG9999:00: acpi_subsys_resume_early+0x0/0x6b returned 0 after 0 usecs
2024-02-06T04:26:01.376774Z INFO kernel: [   77.983546] pci 0000:00:01.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376775Z INFO kernel: [   77.983548] pci 0000:00:02.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376777Z INFO kernel: [   77.983550] amd_gpio AMD0030:00: calling acpi_subsys_resume_early+0x0/0x6b @ 5236, parent: platform
2024-02-06T04:26:01.376778Z INFO kernel: [   77.983554] pcieport 0000:00:02.1: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376780Z INFO kernel: [   77.983555] pcieport 0000:00:02.2: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376781Z INFO kernel: [   77.983559] pcieport 0000:00:02.1: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376783Z INFO kernel: [   77.983560] pcieport 0000:00:02.2: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376784Z INFO kernel: [   77.983563] pcieport 0000:00:02.3: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376785Z INFO kernel: [   77.983565] pci 0000:00:08.0: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376787Z INFO kernel: [   77.983568] pcieport 0000:00:02.3: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376788Z INFO kernel: [   77.983570] pci 0000:00:08.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376806Z INFO kernel: [   77.983599] pci 0000:00:18.1: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376808Z INFO kernel: [   77.983601] dw-apb-uart AMDI0020:01: calling acpi_subsys_resume_early+0x0/0x6b @ 5236, parent: platform
2024-02-06T04:26:01.376810Z INFO kernel: [   77.983603] pci 0000:00:18.2: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376811Z INFO kernel: [   77.983606] pci 0000:00:18.1: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376813Z INFO kernel: [   77.983608] dw-apb-uart AMDI0020:01: acpi_subsys_resume_early+0x0/0x6b returned 0 after 0 usecs
2024-02-06T04:26:01.376815Z INFO kernel: [   77.983610] pci 0000:00:18.2: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376816Z INFO kernel: [   77.983613] pci 0000:00:18.3: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376818Z INFO kernel: [   77.983615] pci 0000:00:18.4: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376819Z INFO kernel: [   77.983618] pci 0000:00:18.3: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376821Z INFO kernel: [   77.983620] cros_ec_lpcs GOOG0004:00: calling acpi_subsys_resume_early+0x0/0x6b @ 5236, parent: PNP0C09:00
2024-02-06T04:26:01.376823Z INFO kernel: [   77.983623] pci 0000:00:18.4: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376824Z INFO kernel: [   77.983626] pci 0000:00:18.5: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376826Z INFO kernel: [   77.983630] pci 0000:00:18.6: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: pci0000:00
2024-02-06T04:26:01.376828Z INFO kernel: [   77.983633] pci 0000:00:18.5: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376830Z INFO kernel: [   77.983638] pci 0000:00:18.6: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376832Z INFO kernel: [   77.983641] pci 0000:00:18.7: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: pci0000:00
2024-02-06T04:26:01.376833Z INFO kernel: [   77.983646] ath11k_pci 0000:01:00.0: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: 0000:00:02.1
2024-02-06T04:26:01.376834Z INFO kernel: [   77.983648] pci 0000:00:18.7: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376836Z INFO kernel: [   77.983652] ath11k_pci 0000:01:00.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376838Z INFO kernel: [   77.983655] sdhci-pci 0000:02:00.0: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: 0000:00:02.2
2024-02-06T04:26:01.376839Z INFO kernel: [   77.983661] nvme 0000:03:00.0: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: 0000:00:02.3
2024-02-06T04:26:01.376841Z INFO kernel: [   77.983663] sdhci-pci 0000:02:00.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376843Z INFO kernel: [   77.983668] nvme 0000:03:00.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376844Z INFO kernel: [   77.983672] amdgpu 0000:04:00.0: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: 0000:00:08.1
2024-02-06T04:26:01.376846Z INFO kernel: [   77.983676] ccp 0000:04:00.2: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: 0000:00:08.1
2024-02-06T04:26:01.376848Z INFO kernel: [   77.983678] amdgpu 0000:04:00.0: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376849Z INFO kernel: [   77.983681] ccp 0000:04:00.2: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376851Z INFO kernel: [   77.983684] xhci_hcd 0000:04:00.3: calling pci_pm_resume_early+0x0/0x29 @ 5281, parent: 0000:00:08.1
2024-02-06T04:26:01.376852Z INFO kernel: [   77.983687] xhci_hcd 0000:04:00.4: calling pci_pm_resume_early+0x0/0x29 @ 5282, parent: 0000:00:08.1
2024-02-06T04:26:01.376854Z INFO kernel: [   77.983690] xhci_hcd 0000:04:00.3: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376856Z INFO kernel: [   77.983693] xhci_hcd 0000:04:00.4: pci_pm_resume_early+0x0/0x29 returned 0 after 0 usecs
2024-02-06T04:26:01.376872Z INFO kernel: [   77.983753] i2c i2c-6: calling i2c_resume_early+0x0/0x55 @ 5284, parent: 0000:04:00.0
2024-02-06T04:26:01.376873Z INFO kernel: [   77.983755] i2c i2c-5: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376878Z INFO kernel: [   77.983757] i2c i2c-4: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376880Z INFO kernel: [   77.983763] i2c i2c-7: calling i2c_resume_early+0x0/0x55 @ 5275, parent: 0000:04:00.0
2024-02-06T04:26:01.376881Z INFO kernel: [   77.983765] i2c i2c-6: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376883Z INFO kernel: [   77.983770] i2c i2c-7: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376885Z WARNING kernel: [   77.983800]  ecdh_generic ecc r8153_ecm cdc_ether usbnet r8152 mii uvcvideo videobuf2_vmalloc videobuf2_memops videobuf2_v4l2 videobuf2_common
2024-02-06T04:26:01.376886Z INFO kernel: [   77.983812] i2c i2c-11: calling i2c_resume_early+0x0/0x55 @ 5284, parent: 0000:00:14.0
2024-02-06T04:26:01.376888Z INFO kernel: [   77.983816] i2c i2c-12: calling i2c_resume_early+0x0/0x55 @ 5257, parent: 0000:00:14.0
2024-02-06T04:26:01.376890Z WARNING kernel: [   77.983818]  joydev
2024-02-06T04:26:01.376891Z INFO kernel: [   77.983821] i2c i2c-13: calling i2c_resume_early+0x0/0x55 @ 205, parent: 0000:00:14.0
2024-02-06T04:26:01.376893Z INFO kernel: [   77.983823] i2c i2c-12: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376894Z INFO kernel: [   77.983826] i2c i2c-11: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376895Z INFO kernel: [   77.983829] i2c i2c-13: i2c_resume_early+0x0/0x55 returned 0 after 0 usecs
2024-02-06T04:26:01.376897Z WARNING kernel: [   77.983839]
2024-02-06T04:26:01.376939Z WARNING kernel: [   77.983844] CPU: 6 PID: 129 Comm: hwrng Not tainted 5.10.209-24386-g9aebbffff7f8 #1 361ff4ce2750578a0d0f6bde84f484089ad412ad
2024-02-06T04:26:01.376941Z WARNING kernel: [   77.983847] Hardware name: Google Nipperkin/Nipperkin, BIOS Google_Nipperkin.14500.358.0 03/30/2023
2024-02-06T04:26:01.376942Z WARNING kernel: [   77.983850] RIP: 0010:__i2c_check_suspended+0x63/0x67`
	mediatekIOMMUSysLog = `2023-09-03T14:32:45.835792Z ERR kernel: [ 4094.326174] mtk-iommu 1401d000.m4u: fault type=0x280 iova=0x1ff000000 pa=0x0 read`
	mediatekV4L2SysLog  = `2023-04-16T23:06:49.312229Z ERR kernel: [169036.056750] mtk_vcodec_dec_pw_off(),77: [MTK_V4L2][ERROR] pm_runtime_put_sync fail -22`
	qualcommVideoSysLog = `2023-11-09T14:55:16.022617Z ERR kernel: [  951.666894] qcom-venus-decoder aa00000.video-codec:video-decoder: dec: event session error 0`
)

func TestDisableSysLogCheck(t *testing.T) {
	for _, tc := range []struct {
		testName         string
		ignoreCategories []SysLogCategory
		messages         []string
		expectedErr      bool
	}{
		{"nilIgnore", nil, []string{gpuHangsSysLog}, false},
		{"emptyIgnore", []SysLogCategory{}, []string{gpuHangsSysLog}, true},
		{"ignoreGpuHangs", []SysLogCategory{SysLogGpuHangs}, []string{gpuHangsSysLog}, false},
		{"ignoreGpuHangsAndKernelSplats", []SysLogCategory{SysLogGpuHangs, SysLogKernelSplats}, []string{gpuHangsSysLog, kernelSplatsX86SysLog}, false},
		{"ignoreKernelSplatsButGpuHangs", []SysLogCategory{SysLogKernelSplats}, []string{gpuHangsSysLog, kernelSplatsX86SysLog}, true},
		{"ignoreGpuHangsButKernelSplats", []SysLogCategory{SysLogGpuHangs}, []string{gpuHangsSysLog, kernelSplatsX86SysLog}, true},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			if tc.ignoreCategories == nil {
				DisableSysLogCheck(tc.testName)
			} else {
				DisableSysLogCheck(tc.testName, tc.ignoreCategories...)
			}
			reader := newReader(t, tc.messages)
			err := CheckSysLog(context.Background(), tc.testName, reader)
			if err == nil && tc.expectedErr {
				t.Error("Expect to see syslog error but found none")
			}
			if err != nil && !tc.expectedErr {
				t.Errorf("Category %v should be ignored but found: %v", tc.ignoreCategories, err)
			}
		})
	}

}

func TestCheckSysLog(t *testing.T) {
	for _, tc := range []struct {
		testName     string
		messages     []string
		expectResult string
	}{
		{"pass", []string{goodSysLog}, ""},
		{"gpuHangs", []string{gpuHangsSysLog}, "GPU hangs: drm:i915_hangcheck_elapsed"},
		{"amdGpuError", []string{amdGpuErrorSysLog}, "AMDGPU error: [drm:amdgpu_job_run] *ERROR* Error scheduling IBs (-22)"},
		{"kernelSplats", []string{kernelSplatsX86SysLog}, "Kernel splats: intel_dp_compute_config+0x373/0xd4e"},
		{"kernelSplatsARM", []string{kernelSplatsARMSysLog}, "Kernel splats: __list_del_entry_valid+0xb0/0xfc"},
		{"kernelSplatsVeryLong", []string{kernelSplatsVeryLong}, "Kernel splats: __i2c_check_suspended+0x63/0x67"},
		{"mediatekIOMMU", []string{mediatekIOMMUSysLog}, "Mediatek IOMMU fault: mtk-iommu 1401d000.m4u: fault type=0x280 iova=0x1ff000000 pa=0x0 read"},
		{"mediatekVideo", []string{mediatekV4L2SysLog}, "Mediatek video error: mtk_vcodec_dec_pw_off(),77: [MTK_V4L2][ERROR] pm_runtime_put_sync fail -22"},
		{"qualcommVideo", []string{qualcommVideoSysLog}, "Qualcomm video error: qcom-venus-decoder aa00000.video-codec:video-decoder: dec: event session error 0"},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			reader := newReader(t, tc.messages)
			err := CheckSysLog(context.Background(), tc.testName, reader)
			if err == nil && tc.expectResult != "" {
				t.Errorf("Expect: %q, got: %q", tc.expectResult, err)
			}
			if err != nil && tc.expectResult != err.Error() {
				t.Errorf("Expect: %q, got: %q", tc.expectResult, err)
			}
		})
	}
}
