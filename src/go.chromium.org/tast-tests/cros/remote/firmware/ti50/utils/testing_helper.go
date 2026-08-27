// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

// FirmwareTestingHelperDelegate facilitates FirmwareTestingHelper.
type FirmwareTestingHelperDelegate interface {
	Fatalf(format string, args ...interface{})
}

// FirmwareTestingHelper adds convenience functions for test execution.
type FirmwareTestingHelper struct {
	FirmwareTestingHelperDelegate
}

// MustSucceed aborts the test upon error.
func (fs *FirmwareTestingHelper) MustSucceed(err error, format string, args ...interface{}) {
	if err != nil {
		fs.Fatalf(format+": %s", append(args, err)...)
	}
}

// MustSucceedVal aborts the test upon error, otherwise returns val as-is.
func (fs *FirmwareTestingHelper) MustSucceedVal(val interface{}, err error) interface{} {
	if err != nil {
		fs.Fatalf("MustSucceedVal: %s", err)
	}
	return val
}

// Result stores an Ok and Err value as returned from a `(T, error)` function.
// Use NewResult to construct a new Result.
type Result[T any] struct {
	Ok  T
	Err error
}

// NewResult constructs a new Result from success and error values passed via multi-return.
func NewResult[T any](ok T, err error) Result[T] {
	return Result[T]{ok, err}
}

// MustSucceed checks that the Result has a nil error, and returns the stored `Ok` value.
// This is similar in nature to a Rust `Result::expect`, and is intended to chain with `NewResult`.
func (r Result[T]) MustSucceed(s FirmwareTestingHelperDelegate, format string, args ...interface{}) T {
	(&FirmwareTestingHelper{s}).MustSucceed(r.Err, format, args...)
	return r.Ok
}

// MustSucceedBool aborts the test upon error, otherwise returns val as-is for bools.
func (fs *FirmwareTestingHelper) MustSucceedBool(val bool, err error) bool {
	if err != nil {
		fs.Fatalf("MustSucceedBool: %s", err)
	}
	return val
}

// Crc8 performs crc8 on data in the same way that ti50 fw performs crc8
func Crc8(data []byte) byte {
	crc := uint(0)
	for _, v := range data {
		crc ^= uint(v) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc ^= 0x1070 << 3
			}
			crc <<= 1
		}
	}
	return byte(crc >> 8)
}
