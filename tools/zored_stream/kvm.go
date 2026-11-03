package main

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	kvmWindowsInput = "15"
	kvmPink         = 0xFF60A0
	kvmFlash        = 300 * time.Millisecond
	kvmPIPOn        = "33"
	kvmPIPOff       = "0"
)

type kvm struct {
	vars  *vars
	m1ddc string
	mac   net.HardwareAddr
	wol   string
	mu    sync.Mutex
	pip   bool
}

func newKVM(v *vars, m1ddc, mac, wol string) (*kvm, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, fmt.Errorf("kvm mac: %w", err)
	}
	k := &kvm{vars: v, m1ddc: m1ddc, mac: hw, wol: wol}
	k.publish()
	return k, nil
}

func (k *kvm) publish() {
	k.vars.set("kvm_led", strconv.Itoa(kvmPink))
}

func (k *kvm) flash() {
	k.vars.set("kvm_led", strconv.Itoa(0xFFFFFF))
	time.AfterFunc(kvmFlash, k.publish)
}

func (k *kvm) windows() error {
	k.flash()
	if err := wake(k.wol, k.mac); err != nil {
		return err
	}
	return k.ddc("input", kvmWindowsInput)
}

func (k *kvm) togglePIP() error {
	k.flash()
	k.mu.Lock()
	defer k.mu.Unlock()
	value := kvmPIPOn
	if k.pip {
		value = kvmPIPOff
	}
	if err := k.ddc("pbp", value); err != nil {
		return err
	}
	k.pip = !k.pip
	return nil
}

func (k *kvm) ddc(control, value string) error {
	out, err := exec.Command(k.m1ddc, "set", control, value).CombinedOutput()
	if err != nil {
		return fmt.Errorf("m1ddc set %s %s: %w: %s", control, value, err, out)
	}
	return nil
}

func magicPacket(mac net.HardwareAddr) []byte {
	return append(bytes.Repeat([]byte{0xFF}, 6), bytes.Repeat(mac, 16)...)
}

func wake(addr string, mac net.HardwareAddr) error {
	conn, err := net.Dial("udp4", addr)
	if err != nil {
		return fmt.Errorf("wake-on-lan: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write(magicPacket(mac)); err != nil {
		return fmt.Errorf("wake-on-lan: %w", err)
	}
	return nil
}
