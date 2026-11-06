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

if failures > 0 { exit(1) }
print("ok")
