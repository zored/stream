import AppKit
import CoreLocation
import CoreWLAN

let out = URL(fileURLWithPath: CommandLine.arguments.dropFirst().first ?? "/dev/stdout")

func finish(_ text: String, _ code: Int32) -> Never {
    let tmp = out.appendingPathExtension("tmp")
    try? text.write(to: tmp, atomically: false, encoding: .utf8)
    _ = try? FileManager.default.replaceItemAt(out, withItemAt: tmp)
    if !FileManager.default.fileExists(atPath: out.path) { try? FileManager.default.moveItem(at: tmp, to: out) }
    exit(code)
}

func scan() -> Never {
    guard let iface = CWWiFiClient.shared().interface() else { finish("error\tno wi-fi interface\n", 1) }
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
