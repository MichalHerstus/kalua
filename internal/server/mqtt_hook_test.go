//go:build !wasm

package server

import (
	mqttpkg "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

// allowAllAuth is a mochi hook that permits every connection, subscription and
// publish (test-only: the broker is used purely as an in-process MQTT sink,
// so authentication/ACL are irrelevant).
type allowAllAuth struct {
	mqttpkg.HookBase
}

func (a *allowAllAuth) ID() string { return "allow-all-auth" }

func (a *allowAllAuth) Provides(b byte) bool {
	return b == mqttpkg.OnConnectAuthenticate || b == mqttpkg.OnACLCheck
}

func (a *allowAllAuth) OnConnectAuthenticate(cl *mqttpkg.Client, pk packets.Packet) bool {
	return true
}

func (a *allowAllAuth) OnACLCheck(cl *mqttpkg.Client, topic string, write bool) bool {
	return true
}
