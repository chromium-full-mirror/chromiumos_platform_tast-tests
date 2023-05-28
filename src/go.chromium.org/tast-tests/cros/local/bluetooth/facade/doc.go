// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package facade and its subpackages include the BluetoothFacade interface
// and implementations of it for the two different bluetooth stacks ChromeOS
// supports, bluez and floss. These are used to both switch which stack the DUT
// uses and provide a consistent interface for using the stacks. Whenever
// possible, tests should only rely on the BluetoothFacade interface and just
// use NewBluetoothFacade to switch between stacks and get a stack-specific
// implementation of the interface.
package facade
