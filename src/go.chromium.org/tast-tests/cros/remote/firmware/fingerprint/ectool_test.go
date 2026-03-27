// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import "testing"

func TestRollbackStateV0EctoolUnmarshaler(t *testing.T) {
	// Note that the following test string is not exactly what ectool would
	// emit, since it contains tabs at the beginning of each line and includes
	// a few extra newlines. These tabs and newlines are purely consmetic.
	var out = []byte(`
	Rollback block id:    19
	Rollback min version: 0
	RW rollback version:  255
	`)
	var rExpect = RollbackState{BlockID: 19, MinVersion: 0, RWVersion: 255, SecretInitialized: SecretInitializedUnknown}

	var r RollbackState
	if err := r.UnmarshalerEctool(out); err != nil {
		t.Fatal("Failed to unmarshal: ", err)
	}

	if r != rExpect {
		t.Fatalf("Unmarshaled rollback block %+v doesn't match expected block %+v.", r, rExpect)
	}
}

func TestRollbackStateV1EctoolUnmarshalerSecretInitialized(t *testing.T) {
	// Note that the following test string is not exactly what ectool would
	// emit, since it contains tabs at the beginning of each line and includes
	// a few extra newlines. These tabs and newlines are purely consmetic.
	var out = []byte(`
	Rollback block id:    19
	Rollback min version: 0
	RW rollback version:  255
	Secret initialized:   1
	`)
	var rExpect = RollbackState{BlockID: 19, MinVersion: 0, RWVersion: 255, SecretInitialized: SecretInitializedTrue}

	var r RollbackState
	if err := r.UnmarshalerEctool(out); err != nil {
		t.Fatal("Failed to unmarshal: ", err)
	}

	if r != rExpect {
		t.Fatalf("Unmarshaled rollback block %+v doesn't match expected block %+v.", r, rExpect)
	}
}

func TestRollbackStateV1EctoolUnmarshalerSecretNotInitialized(t *testing.T) {
	// Note that the following test string is not exactly what ectool would
	// emit, since it contains tabs at the beginning of each line and includes
	// a few extra newlines. These tabs and newlines are purely consmetic.
	var out = []byte(`
	Rollback block id:    19
	Rollback min version: 0
	RW rollback version:  255
	Secret initialized:   0
	`)
	var rExpect = RollbackState{BlockID: 19, MinVersion: 0, RWVersion: 255, SecretInitialized: SecretInitializedFalse}

	var r RollbackState
	if err := r.UnmarshalerEctool(out); err != nil {
		t.Fatal("Failed to unmarshal: ", err)
	}

	if r != rExpect {
		t.Fatalf("Unmarshaled rollback block %+v doesn't match expected block %+v.", r, rExpect)
	}
}

func TestRollbackStateEctoolUnmarshalerError(t *testing.T) {
	var out = []byte(`
	Rollback block id:    19
	Rollback min version: 0F
	RW rollback version:  255
	`)

	var r RollbackState
	if err := r.UnmarshalerEctool(out); err == nil {
		t.Fatalf("Failed to detect error in rollback min version. Produced rollback block %+v.", r)
	}
}

func TestEctoolFlagsUnmarshaler(t *testing.T) {
	var out = `0x00000c02`
	var expect uint32 = 0xc02

	actual, err := UnmarshalEctoolFlags(out)
	if err != nil {
		t.Fatal("Failed to unmarshal ectool flags: ", err)
	}

	if actual != expect {
		t.Fatalf("Unmarshaled ectool flags  %+v doesn't match expected flags %+v.", actual, expect)
	}
}

func TestEctoolFlagsUnmarshalerLargerThanUint32(t *testing.T) {
	var out = `0x100000c02`

	_, err := UnmarshalEctoolFlags(out)
	if err == nil {
		t.Fatal("Expected parsing error for numbers larger than uint32")
	}
}
