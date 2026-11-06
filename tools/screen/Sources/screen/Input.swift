import AppKit
import ScreenCore

final class Input {
    private final class Gesture {
        let frame: ElementFrame
        let point: NSPoint
        let time: TimeInterval
        let button: Int
        var y: Int?
        var swipeSent = false
        var pending = false
        var released = false
        var cancelled = false
        init(frame: ElementFrame, point: NSPoint, event: NSEvent) {
            self.frame = frame
            self.point = point
            time = event.timestamp
            button = event.buttonNumber
        }
    }
    private unowned let panel: Panel
    private let connection: SatelliteConnection
    private var held = HeldControls()
    private var gestures: [Int: Gesture] = [:]
    private var pending: [Gesture] = []
    private var wheelSteps = WheelSteps()
    private var wheelTarget: Element?
    var isHeld: Bool { !gestures.isEmpty }

    init(panel: Panel, connection: SatelliteConnection) {
        self.panel = panel
        self.connection = connection
    }

    func down(_ event: NSEvent) {
        let point = panel.point(event)
        guard let frame = panel.hit(point), gestures[event.buttonNumber] == nil else { return }
        if case .strip = frame.element, event.buttonNumber == 0, event.clickCount == 2 {
            releaseAll()
            panel.onStripDoubleClick()
            return
        }
        let gesture = Gesture(frame: frame, point: point, event: event)
        switch frame.element {
        case let .tile(n, grid):
            let id: GridID?
            switch event.buttonNumber {
            case 0: id = grid
            case 1: id = Layout.knob(forTile: n)
            case 2: id = Layout.button(forTile: n)
            default: id = nil
            }
            guard let id, held.down(button: event.buttonNumber, id: id) else { return }
            panel.freezeLayout()
            connection.press(id, down: true)
        case let .strip(side):
            guard event.buttonNumber == 0 else { return }
            panel.freezeLayout()
            let y = panel.deviceY(point, frame: frame, side: side)
            gesture.y = y
            post(47128, "emoji/start?s=\(side == .left ? "l" : "r")&y=\(y)")
        }
        gestures[event.buttonNumber] = gesture
    }

    func drag(_ event: NSEvent) {
        guard let gesture = gestures[event.buttonNumber] else { return }
        let point = panel.point(event)
        switch gesture.frame.element {
        case let .tile(_, grid):
            guard gesture.button == 0, !gesture.swipeSent,
                  let dir = Geometry.swipe(dx: point.x - gesture.point.x, dy: point.y - gesture.point.y, ms: (event.timestamp - gesture.time) * 1000) else { return }
            gesture.swipeSent = true
            gesture.pending = true
            pending.append(gesture)
            Daemon.post("http://127.0.0.1:47113/tile/swipe?id=\(grid.id)&dir=\(dir)") { [weak self] status in
                guard let self, !gesture.cancelled else { return }
                gesture.pending = false
                self.pending.removeAll { $0 === gesture }
                if status != 404 || gesture.released { self.release(button: gesture.button) }
            }
        case let .strip(side):
            let y = panel.deviceY(point, frame: gesture.frame, side: side)
            guard let previous = gesture.y, previous != y else { return }
            gesture.y = y
            if side == .left { post(47113, "volume/drag?dy=\(previous - y)") }
            else { post(47117, "ai/slide?y=\(y)") }
        }
    }

    func up(_ event: NSEvent) {
        guard let gesture = gestures.removeValue(forKey: event.buttonNumber) else { return }
        gesture.released = true
        if case let .strip(side) = gesture.frame.element {
            let y = panel.deviceY(panel.point(event), frame: gesture.frame, side: side)
            post(47128, "emoji/end?s=\(side == .left ? "l" : "r")&y=\(y)")
        } else if !gesture.pending { release(button: event.buttonNumber) }
        panel.relayout(hover: panel.window.frame.contains(NSEvent.mouseLocation))
    }

    private func release(button: Int) {
        if let id = held.up(button: button) { connection.press(id, down: false) }
    }

    func releaseAll() {
        for gesture in pending { gesture.cancelled = true }
        pending.removeAll()
        for gesture in gestures.values {
            gesture.cancelled = true
            if case let .strip(side) = gesture.frame.element {
                post(47128, "emoji/end?s=\(side == .left ? "l" : "r")&y=\(gesture.y ?? 0)")
            }
        }
        gestures.removeAll()
        for id in held.releaseAll() { connection.press(id, down: false) }
        panel.relayout()
    }

    func wheel(_ event: NSEvent) {
        guard let frame = panel.hit(panel.point(event)) else { return }
        if wheelTarget != frame.element { wheelSteps = WheelSteps(); wheelTarget = frame.element }
        let steps = wheelSteps.consume(delta: event.scrollingDeltaY, precise: event.hasPreciseScrollingDeltas, inverted: event.isDirectionInvertedFromDevice)
        guard steps != 0 else { return }
        switch frame.element {
        case let .tile(n, _):
            guard let knob = Layout.knob(forTile: n) else { return }
            for _ in 0..<abs(steps) { connection.rotate(knob, right: steps > 0) }
        case .strip(.left):
            for _ in 0..<abs(steps) { post(47113, "volume/drag?dy=\(steps > 0 ? 5 : -5)") }
        case .strip(.right): break
        }
    }

    private func post(_ port: Int, _ path: String) { Daemon.post("http://127.0.0.1:\(port)/\(path)") { _ in } }
}
