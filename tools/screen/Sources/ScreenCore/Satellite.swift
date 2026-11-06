import Foundation

public enum Satellite {
    public static let deviceID = "zored-screen"
    public static let productName = "zored screen"
    public static let tileBitmap = 160
    public static let stripBitmap = (w: 160, h: 287)

    public enum Message: Equatable, Sendable {
        case begin
        case keyState(GridID, Data)
        case ping(String)
        case ok
        case error(String)
        case other
    }

    public static func preset(_ id: GridID) -> String? {
        if Layout.strip(of: id) != nil { return "strip" }
        if Layout.tiles.contains(id) { return "tile" }
        return nil
    }

    public static func manifest() -> String {
        var controls: [String: Any] = [:]
        for id in Layout.all {
            var c: [String: Any] = ["row": id.row, "column": id.col]
            if let p = preset(id) { c["stylePreset"] = p }
            controls[id.id] = c
        }
        let doc: [String: Any] = [
            "stylePresets": [
                "default": [String: Any](),
                "tile": ["bitmap": ["w": tileBitmap, "h": tileBitmap]],
                "strip": ["bitmap": ["w": stripBitmap.w, "h": stripBitmap.h]],
            ],
            "controls": controls,
        ]
        let data = try! JSONSerialization.data(withJSONObject: doc, options: [.sortedKeys])
        return data.base64EncodedString()
    }

    public static func addDevice() -> String {
        "ADD-DEVICE DEVICEID=\(deviceID) PRODUCT_NAME=\"\(productName)\" BITMAP_FORMAT=rgb BRIGHTNESS=false LAYOUT_MANIFEST=\(manifest())\n"
    }

    public static func press(_ id: GridID, down: Bool) -> String {
        "KEY-PRESS DEVICEID=\(deviceID) CONTROLID=\(id.id) PRESSED=\(down)\n"
    }

    public static func rotate(_ id: GridID, right: Bool) -> String {
        "KEY-ROTATE DEVICEID=\(deviceID) CONTROLID=\(id.id) DIRECTION=\(right ? 1 : -1)\n"
    }

    public static func pong(_ payload: String) -> String {
        "PONG \(payload)\n"
    }

    public static let removeDevice = "REMOVE-DEVICE DEVICEID=\(deviceID)\n"

    public static func parse(_ line: String) -> Message {
        let space = line.firstIndex(of: " ")
        let cmd = String(space.map { line[..<$0] } ?? Substring(line)).uppercased()
        let rest = space.map { String(line[line.index(after: $0)...]) } ?? ""
        let tokens = tokenize(rest)
        let args = params(tokens)
        switch cmd {
        case "BEGIN":
            return .begin
        case "PING":
            return .ping(rest)
        case "ERROR":
            return .error(args["MESSAGE"] ?? rest)
        case "KEY-STATE":
            guard let raw = args["CONTROLID"], let id = GridID(id: raw),
                  let b64 = args["BITMAP"], let data = Data(base64Encoded: b64) else { return .other }
            return .keyState(id, data)
        default:
            switch tokens.first {
            case "OK": return .ok
            case "ERROR": return .error("\(cmd): \(args["MESSAGE"] ?? "")")
            default: return .other
            }
        }
    }

    static func tokenize(_ s: String) -> [String] {
        var out = [""]
        var quoted = false
        var it = s.makeIterator()
        while let ch = it.next() {
            switch ch {
            case "\\":
                if let next = it.next() { out[out.count - 1].append(next) }
            case "\"":
                quoted.toggle()
            case " " where !quoted:
                out.append("")
            default:
                out[out.count - 1].append(ch)
            }
        }
        return out.filter { !$0.isEmpty }
    }

    static func params(_ tokens: [String]) -> [String: String] {
        var out: [String: String] = [:]
        for t in tokens {
            guard let eq = t.firstIndex(of: "=") else { continue }
            out[String(t[..<eq])] = String(t[t.index(after: eq)...])
        }
        return out
    }
}
