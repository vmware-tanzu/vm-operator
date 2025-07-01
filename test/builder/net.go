// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
)

const (
	minTCPPort         = 0
	maxTCPPort         = 65535
	maxReservedTCPPort = 1024
	maxRandTCPPort     = maxTCPPort - (maxReservedTCPPort + 1)
)

// isTCPPortAvailable returns a flag indicating whether or not a TCP port is
// available.
func isTCPPortAvailable(port int) bool {
	if port < minTCPPort || port > maxTCPPort {
		return false
	}
	conn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// randomTCPPort gets a free, random TCP port between 1025-65535. If no free
// ports are available -1 is returned.
func randomTCPPort() int {
	for i := maxReservedTCPPort; i < maxTCPPort; i++ {
		p, err := rand.Int(rand.Reader, big.NewInt(int64(maxRandTCPPort)))
		// continue generating random port numbers if we hit an error.
		if err != nil {
			continue
		}
		portNum := int(p.Int64()) + maxReservedTCPPort + 1
		if isTCPPortAvailable(portNum) {
			return portNum
		}
	}
	return -1
}
