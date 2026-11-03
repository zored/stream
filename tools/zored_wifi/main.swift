import AppKit
import CoreLocation
import CoreWLAN
import Security

let args = Array(CommandLine.arguments.dropFirst())
let out = URL(fileURLWithPath: args.first ?? "/dev/stdout")
let joinSSID = args.count > 2 && args[1] == "join" ? args[2] : nil

func finish(_ text: String, _ code: Int32) -> Never {
    let tmp = out.appendingPathExtension("tmp")
    try? text.write(to: tmp, atomically: false, encoding: .utf8)
    _ = try? FileManager.default.replaceItemAt(out, withItemAt: tmp)
    if !FileManager.default.fileExists(atPath: out.path) { try? FileManager.default.moveItem(at: tmp, to: out) }
    exit(code)
}

func systemPassword(_ ssid: String) throws -> String {
    var keychain: SecKeychain?
    var status = SecKeychainOpen("/Library/Keychains/System.keychain", &keychain)
    guard status == errSecSuccess else { throw NSError(domain: NSOSStatusErrorDomain, code: Int(status)) }
    var length: UInt32 = 0
    var data: UnsafeMutableRawPointer?
    let service = "AirPort"
    status = SecKeychainFindGenericPassword(keychain, UInt32(service.utf8.count), service, UInt32(ssid.utf8.count), ssid, &length, &data, nil)
    guard status == errSecSuccess, let data else {
        throw NSError(domain: NSOSStatusErrorDomain, code: Int(status), userInfo: [NSLocalizedDescriptionKey: "keychain: \(SecCopyErrorMessageString(status, nil) ?? "" as CFString)"])
    }
    defer { SecKeychainItemFreeContent(nil, data) }
    return String(decoding: Data(bytes: data, count: Int(length)), as: UTF8.self)
}

func join(_ ssid: String, _ iface: CWInterface) -> Never {
    do {
        guard let network = try iface.scanForNetworks(withName: ssid).max(by: { $0.rssiValue < $1.rssiValue }) else {
            finish("error\tnot in range\n", 1)
        }
        let open = network.supportsSecurity(.none)
        try iface.associate(to: network, password: open ? nil : try systemPassword(ssid))
        finish("ok\n", 0)
    } catch {
        finish("error\t\(error.localizedDescription)\n", 1)
    }
}

func scan() -> Never {
    guard let iface = CWWiFiClient.shared().interface() else { finish("error\tno wi-fi interface\n", 1) }
    if let ssid = joinSSID { join(ssid, iface) }
    do {
        var lines = ""
        for n in try iface.scanForNetworks(withName: nil) {
            guard let ssid = n.ssid else { finish("error\tssid redacted\n", 1) }
            lines += "\(ssid)\t\(n.rssiValue)\n"
        }
        finish(lines, 0)
    } catch {
        finish("error\t\(error.localizedDescription)\n", 1)
    }
}

final class Auth: NSObject, CLLocationManagerDelegate {
    let manager = CLLocationManager()
    func start() {
        manager.delegate = self
        check()
    }
    func check() {
        switch manager.authorizationStatus {
        case .authorizedAlways, .authorized: scan()
        case .denied, .restricted: finish("error\tlocation denied\n", 1)
        default:
            NSApp.activate(ignoringOtherApps: true)
            manager.requestAlwaysAuthorization()
        }
    }
    func locationManagerDidChangeAuthorization(_ m: CLLocationManager) {
        if m.authorizationStatus != .notDetermined { check() }
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let auth = Auth()
auth.start()
DispatchQueue.main.asyncAfter(deadline: .now() + 120) { finish("error\tlocation prompt timed out\n", 1) }
app.run()
