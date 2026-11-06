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

if failures > 0 { exit(1) }
print("ok")
