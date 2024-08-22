// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"context"
	"testing"
	"time"
)

func TestIperfResult(t *testing.T) {
	testcases := []struct {
		output string
		config Config
		expect *Result
		err    bool
	}{
		{
			output: `20240718030119,192.168.0.8,59608,172.16.60.140,5001,3,0.0-10.0,393577800,314824870
20240718030119,172.16.60.140,5001,192.168.0.8,59608,3,0.0-9.8,392785470,320693067,0.114,539,267740,0.201,0
20240718030119,192.168.0.8,5001,172.16.60.140,51402,4,0.0-10.0,368989110,295186328,0.060,3822,254835,1.500,0`,
			config: Config{Version: Version2, Protocol: ProtocolUDP, PortCount: 1, Bidirectional: true},
			expect: &Result{Duration: 9 * time.Nanosecond, Throughput: 615832488, ServerToClient: 615832488, PercentLoss: 0.8505, Jitter: []time.Duration{60 * time.Microsecond}},
		},
		{
			output: `Connecting to host 192.168.0.254, port 5001
[  5] local 192.168.0.8 port 59860 connected to 192.168.0.254 port 5001
[  7] local 192.168.0.8 port 36176 connected to 192.168.0.254 port 5001
[ ID][Role] Interval           Transfer     Bitrate         Jitter    Lost/Total Datagrams
[  5][TX-C]   0.00-1.00   sec  9.35 MBytes  78.4 Mbits/sec            6769
[  7][RX-C]   0.00-1.00   sec  4.90 MBytes  41.1 Mbits/sec  0.385 ms  218/3766 (5.8%)
[  5][TX-C]   1.00-2.00   sec  9.08 MBytes  76.1 Mbits/sec            6573
[  7][RX-C]   1.00-2.00   sec  5.45 MBytes  45.7 Mbits/sec  0.420 ms  0/3948 (0%)
[  5][TX-C]   2.00-3.00   sec  9.30 MBytes  78.0 Mbits/sec            6737
[  7][RX-C]   2.00-3.00   sec  4.57 MBytes  38.4 Mbits/sec  0.439 ms  0/3311 (0%)
[  5][TX-C]   3.00-4.00   sec  9.34 MBytes  78.3 Mbits/sec            6762
[  7][RX-C]   3.00-4.00   sec  4.37 MBytes  36.7 Mbits/sec  0.412 ms  0/3165 (0%)
[  5][TX-C]   4.00-5.00   sec  9.41 MBytes  78.9 Mbits/sec            6814
[  7][RX-C]   4.00-5.00   sec  4.37 MBytes  36.7 Mbits/sec  0.442 ms  0/3166 (0%)
[  5][TX-C]   5.00-6.00   sec  9.27 MBytes  77.8 Mbits/sec            6715
[  7][RX-C]   5.00-6.00   sec  4.35 MBytes  36.5 Mbits/sec  0.423 ms  0/3151 (0%)
[  5][TX-C]   6.00-7.00   sec  9.32 MBytes  78.2 Mbits/sec            6749
[  7][RX-C]   6.00-7.00   sec  4.24 MBytes  35.6 Mbits/sec  0.441 ms  0/3074 (0%)
[  5][TX-C]   7.00-8.00   sec  9.38 MBytes  78.7 Mbits/sec            6790
[  7][RX-C]   7.00-8.00   sec  4.22 MBytes  35.4 Mbits/sec  0.444 ms  0/3054 (0%)
[  5][TX-C]   8.00-9.00   sec  9.32 MBytes  78.2 Mbits/sec            6751
[  7][RX-C]   8.00-9.00   sec  4.40 MBytes  36.9 Mbits/sec  0.373 ms  0/3188 (0%)
[  5][TX-C]   9.00-10.00  sec  9.51 MBytes  79.8 Mbits/sec            6887
[  7][RX-C]   9.00-10.00  sec  4.41 MBytes  37.0 Mbits/sec  0.391 ms  0/3194 (0%)
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID][Role] Interval           Transfer     Bitrate         Jitter    Lost/Total Datagrams
[  5][TX-C]   0.00-10.00  sec  93.3 MBytes  78.2 Mbits/sec  0.000 ms  0/67547 (0%)  sender
[  5][TX-C]   0.00-10.05  sec  42.3 MBytes  35.8 Mbits/sec  0.341 ms  34716/67486 (51%)  receiver
[  7][RX-C]   0.00-10.00  sec  45.6 MBytes  38.3 Mbits/sec  0.000 ms  0/33047 (0%)  sender
[  7][RX-C]   0.00-10.05  sec  45.3 MBytes  37.8 Mbits/sec  0.391 ms  218/33017 (0.66%)  receiver

iperf Done.
`,
			config: Config{Version: Version3, Protocol: ProtocolUDP, PortCount: 1, Bidirectional: true},
			expect: &Result{Duration: 10050 * time.Millisecond, Throughput: 73118608, ClientToServer: 35307272, ServerToClient: 37811336, PercentLoss: 0.3475916141806712, Jitter: []time.Duration{442 * time.Microsecond}},
		},
		{
			output: `Connecting to host 192.168.1.51, port 5001
[  5] local 192.168.0.13 port 37539 connected to 192.168.1.51 port 5001
[  7] local 192.168.0.13 port 33762 connected to 192.168.1.51 port 5001
[ ID][Role] Interval           Transfer     Bitrate         Jitter    Lost/Total Datagrams
[  5][TX-C]   0.00-1.00   sec  96.2 KBytes   788 Kbits/sec            68
[  7][RX-C]   0.00-1.00   sec  67.9 KBytes   556 Kbits/sec  11185108.147 ms  17770/17818 (1e+02%)
[  5][TX-C]   1.00-2.00   sec  76.4 KBytes   626 Kbits/sec            54
[  7][RX-C]   1.00-2.00   sec  14.1 KBytes   116 Kbits/sec  5866178.783 ms  12481/12491 (1e+02%)
[  5][TX-C]   2.00-3.00   sec  1.20 MBytes  10.1 Mbits/sec            870
[  7][RX-C]   2.00-3.00   sec  15.6 KBytes   127 Kbits/sec  2884339.680 ms  11480/11491 (1e+02%)
[  5][TX-C]   3.00-4.00   sec  1.52 MBytes  12.8 Mbits/sec            1101
[  7][RX-C]   3.00-4.00   sec  12.7 KBytes   104 Kbits/sec  1613588.790 ms  9970/9979 (1e+02%)
[  5][TX-C]   4.00-5.00   sec  1.48 MBytes  12.4 Mbits/sec            1070
[  7][RX-C]   4.00-5.00   sec  17.0 KBytes   139 Kbits/sec  743812.590 ms  9488/9500 (1e+02%)
[  5][TX-C]   5.00-6.00   sec  1.44 MBytes  12.1 Mbits/sec            1042
[  7][RX-C]   5.00-6.00   sec  14.1 KBytes   116 Kbits/sec  390151.766 ms  10670/10680 (1e+02%)
[  5][TX-C]   6.00-7.00   sec  1.39 MBytes  11.7 Mbits/sec            1007
[  7][RX-C]   6.00-7.00   sec  14.1 KBytes   116 Kbits/sec  204654.029 ms  9212/9222 (1e+02%)
[  5][TX-C]   7.00-8.00   sec  1.59 MBytes  13.3 Mbits/sec            1148
[  7][RX-C]   7.00-8.00   sec  11.3 KBytes  92.7 Kbits/sec  122175.934 ms  11568/11576 (1e+02%)
[  5][TX-C]   8.00-9.00   sec  1.21 MBytes  10.1 Mbits/sec            876
[  7][RX-C]   8.00-9.00   sec  15.6 KBytes   127 Kbits/sec  60115.227 ms  7327/7338 (1e+02%)
[  5][TX-C]   9.00-10.00  sec  1.38 MBytes  11.5 Mbits/sec            996
[  7][RX-C]   9.00-10.00  sec  14.1 KBytes   116 Kbits/sec  31547.109 ms  9666/9676 (1e+02%)
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID][Role] Interval           Transfer     Bitrate         Jitter    Lost/Total Datagrams
[  5][TX-C]   0.00-10.00  sec  11.4 MBytes  9.54 Mbits/sec  0.000 ms  0/8232 (0%)  sender
[  5][TX-C]   0.00-15.87  sec  11.3 MBytes  5.99 Mbits/sec  1.704 ms  0/8206 (0%)  receiver
[  7][RX-C]   0.00-10.00  sec   214 MBytes   179 Mbits/sec  0.000 ms  0/154640 (0%)  sender
[  7][RX-C]   0.00-15.87  sec   197 KBytes   101 Kbits/sec  31547.109 ms  109632/109771 (1e+02%)  receiver

iperf Done.
`,
			config: Config{Version: Version3, Protocol: ProtocolUDP, PortCount: 1, Bidirectional: true},
			expect: &Result{Duration: 15870 * time.Millisecond, Throughput: 5973080, ClientToServer: 5972984, ServerToClient: 96, PercentLoss: 0.9292658738567687, Jitter: []time.Duration{442 * time.Microsecond}},
		},
		{
			output: `[ ID] Interval           Transfer     Bitrate         Retr
[  5]   0.00-10.00  sec  29.6 MBytes  24.9 Mbits/sec    0             sender
[  5]   0.00-10.02  sec  29.3 MBytes  24.5 Mbits/sec                  receiver
[  7]   0.00-10.00  sec  26.9 MBytes  22.5 Mbits/sec    0             sender
[  7]   0.00-10.02  sec  26.6 MBytes  22.3 Mbits/sec                  receiver
[  9]   0.00-10.00  sec  31.0 MBytes  26.0 Mbits/sec    0             sender
[  9]   0.00-10.02  sec  30.8 MBytes  25.7 Mbits/sec                  receiver
[ 11]   0.00-10.00  sec  27.2 MBytes  22.9 Mbits/sec    0             sender
[ 11]   0.00-10.02  sec  27.0 MBytes  22.6 Mbits/sec                  receiver
[SUM]   0.00-10.00  sec   115 MBytes  96.3 Mbits/sec    0             sender
[SUM]   0.00-10.02  sec   114 MBytes  95.2 Mbits/sec                  receiver

iperf Done.
`,
			config: Config{Version: Version3, Protocol: ProtocolTCP, PortCount: 1, Bidirectional: true},
			expect: &Result{Duration: 10020 * time.Millisecond, Throughput: 95439256, ClientToServer: 0, ServerToClient: 0, PercentLoss: 0.0, Jitter: nil},
		},
	}

	ctx := context.Background()
	for i, tc := range testcases {
		t.Log("Testcase: ", i)
		result, err := newResultFromOutput(ctx, tc.output, &tc.config)
		if (err != nil) != tc.err {
			t.Errorf("newResultFromOutput errors differs; got \"%v\", want error==%v", err, tc.err)
		}
		ok, err := result.Equals(tc.expect)
		if !ok {
			t.Errorf("newResultFromOutput result differs; got %v, want %v, error: %v", result, tc.expect, err)
		}
	}
}
