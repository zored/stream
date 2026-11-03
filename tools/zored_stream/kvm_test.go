package main

import (
	"net"
	"testing"
)

func TestMagicPacket(t *testing.T) {
	mac, _ := net.ParseMAC("ec:3a:56:fa:39:ac")
	p := magicPacket(mac)
	if len(p) != 102 || p[5] != 0xFF || p[6] != 0xEC || p[101] != 0xAC {
		t.Fatalf("bad packet % x", p)
	}
}
