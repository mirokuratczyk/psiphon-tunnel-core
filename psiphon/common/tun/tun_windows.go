// +build windows

/*
 * Copyright (c) 2017, Psiphon Inc.
 * All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package tun

import (
	"net"
	"os"

	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common"
	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
)

const (
	DEFAULT_PUBLIC_INTERFACE_NAME = "" // TODO/miro: for compilation, implement or remove
)

func IsSupported() bool {
	return true
}

func makeDeviceInboundBuffer(MTU int) []byte {
	return make([]byte, MTU)
}

func makeDeviceOutboundBuffer(MTU int) []byte {
	return make([]byte, MTU)
}

func OpenTunDevice(_ string) (*os.File, string, error) {
	return nil, "", errors.Trace(errUnsupported)
}

func (device *Device) readTunPacket() (int, int, error) {
	n, err := device.deviceIO.Read(device.inboundBuffer)
	if err != nil {
		return 0, 0, errors.Trace(err)
	}
	return 0, n, nil
}

func (device *Device) writeTunPacket(packet []byte) error {
	copy(device.outboundBuffer[:], packet)

	size := len(packet)

	_, err := device.deviceIO.Write(device.outboundBuffer[:size])
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func configureNetworkConfigSubprocessCapabilities() error {
	return errors.Trace(errUnsupported)
}

func resetNATTables(_ *ServerConfig, _ net.IP) error {
	return errors.Trace(errUnsupported)
}

func configureServerInterface(_ *ServerConfig, _ string) error {
	return errors.Trace(errUnsupported)
}

func configureClientInterface(_ *ClientConfig, _ string) error {
	return errors.Trace(errUnsupported)
}

func BindToDevice(_ int, _ string) error {
	// TODO/miro: not required ATM because we'll whitelist the process by pid
	// for the POC
	return errors.Trace(errUnsupported)
}

func fixBindToDevice(_ common.Logger, _ bool, _ string) error {
	// Not required
	return nil
}
