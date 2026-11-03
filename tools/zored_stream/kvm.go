package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

const (
	kvmWindowsInput = "15"
	kvmFlash        = 300 * time.Millisecond
)

type kvm struct {
	vars  *vars
	m1ddc string
}

func newKVM(v *vars, m1ddc string) *kvm {
	k := &kvm{vars: v, m1ddc: m1ddc}
	k.vars.set("kvm_led", "0")
	return k
}

func (k *kvm) windows() error {
	k.vars.set("kvm_led", strconv.Itoa(0xFFFFFF))
	defer time.AfterFunc(kvmFlash, func() { k.vars.set("kvm_led", "0") })
	out, err := exec.Command(k.m1ddc, "set", "input", kvmWindowsInput).CombinedOutput()
	if err != nil {
		return fmt.Errorf("m1ddc set input %s: %w: %s", kvmWindowsInput, err, out)
	}
	return nil
}
