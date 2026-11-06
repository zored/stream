import Foundation
import ScreenCore

nonisolated(unsafe) var failures = 0

func check(_ cond: Bool, _ msg: String, line: Int = #line) {
    if !cond {
        print("FAIL line \(line): \(msg)")
        failures += 1
    }
}

check(Layout.tiles.map(\.id) == ["0/2", "1/2", "2/2", "0/3", "1/3", "2/3", "0/4", "1/4", "2/4", "0/5", "1/5", "2/5"], "tiles order")
check(Layout.knob(forTile: 1) == GridID(0, 0), "knob 1")
check(Layout.knob(forTile: 3) == GridID(2, 0), "knob 3")
check(Layout.knob(forTile: 10) == GridID(0, 7), "knob 10")
check(Layout.knob(forTile: 12) == GridID(2, 7), "knob 12")
check(Layout.knob(forTile: 5) == nil, "knob 5")
check(Layout.button(forTile: 1) == GridID(3, 0), "button 1")
check(Layout.button(forTile: 8) == GridID(3, 7), "button 8")
check(Layout.button(forTile: 9) == nil, "button 9")
check(Layout.stripCells(.left).map(\.id) == ["0/1", "1/1", "2/1"], "left strip")
check(Layout.stripCells(.right).map(\.id) == ["0/6", "1/6", "2/6"], "right strip")

check(Geometry.deviceY(side: .left, x: 0, width: 215) == 0, "left y 0")
check(Geometry.deviceY(side: .left, x: 215, width: 215) == 270, "left y end")
check(Geometry.deviceY(side: .right, x: 0, width: 215) == 270, "right y 0")
check(Geometry.deviceY(side: .right, x: 215, width: 215) == 0, "right y end")
check(Geometry.deviceY(side: .left, x: -5, width: 215) == 0, "clamp")
check(Geometry.scale(distance: 0, small: 40, max: 80, range: 200) == 2.0, "scale 0")
check(Geometry.scale(distance: 200, small: 40, max: 80, range: 200) == 1.0, "scale range")
check(abs(Geometry.scale(distance: 100, small: 40, max: 80, range: 200) - 1.5) < 1e-9, "scale half")
check(Geometry.swipe(dx: 50, dy: 3, ms: 100) == "r", "swipe r")
check(Geometry.swipe(dx: -3, dy: -45, ms: 100) == "u", "swipe u")
check(Geometry.swipe(dx: 30, dy: 0, ms: 100) == nil, "swipe short")
check(Geometry.swipe(dx: 60, dy: 0, ms: 500) == nil, "swipe slow")

let manifestJSON = try! JSONSerialization.jsonObject(with: Data(base64Encoded: Satellite.manifest())!) as! [String: Any]
let controls = manifestJSON["controls"] as! [String: [String: Any]]
let presets = manifestJSON["stylePresets"] as! [String: [String: Any]]
check(controls.count == 32, "manifest controls")
check(controls["0/1"]?["stylePreset"] as? String == "strip", "strip preset")
check(controls["1/3"]?["stylePreset"] as? String == "tile", "tile preset")
check(controls["3/0"]?["stylePreset"] == nil, "button default preset")
check(controls["2/7"]?["row"] as? Int == 2 && controls["2/7"]?["column"] as? Int == 7, "row/column")
check((presets["tile"]?["bitmap"] as? [String: Int]) == ["w": 160, "h": 160], "tile bitmap")
check((presets["strip"]?["bitmap"] as? [String: Int]) == ["w": 120, "h": 180], "strip bitmap")
check(presets["default"]?.isEmpty == true, "default preset")
check(Satellite.addDevice().hasPrefix("ADD-DEVICE DEVICEID=zored-screen PRODUCT_NAME=\"zored screen\" "), "add device")
check(Satellite.addDevice().contains(" BITMAP_FORMAT=rgb ") && Satellite.addDevice().contains(" LAYOUT_MANIFEST=\(Satellite.manifest())\n"), "add device params")
check(Satellite.addDevice().hasSuffix("\n"), "add device newline")
check(Satellite.press(GridID(0, 2), down: true) == "KEY-PRESS DEVICEID=zored-screen CONTROLID=0/2 PRESSED=true\n", "press")
check(Satellite.press(GridID(0, 2), down: false) == "KEY-PRESS DEVICEID=zored-screen CONTROLID=0/2 PRESSED=false\n", "release")
check(Satellite.rotate(GridID(1, 0), right: false) == "KEY-ROTATE DEVICEID=zored-screen CONTROLID=1/0 DIRECTION=-1\n", "rotate left")
check(Satellite.rotate(GridID(1, 0), right: true) == "KEY-ROTATE DEVICEID=zored-screen CONTROLID=1/0 DIRECTION=1\n", "rotate right")
check(Satellite.parse("PING 1") == .ping("1"), "ping")
check(Satellite.pong("1") == "PONG 1\n", "pong")
check(Satellite.parse("KEY-STATE DEVICEID=zored-screen CONTROLID=0/2 BITMAP=AAAA") == .keyState(GridID(0, 2), Data([0, 0, 0])), "key-state plain")
check(Satellite.parse("KEY-STATE DEVICEID=\"zored-screen\" CONTROLID=\"0/2\" LOCATION=\"1/0/2\" PRESSED=0 TYPE=\"BUTTON\" BITMAP=\"AAAA\"") == .keyState(GridID(0, 2), Data([0, 0, 0])), "key-state quoted")
check(Satellite.parse("KEY-STATE DEVICEID=\"zored-screen\" CONTROLID=\"0/2\" PRESSED=0") == .other, "key-state no bitmap")
check(Satellite.parse("BEGIN CompanionVersion=\"5.1.1\" ApiVersion=\"1.14.0\"") == .begin, "begin")
check(Satellite.parse("ADD-DEVICE OK DEVICEID=\"zored-screen\"") == .ok, "ok")
check(Satellite.parse("KEY-PRESS ERROR DEVICEID=\"zored-screen\" MESSAGE=\"Unknown CONTROLID\"") == .error("KEY-PRESS: Unknown CONTROLID"), "error")
check(Satellite.parse("ERROR MESSAGE=\"Unknown command: X\"") == .error("Unknown command: X"), "bare error")
check(Satellite.parse("BRIGHTNESS DEVICEID=\"zored-screen\" VALUE=50") == .other, "other")
check(Satellite.parse("KEY-PRESS ERROR DEVICEID=\"x\" MESSAGE=\"a \\\"b\\\" c\"") == .error("KEY-PRESS: a \"b\" c"), "escaped quotes")

if failures > 0 { exit(1) }
print("ok")
