import AppKit

let keys: [String: Int] = ["volup": 0, "voldown": 1, "mute": 7, "play": 16, "next": 17, "prev": 18]

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

guard CommandLine.arguments.count == 2, let key = keys[CommandLine.arguments[1]] else {
    fail("usage: mediakey \(keys.keys.sorted().joined(separator: "|"))")
}

guard AXIsProcessTrusted() else {
    fail("mediakey: no Accessibility permission for the calling app (System Settings → Privacy & Security → Accessibility)")
}

func post(down: Bool) {
    let state = down ? 0xa : 0xb
    guard let event = NSEvent.otherEvent(
        with: .systemDefined,
        location: .zero,
        modifierFlags: NSEvent.ModifierFlags(rawValue: UInt(state << 8)),
        timestamp: 0,
        windowNumber: 0,
        context: nil,
        subtype: 8,
        data1: (key << 16) | (state << 8),
        data2: -1
    ), let cgEvent = event.cgEvent else {
        fail("mediakey: can't create event")
    }
    cgEvent.post(tap: .cghidEventTap)
}

post(down: true)
post(down: false)
