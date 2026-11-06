import AppKit
import ScreenCore

final class OverlayWindow: NSPanel {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class Panel {
    let window = OverlayWindow(contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
    private let view = PanelView()
    private var layers: [Element: CALayer] = [:]
    private var cells: [GridID: Data] = [:]
    private var observer: NSObjectProtocol?
    var onStripDoubleClick: () -> Void = {}
    private(set) var frames = RowLayout.frames(cursorX: nil)
    private(set) var input: Input!
    var isHidden: Bool { !window.isVisible }

    init(connection: SatelliteConnection) {
        window.level = .statusBar
        window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        window.backgroundColor = .clear
        window.isOpaque = false
        window.hasShadow = false
        window.hidesOnDeactivate = false
        window.acceptsMouseMovedEvents = true
        window.contentView = view
        view.owner = self
        view.wantsLayer = true
        for element in Layout.elements {
            let layer = CALayer()
            layer.backgroundColor = NSColor.black.cgColor
            layer.borderColor = NSColor.white.withAlphaComponent(0.14).cgColor
            layer.borderWidth = 0.5
            layer.masksToBounds = true
            layer.contentsGravity = .resize
            view.layer!.addSublayer(layer)
            layers[element] = layer
        }
        input = Input(panel: self, connection: connection)
        observer = NotificationCenter.default.addObserver(forName: NSApplication.didChangeScreenParametersNotification, object: nil, queue: .main) { [weak self] _ in self?.releaseAll(); self?.relayout() }
        relayout()
    }

    deinit { if let observer { NotificationCenter.default.removeObserver(observer) } }
    func show() { relayout(); window.orderFrontRegardless() }
    func hide() { window.orderOut(nil) }
    func releaseAll() { input.releaseAll() }

    func relayout(hover: Bool = false) {
        guard !input.isHeld, let screen = NSScreen.screens.first else { return }
        let cursor = hover ? Double(NSEvent.mouseLocation.x - screen.frame.midX) : nil
        frames = RowLayout.frames(cursorX: cursor)
        let width = RowLayout.width(frames)
        let height = RowLayout.height(frames) + 4
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        window.setFrame(NSRect(x: screen.frame.midX - width / 2, y: screen.frame.maxY - height, width: width, height: height), display: true)
        for frame in frames {
            let layer = layers[frame.element]!
            layer.frame = CGRect(x: frame.x + width / 2, y: 4, width: frame.width, height: frame.height)
            let radius: Double = if case .strip = frame.element { 8 } else { 9 }
            layer.cornerRadius = radius * frame.scale
        }
        CATransaction.commit()
    }

    func hit(_ point: NSPoint) -> ElementFrame? {
        let x = point.x - RowLayout.width(frames) / 2
        return frames.first { x >= $0.x && x < $0.x + $0.width && point.y >= 4 && point.y < 4 + $0.height }
    }

    func point(_ event: NSEvent) -> NSPoint { view.convert(event.locationInWindow, from: nil) }
    func deviceY(_ point: NSPoint, frame: ElementFrame, side: Side) -> Int {
        Geometry.deviceY(side: side, x: point.x - RowLayout.width(frames) / 2 - frame.x, width: frame.width)
    }

    func frame(id: GridID, data: Data, image: CGImage) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        if let side = Layout.strip(of: id) {
            cells[id] = data
            let bitmap = Satellite.stripBitmap
            let composed = Strip.compose(Layout.stripCells(side).map { cells[$0] }, cellW: bitmap.w, cellH: bitmap.h, side: side)
            let provider = CGDataProvider(data: composed as CFData)!
            layers[.strip(side)]!.contents = CGImage(width: bitmap.h * 3, height: bitmap.w, bitsPerComponent: 8, bitsPerPixel: 24, bytesPerRow: bitmap.h * 9, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGBitmapInfo(rawValue: 0), provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent)!
        } else if let n = Layout.tiles.firstIndex(of: id) {
            layers[.tile(n: n + 1, grid: id)]!.contents = image
        }
        CATransaction.commit()
    }
}

private final class PanelView: NSView {
    weak var owner: Panel?
    override var isFlipped: Bool { true }
    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        trackingAreas.forEach(removeTrackingArea)
        addTrackingArea(NSTrackingArea(rect: .zero, options: [.mouseMoved, .mouseEnteredAndExited, .activeAlways, .inVisibleRect], owner: self))
    }
    override func mouseMoved(with event: NSEvent) { owner?.relayout(hover: true) }
    override func mouseEntered(with event: NSEvent) { owner?.relayout(hover: true) }
    override func mouseExited(with event: NSEvent) { owner?.relayout() }
    override func mouseDown(with event: NSEvent) { owner?.input.down(event) }
    override func rightMouseDown(with event: NSEvent) { owner?.input.down(event) }
    override func otherMouseDown(with event: NSEvent) { owner?.input.down(event) }
    override func mouseDragged(with event: NSEvent) { owner?.input.drag(event) }
    override func rightMouseDragged(with event: NSEvent) { owner?.input.drag(event) }
    override func otherMouseDragged(with event: NSEvent) { owner?.input.drag(event) }
    override func mouseUp(with event: NSEvent) { owner?.input.up(event) }
    override func rightMouseUp(with event: NSEvent) { owner?.input.up(event) }
    override func otherMouseUp(with event: NSEvent) { owner?.input.up(event) }
    override func scrollWheel(with event: NSEvent) { owner?.input.wheel(event) }
}
