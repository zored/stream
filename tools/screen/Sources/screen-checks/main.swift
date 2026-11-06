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
check((presets["strip"]?["bitmap"] as? [String: Int]) == ["w": 160, "h": 287], "strip bitmap")
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

let rest = RowLayout.frames(cursorX: nil)
check(rest.count == 14, "row elements")
check(abs(RowLayout.width(rest) - 979) < 1e-9, "row width at rest")
check(rest.allSatisfy { $0.scale == 1 }, "no magnification at rest")
check(rest.first?.element == .strip(.left) && rest.last?.element == .strip(.right), "strips at ends")
check(rest[1].width == 40 && rest[1].height == 40 && rest[0].width == 215 && rest[0].height == 40, "small sizes")
check(abs((rest.first!.x + RowLayout.width(rest) / 2)) < 1e-9, "row centered on 0")
let tile1 = rest[1]
let mag = RowLayout.frames(cursorX: tile1.x + tile1.width / 2)
check(mag[1].width == 80 && mag[1].height == 80, "tile under cursor at max")
check(mag[13].scale == 1, "far strip unscaled")
check(mag[0].scale > 1 && mag[0].scale < 2, "near strip partly scaled")
check(zip(mag, mag.dropFirst()).allSatisfy { $0.x + $0.width < $1.x }, "no overlap")
check(RowLayout.hit(mag, x: mag[1].x + 1) == mag[1].element, "hit tile")
check(RowLayout.hit(mag, x: mag[0].x - 1) == nil, "miss")

let px: (UInt8) -> [UInt8] = { [$0, $0, $0] }
let cells = [Data(px(1) + px(2)), Data(px(3) + px(4)), Data(px(5) + px(6))]
let leftStrip = Strip.compose(cells, cellW: 2, cellH: 1, side: .left)
check(leftStrip == Data(px(2) + px(4) + px(6) + px(1) + px(3) + px(5)), "left strip: device top left, device left bottom")
let rightStrip = Strip.compose(cells, cellW: 2, cellH: 1, side: .right)
check(rightStrip == Data(px(5) + px(3) + px(1) + px(6) + px(4) + px(2)), "right strip: device top right, device left top")
check(Strip.compose([nil, cells[1], nil], cellW: 2, cellH: 1, side: .left) == Data(px(0) + px(4) + px(0) + px(0) + px(3) + px(0)), "missing cells black")

var heldControls = HeldControls()
check(heldControls.down(button: 0, id: GridID(0, 2)), "initial mouse down")
check(!heldControls.down(button: 0, id: GridID(0, 3)), "duplicate mouse down rejected")
check(heldControls.up(button: 0) == GridID(0, 2), "release belongs to original press")
check(heldControls.up(button: 0) == nil, "release only once")
check(heldControls.down(button: 1, id: GridID(1, 0)), "knob held")
check(heldControls.releaseAll() == [GridID(1, 0)], "release all held controls")
check(heldControls.releaseAll().isEmpty, "release all clears held controls")
var wheelSteps = WheelSteps()
check(wheelSteps.consume(delta: 25, precise: true, inverted: false) == 2, "trackpad 10 points per step")
check(wheelSteps.consume(delta: 5, precise: true, inverted: false) == 1, "trackpad remainder")
check(wheelSteps.consume(delta: 1, precise: false, inverted: true) == -1, "undo natural scrolling")

check(wheelSteps.consume(delta: 0.5, precise: false, inverted: false) == 0, "fractional mouse delta retained")
check(wheelSteps.consume(delta: 0.5, precise: false, inverted: false) == 1, "fractional mouse deltas accumulate")

check(ScreenVisibility.shouldShow(devicePlugged: true, userHidden: false), "panel stays visible with controller plugged")
check(ScreenVisibility.shouldShow(devicePlugged: false, userHidden: false), "panel visible unplugged")
check(!ScreenVisibility.shouldShow(devicePlugged: true, userHidden: true), "user hide wins while plugged")

check(Motion.hoverDuration == 0.16 && Motion.visibilityDuration == 0.18, "motion durations")
check(Motion.value(from: -80, to: 0, progress: -1) == -80, "slide clamps before start")
check(Motion.value(from: -80, to: 0, progress: 2) == 0, "slide clamps after finish")
check(Motion.value(from: -80, to: 0, progress: 0.5) == -40, "slide midpoint")
check(Motion.value(from: 0, to: 1, progress: 0.25) == 0.15625, "smoothstep quarter")
let slideValues = (0...10).map { Motion.value(from: -80, to: 0, progress: Double($0) / 10) }
check(zip(slideValues, slideValues.dropFirst()).allSatisfy { $0 <= $1 }, "slide progresses monotonically")
check(Motion.frames(from: rest, to: mag, progress: 0) == rest, "motion exact start frames")
check(Motion.frames(from: rest, to: mag, progress: 1) == mag, "motion exact final frames")
check(Motion.frames(from: rest, to: mag, progress: -1) == rest, "frame motion clamps before start")
check(Motion.frames(from: rest, to: mag, progress: 2) == mag, "frame motion clamps after end")
let halfwayFrames = Motion.frames(from: rest, to: mag, progress: 0.5)
check(halfwayFrames.map(\.element) == rest.map(\.element), "motion preserves element identity")
for i in rest.indices {
    check(halfwayFrames[i].x == (rest[i].x + mag[i].x) / 2, "motion midpoint x")
    check(halfwayFrames[i].width == (rest[i].width + mag[i].width) / 2, "motion midpoint width")
    check(halfwayFrames[i].height == (rest[i].height + mag[i].height) / 2, "motion midpoint height")
    check(halfwayFrames[i].scale == (rest[i].scale + mag[i].scale) / 2, "motion midpoint scale")
}
let motionSamples = (0...10).map { Motion.frames(from: rest, to: mag, progress: Double($0) / 10) }
for i in rest.indices {
    for component in [\ElementFrame.x, \ElementFrame.width, \ElementFrame.height, \ElementFrame.scale] {
        let values = motionSamples.map { $0[i][keyPath: component] }
        let increases = rest[i][keyPath: component] <= mag[i][keyPath: component]
        check(zip(values, values.dropFirst()).allSatisfy { increases ? $0 <= $1 : $0 >= $1 }, "frame components progress monotonically")
    }
}

if failures > 0 { exit(1) }
print("ok")
