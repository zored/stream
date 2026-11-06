import Foundation
import IOKit

final class Device {
    private let port: IONotificationPortRef
    private var added: io_iterator_t = 0
    private var removed: io_iterator_t = 0
    private var services = Set<UInt64>()
    private let onChange: (Bool) -> Void
    private var ready = false

    init(onChange: @escaping (Bool) -> Void) {
        self.onChange = onChange
        guard let port = IONotificationPortCreate(kIOMainPortDefault) else {
            fail("cannot create USB notification port")
        }
        self.port = port
        CFRunLoopAddSource(CFRunLoopGetMain(), IONotificationPortGetRunLoopSource(port).takeUnretainedValue(), .defaultMode)
        let context = Unmanaged.passUnretained(self).toOpaque()
        let match: () -> CFDictionary = {
            let dictionary = IOServiceMatching("IOUSBDevice")! as NSMutableDictionary
            dictionary["idVendor"] = 0x1532
            dictionary["idProduct"] = 0x0D06
            return dictionary
        }
        let addResult = IOServiceAddMatchingNotification(port, kIOFirstMatchNotification, match(), { context, iterator in
            Unmanaged<Device>.fromOpaque(context!).takeUnretainedValue().drain(iterator, adding: true)
        }, context, &added)
        guard addResult == KERN_SUCCESS else { fail("USB match notification: \(addResult)") }
        drain(added, adding: true)
        let removeResult = IOServiceAddMatchingNotification(port, kIOTerminatedNotification, match(), { context, iterator in
            Unmanaged<Device>.fromOpaque(context!).takeUnretainedValue().drain(iterator, adding: false)
        }, context, &removed)
        guard removeResult == KERN_SUCCESS else { fail("USB termination notification: \(removeResult)") }
        drain(removed, adding: false)
        ready = true
        onChange(!services.isEmpty)
    }

    private func drain(_ iterator: io_iterator_t, adding: Bool) {
        let wasPlugged = !services.isEmpty
        while true {
            let service = IOIteratorNext(iterator)
            guard service != 0 else { break }
            var id: UInt64 = 0
            let result = IORegistryEntryGetRegistryEntryID(service, &id)
            IOObjectRelease(service)
            guard result == KERN_SUCCESS else { fail("USB registry identifier: \(result)") }
            if adding { services.insert(id) } else { services.remove(id) }
        }
        if ready && wasPlugged != !services.isEmpty { onChange(!services.isEmpty) }
    }

    deinit {
        IOObjectRelease(added)
        IOObjectRelease(removed)
        IONotificationPortDestroy(port)
    }
}
