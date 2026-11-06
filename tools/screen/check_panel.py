#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parent


def main():
    with tempfile.TemporaryDirectory(prefix="screen-panel-check-") as directory:
        work = Path(directory)
        modules = ROOT / ".build/panel-check-modules"
        modules.mkdir(parents=True, exist_ok=True)
        environment = os.environ | {"CLANG_MODULE_CACHE_PATH": str(modules)}
        common = ["swiftc", "-swift-version", "5", "-module-cache-path", str(modules)]
        subprocess.run(common + ["-emit-library", "-emit-module", "-module-name", "ScreenCore"]
                       + [str(path) for path in sorted((ROOT / "Sources/ScreenCore").glob("*.swift"))]
                       + ["-o", str(work / "libScreenCore.dylib")], cwd=work, env=environment, check=True)
        panel = (ROOT / "Sources/screen/Panel.swift").read_text()
        if "NSEvent.mouseLocation" not in panel:
            raise RuntimeError("Panel no longer reads NSEvent.mouseLocation; update the cursor fixture")
        (work / "Panel.swift").write_text(panel.replace("NSEvent.mouseLocation", "fixedMouseLocation()"))
        (work / "main.swift").write_text('''import AppKit
import ScreenCore

var cursorOffset = 0.0

func fixedMouseLocation() -> NSPoint {
    let screen = NSScreen.screens.first!.frame
    let tile = RowLayout.frames(cursorX: nil)[1]
    return NSPoint(x: screen.midX + tile.x + tile.width / 2 + cursorOffset, y: screen.maxY - 20)
}

func require(_ condition: Bool, _ message: String) {
    guard condition else { fputs("FAIL: \\(message)\\n", stderr); exit(1) }
}

func animationsSettled(_ layer: CALayer) -> Bool {
    (layer.animationKeys() ?? []).isEmpty && (layer.sublayers ?? []).allSatisfy(animationsSettled)
}

let application = NSApplication.shared
application.setActivationPolicy(.accessory)
require(NSScreen.screens.first != nil, "no display available")
let panel = Panel(connection: SatelliteConnection())
let rest = RowLayout.frames(cursorX: nil)
panel.show()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(panel.window.isVisible, "show must reveal the panel")
require(!panel.isAnimating, "show animation must settle")
let refreshRate = NSScreen.screens.first!.maximumFramesPerSecond
print("display maximum: \(refreshRate) Hz")
if refreshRate > 60 {
    var scales = Set<Double>()
    let sampler = Timer(timeInterval: 0.001, repeats: true) { _ in scales.insert(panel.frames[1].scale) }
    RunLoop.main.add(sampler, forMode: .common)
    panel.relayout(hover: true)
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.18))
    sampler.invalidate()
    print("distinct hover frames in 180 ms: \(scales.count)")
    require(Double(scales.count) >= Double(refreshRate) * 0.18 * 0.7, "hover must follow high-refresh display cadence")
    panel.relayout()
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
}

panel.relayout(hover: true)
require(panel.frames == rest, "hover must not jump directly to the target")
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.03))
require(panel.frames[1].scale > 1 && panel.frames[1].scale < 2, "hover must interpolate through an intermediate scale")
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(abs(panel.frames[1].scale - 2) < 0.0001, "hover must reach the magnified target")
require(!panel.isAnimating, "hover animation must settle")

cursorOffset = 200
panel.relayout(hover: true)
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(abs(panel.frames[1].scale - 1) < 0.0001, "retarget fixture must start outside tile magnification range")
let retargetStart = ProcessInfo.processInfo.systemUptime
for sample in 1...12 {
    cursorOffset = 200 * (1 - Double(sample) / 12)
    panel.relayout(hover: true)
    let remaining = retargetStart + Double(sample) / 120 - ProcessInfo.processInfo.systemUptime
    if remaining > 0 { RunLoop.main.run(until: Date(timeIntervalSinceNow: remaining)) }
}
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.016))
require(panel.frames[1].scale >= 1.75, "continuous hover must track 120 Hz cursor motion without restarting easing; scale=\\(panel.frames[1].scale)")
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(abs(panel.frames[1].scale - 2) < 0.0001 && !panel.isAnimating, "continuous hover must settle at its final target")

let bitmap = Data(repeating: 255, count: 160 * 160 * 3)
let image = SatelliteConnection.image(bitmap, width: 160, height: 160)!
panel.frame(id: GridID(0, 2), data: bitmap, image: image)
func pictureViews(_ view: NSView) -> [NSView] {
    let own = view.layer?.contents == nil ? [] : [view]
    return own + view.subviews.flatMap(pictureViews)
}
let pictures = pictureViews(panel.window.contentView!)
require(!pictures.isEmpty, "fixture must populate a picture layer")
for picture in pictures {
    require(picture.layer!.masksToBounds, "artwork must be clipped to its own layer")
    require(picture.layer!.cornerRadius > 0 && picture.layer!.cornerRadius <= 8, "artwork radius must match the smaller scaled corners")
    require(picture.bounds.width > 0 && picture.bounds.height > 0, "artwork must fill a nonempty content view")
    require(picture.frame.minX > 0 && picture.frame.minY > 0, "artwork must leave the native glass edge unobstructed")
}

NotificationCenter.default.post(name: NSApplication.didResignActiveNotification, object: application)
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(panel.frames.allSatisfy { abs($0.scale - 1) < 0.0001 }, "focus loss must collapse the magnified row")
require(!panel.isAnimating, "focus loss must stop the animation timer")
if let layer = panel.window.contentView?.layer {
    require(animationsSettled(layer), "focus-loss animation must settle")
}
for unsupportedKnob in [false, true] {
    panel.relayout(hover: true)
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
    panel.relayout()
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.05))
    require(panel.frames[1].scale > 1, "invalid-click fixture must still be collapsing")
    let frame = panel.frames[unsupportedKnob ? 4 : 0]
    let x = frame.x + RowLayout.width(panel.frames) / 2 + (unsupportedKnob ? frame.width / 2 : frame.width + 1)
    let point = panel.window.contentView!.convert(NSPoint(x: x, y: 20), to: nil)
    for type: NSEvent.EventType in unsupportedKnob ? [.rightMouseDown, .rightMouseUp] : [.leftMouseDown, .leftMouseUp] {
        let event = NSEvent.mouseEvent(with: type, location: point, modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime,
                                      windowNumber: panel.window.windowNumber, context: nil, eventNumber: 1, clickCount: 1, pressure: 1)!
        if type == .leftMouseDown || type == .rightMouseDown { panel.input.down(event) }
        else { panel.input.up(event) }
    }
    RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
    require(panel.frames == rest, unsupportedKnob ? "unsupported right click must not freeze collapse" : "transparent gap click must not freeze collapse")
    require(!panel.isAnimating, "invalid click must leave no animation timer")
}
panel.hide()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.04))
require(panel.window.isVisible, "hide must slide before ordering the panel out")
require(panel.isAnimating, "hide must animate at an intermediate time")
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(!panel.window.isVisible, "hide must eventually order the panel out")
require(!panel.isAnimating, "hide animation must settle")
panel.show()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
require(panel.window.isVisible && !panel.isAnimating, "reveal must finish visible with no animation timer")
panel.hide()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.25))
print("panel ok")
''')
        sources = [str(path) for path in sorted((ROOT / "Sources/screen").glob("*.swift"))
                   if path.name not in {"main.swift", "Panel.swift"}]
        subprocess.run(common + ["-I", str(work), "-L", str(work), "-lScreenCore", "-Xlinker", "-rpath",
                                 "-Xlinker", str(work)] + sources
                       + [str(work / "Panel.swift"), str(work / "main.swift"), "-o", str(work / "check-panel")],
                       cwd=work, env=environment, check=True)
        subprocess.run([str(work / "check-panel")], cwd=work, env=environment, check=True, timeout=10)


if __name__ == "__main__":
    main()
