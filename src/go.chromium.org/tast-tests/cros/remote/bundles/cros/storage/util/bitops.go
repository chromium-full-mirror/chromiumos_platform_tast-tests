// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

func getBit(s []byte, bit int) int {
	byteNum := bit / 8
	bitNum := bit % 8
	b := int(s[byteNum])
	return (b & (1 << bitNum)) >> bitNum
}

func swap2(b []byte) int {
	return (int(b[1]) << 8) + int(b[0])
}

func swap4(b []byte) int {
	res := int(b[3]) << 24
	res = (int(b[2]) << 16) + res
	res = (int(b[1]) << 8) + res
	res = int(b[0]) + res
	return res
}
