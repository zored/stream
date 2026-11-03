package main

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/pwr_mgt/IOPMLib.h>

static IOReturn holdAwake(IOPMAssertionID *id) {
	return IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleDisplaySleep, kIOPMAssertionLevelOn,
		CFSTR("zored_stream keep-awake"), id);
}

static IOReturn sleepNow(void) {
	io_connect_t port = IOPMFindPowerManagement(MACH_PORT_NULL);
	if (port == MACH_PORT_NULL) {
		return kIOReturnNotFound;
	}
	IOReturn r = IOPMSleepSystem(port);
	IOServiceClose(port);
	return r;
}
*/
import "C"

import "fmt"

type awakeAssertion C.IOPMAssertionID

func holdAwake() (awakeAssertion, error) {
	var id C.IOPMAssertionID
	if r := C.holdAwake(&id); r != C.kIOReturnSuccess {
		return 0, fmt.Errorf("IOPMAssertionCreateWithName: 0x%x", uint32(r))
	}
	return awakeAssertion(id), nil
}

func (a awakeAssertion) release() error {
	if r := C.IOPMAssertionRelease(C.IOPMAssertionID(a)); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMAssertionRelease: 0x%x", uint32(r))
	}
	return nil
}

func sleepSystem() error {
	if r := C.sleepNow(); r != C.kIOReturnSuccess {
		return fmt.Errorf("IOPMSleepSystem: 0x%x", uint32(r))
	}
	return nil
}
