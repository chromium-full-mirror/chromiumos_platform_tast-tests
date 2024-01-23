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
