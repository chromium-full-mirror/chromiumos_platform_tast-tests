// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import "testing"

func TestGetJitterPercentileFromJittermarkResult(t *testing.T) {
	exampleResultText := `Starting test JitterMark
Fake touches: Off
Test initialized
       0, Wait 4 seconds for the test to start...
       0,  Group : JitterMark
 [0] Sample Rate : 48000*
 [1] Samples per Frame : 2*
 [2] Frames per Render : 8*
 [3] Frames per Burst : 96*
 [4] Audio Level: 1=callback, 2=audible : 1*
 [5] ADPF: 0=off, 1=on : 0*
 [6] Note On Delay Seconds : 0*
 [7] Test Start Delay Seconds : 4*
 [8] Number of Seconds : 10*
 [9] Core Affinity : UNSPECIFIED*
 [10] Number of Voices : 10*
 [11] Number of Voices High : 0*
       0, ---- Measure scheduling jitter ---- #voices = 10
    4999,
    4999, RESULTS_BEGIN
    4999, JitterMark = 0
    4999, CSV_BEGIN
    4999,  bin#,  msec,   wakeup#,  wlast,   render#,  rlast
    4999,     0,  0.00,      1761,   4998,       389,   4981
    4999,     1,  0.10,       310,   4986,      3329,   4998
    4999,     2,  0.20,       519,   4975,      1188,   4997
    4999,     3,  0.30,       556,   4948,        61,   4959
    4999,     4,  0.40,      1499,   4974,        20,   4870
    4999,     5,  0.50,       157,   4973,         7,   4505
    4999,     6,  0.60,        76,   4861,         1,    214
    4999,     7,  0.70,        83,   4934,         1,    414
    4999,     8,  0.80,        11,   4582,         1,   1739
    4999,     9,  0.90,         3,   2278,         0,      0
    4999,    10,  1.00,         5,   4774,         2,    108
    4999,    11,  1.10,         2,   3766,         0,      0
    4999,    14,  1.40,         1,    462,         0,      0
    4999,    16,  1.60,         1,      5,         0,      0
    4999,    17,  1.70,         1,    129,         0,      0
    4999,    18,  1.80,         1,     10,         0,      0
    4999,    20,  2.00,         2,    407,         0,      0
    4999,    21,  2.10,         1,   3773,         0,      0
    4999,    25,  2.50,         1,      1,         0,      0
    4999,    26,  2.60,         1,    467,         0,      0
    4999,    28,  2.80,         1,    331,         0,      0
    4999,    36,  3.60,         1,    404,         0,      0
    4999,    38,  3.80,         1,    124,         0,      0
    4999,    41,  4.10,         1,    334,         0,      0
    4999,    45,  4.50,         1,    411,         0,      0
    4999,    60,  6.00,         1,    749,         0,      0
    4999,    66,  6.60,         1,    115,         0,      0
    4999,    88,  8.80,         1,    320,         0,      0
    4999, CSV_END
    4999, average.wakeup.delay.micros = 272.80
    4999, underrun.count = 0
    4999,
    4999, CPU Core Migration
    4999, migration.count = 15
    4999, migration.measurements = 5000
    4999, CSV_BEGIN
    4999,  cpu#,    count,     last
    4999,     0,     4741,     4999
    4999,     1,      259,     4913
    4999, CSV_END
    4999,
    4999, AudioSink:
    4999,   frames.per.burst       = 96
    4999,   scheduler              = SCHED_FIFO # 0x40000001
    4999,   buffer.size.frames     = 768
    4999,   buffer.size.bursts     = 8
    4999,   buffer.capacity.frames = 49152
    4999,   sample.rate            = 48000
    4999,   cpu.affinity           = -1
    4999, RESULTS_END
Finished test JitterMark`

	for _, tc := range []struct {
		resultText string
		wantJitter audioSynthmarkJitterPercentile
		wantError  bool
	}{
		{
			resultText: exampleResultText,
			wantJitter: audioSynthmarkJitterPercentile{
				p50:  200,
				p99:  700,
				p100: 8800,
			},
			wantError: false,
		},
		{
			resultText: "INVALID RESULT TEXT",
			wantError:  true,
		},
	} {
		jitter, err := getJitterPercentileFromJittermarkResult(tc.resultText)
		if tc.wantError {
			if err == nil {
				t.Errorf("getJitterPercentileFromJittermarkResult(%q) unexpectedly succeeded", tc.resultText)
			}
			continue
		}
		if err != nil {
			t.Errorf("getJitterPercentileFromJittermarkResult(%q) unexpectedly failed: %v", tc.resultText, err)
		} else if jitter != tc.wantJitter {
			t.Errorf("getJitterPercentileFromJittermarkResult(%q) = %v; want %v", tc.resultText, jitter, tc.wantJitter)
		}
	}
}
