import AppKit

let keys: [String: Int] = ["volup": 0, "voldown": 1, "mute": 7, "play": 16, "next": 17, "prev": 18]
let volumeKeys: Set<String> = ["volup", "voldown", "mute"]

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

let name = CommandLine.arguments.count == 2 ? CommandLine.arguments[1] : ""
guard let key = keys[name] else {
    fail("usage: mediakey \(keys.keys.sorted().joined(separator: "|"))")
}

guard AXIsProcessTrusted(), CGPreflightPostEventAccess() else {
    fail("mediakey: the calling app needs Accessibility (System Settings → Privacy & Security → Accessibility)")
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

func volumeSetting(_ field: String) -> NSAppleEventDescriptor {
    var error: NSDictionary?
    let result = NSAppleScript(source: "\(field) of (get volume settings)")!.executeAndReturnError(&error)
    if let error { fail("mediakey: can't read \(field): \(error)") }
    return result
}

func volume() -> Int {
    volumeSetting("output muted").booleanValue ? 0 : Int(volumeSetting("output volume").int32Value)
}

post(down: true)
post(down: false)
usleep(100_000)

if volumeKeys.contains(name) {
    print(volume())
}
