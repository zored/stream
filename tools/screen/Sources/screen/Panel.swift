import AppKit
import QuartzCore
import ScreenCore

final class OverlayWindow: NSPanel {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class Panel {
    let window = OverlayWindow(contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
    private let view = PanelView()
    private var tiles: [Element: TileView] = [:]
    private var cells: [GridID: Data] = [:]
    private var observers: [(NotificationCenter, NSObjectProtocol)] = []
    private var mouseMonitor: Any?
    private final class FrameTarget: NSObject {
        weak var owner: Panel?
        @objc func tick(_ link: CADisplayLink) { owner?.advance(at: link.targetTimestamp) }
    }
    private let frameTarget = FrameTarget()
    private var displayLink: CADisplayLink?
    private var animationTime = CACurrentMediaTime()
    private var layoutMotion: (tracker: HoverMotion, to: [ElementFrame], time: TimeInterval)?
    private var visibilityMotion: (from: Double, to: Double, start: TimeInterval)?
    private var slide = 1.0
    private var wantsVisible = false
    var onStripDoubleClick: () -> Void = {}
    private(set) var frames = RowLayout.frames(cursorX: nil)
    private(set) var input: Input!
    var isHidden: Bool { !wantsVisible }
    var isAnimating: Bool { displayLink != nil }

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
        let content = PassThroughView()
        let container: NSView
        if #available(macOS 26.0, *) {
            let glass = NSGlassEffectContainerView()
            glass.spacing = 0
            glass.contentView = content
            container = glass
        } else { container = content }
        container.frame = view.bounds
        container.autoresizingMask = [.width, .height]
        view.addSubview(container)
        for element in Layout.elements {
            let tile = TileView()
            content.addSubview(tile)
            tiles[element] = tile
        }
        frameTarget.owner = self
        input = Input(panel: self, connection: connection)
        observe(.default, NSApplication.didChangeScreenParametersNotification) { panel in
            panel.releaseAll()
            panel.layoutMotion = nil
            panel.frames = RowLayout.frames(cursorX: nil)
            panel.applyLayout()
            panel.displayLink?.invalidate()
            panel.displayLink = nil
            if panel.visibilityMotion != nil { panel.startAnimation() }
        }
        observe(.default, NSApplication.didResignActiveNotification) { $0.focusLost() }
        observe(.default, NSApplication.didHideNotification) { $0.focusLost() }
        observe(NSWorkspace.shared.notificationCenter, NSWorkspace.didActivateApplicationNotification) { $0.focusLost() }
        observe(NSWorkspace.shared.notificationCenter, NSWorkspace.sessionDidResignActiveNotification) { $0.focusLost() }
        applyLayout()
    }

    private func observe(_ center: NotificationCenter, _ name: Notification.Name, action: @escaping (Panel) -> Void) {
        let token = center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
            if let self { action(self) }
        }
        observers.append((center, token))
    }

    deinit {
        displayLink?.invalidate()
        if let mouseMonitor { NSEvent.removeMonitor(mouseMonitor) }
        for (center, token) in observers { center.removeObserver(token) }
    }

    func show() {
        guard !wantsVisible else { return }
        advance()
        wantsVisible = true
        window.ignoresMouseEvents = false
        window.orderFrontRegardless()
        visibilityMotion = (slide, 0, CACurrentMediaTime())
        startAnimation()
        mouseMonitor = NSEvent.addGlobalMonitorForEvents(matching: .mouseMoved) { [weak self] _ in
            self?.relayout(hover: true)
        }
        guard mouseMonitor != nil else { fail("cannot install panel mouse monitor") }
    }

    func hide() {
        guard wantsVisible else { return }
        advance()
        wantsVisible = false
        window.ignoresMouseEvents = true
        if let mouseMonitor { NSEvent.removeMonitor(mouseMonitor) }
        mouseMonitor = nil
        setLayoutTarget(RowLayout.frames(cursorX: nil))
        visibilityMotion = (slide, 1, CACurrentMediaTime())
        startAnimation()
    }

    func releaseAll() { input.releaseAll() }

    private func focusLost() {
        releaseAll()
        relayout()
    }

    func freezeLayout() {
        layoutMotion = nil
        stopAnimationIfIdle()
    }

    func relayout(hover: Bool = false) {
        guard !input.isHeld, let screen = NSScreen.screens.first else { return }
        let location = NSEvent.mouseLocation
        let cursor = hover && wantsVisible && visibilityMotion == nil && window.frame.contains(location)
            ? Double(location.x - screen.frame.midX) : nil
        let target = RowLayout.frames(cursorX: cursor)
        if let motion = layoutMotion, motion.to == target { return }
        if layoutMotion == nil && target == frames { return }
        setLayoutTarget(target)
        startAnimation()
    }

    private func setLayoutTarget(_ target: [ElementFrame]) {
        let now = CACurrentMediaTime()
        var motion = layoutMotion ?? (HoverMotion(frames: frames), target, now)
        motion.tracker.retarget(target)
        motion.to = target
        layoutMotion = motion
    }

    private func startAnimation() {
        guard displayLink == nil else { return }
        guard let screen = NSScreen.screens.first else { fail("panel display is unavailable") }
        let link = screen.displayLink(target: frameTarget, selector: #selector(FrameTarget.tick(_:)))
        configure(link)
        displayLink = link
        link.add(to: .main, forMode: .common)
    }

    private func configure(_ link: CADisplayLink) {
        guard let screen = NSScreen.screens.first, screen.maximumFramesPerSecond > 0 else {
            fail("cannot determine panel display refresh rate")
        }
        let rate = Float(screen.maximumFramesPerSecond)
        link.preferredFrameRateRange = CAFrameRateRange(minimum: rate, maximum: rate, preferred: rate)
    }

    private func stopAnimationIfIdle() {
        if layoutMotion == nil && visibilityMotion == nil { displayLink?.invalidate(); displayLink = nil }
    }

    private func advance(at timestamp: TimeInterval? = nil) {
        let now = max(timestamp ?? CACurrentMediaTime(), animationTime)
        animationTime = now
        if var motion = layoutMotion {
            motion.tracker.advance(seconds: now - motion.time)
            frames = motion.tracker.frames
            motion.time = now
            layoutMotion = motion.tracker.isSettled ? nil : motion
        }
        if let motion = visibilityMotion {
            let progress = (now - motion.start) / Motion.visibilityDuration
            slide = Motion.value(from: motion.from, to: motion.to, progress: progress)
            if progress >= 1 {
                visibilityMotion = nil
                if !wantsVisible { window.orderOut(nil) }
            }
        }
        applyLayout()
        stopAnimationIfIdle()
    }

    private func applyLayout() {
        guard let screen = NSScreen.screens.first else { return }
        let width = RowLayout.width(frames)
        let height = RowLayout.height(frames) + 4
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        let bounds = NSRect(x: screen.frame.midX - width / 2, y: screen.frame.maxY - height, width: width, height: height)
        if window.frame != bounds { window.setFrame(bounds, display: false) }
        for frame in frames {
            let tile = tiles[frame.element]!
            tile.frame = CGRect(x: frame.x + width / 2, y: 4 - 84 * slide, width: frame.width, height: frame.height)
            tile.setRadius(4 * frame.scale)
        }
        CATransaction.commit()
    }

    func hit(_ point: NSPoint) -> ElementFrame? {
        guard wantsVisible, visibilityMotion == nil else { return nil }
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
            let strip = glassImage(composed, width: bitmap.h * 3, height: bitmap.w)
            tiles[.strip(side)]!.picture.layer!.contents = strip
        } else if let n = Layout.tiles.firstIndex(of: id) {
            tiles[.tile(n: n + 1, grid: id)]!.picture.layer!.contents = glassImage(data, width: image.width, height: image.height, preserveInset: id == GridID(1, 2) ? 0.06 : nil)
        }
        CATransaction.commit()
    }

    private func glassImage(_ rgb: Data, width: Int, height: Int, preserveInset: Double? = nil) -> CGImage {
        let rgba = GlassBitmap.rgba(rgb, width: width, height: height, preserveInset: preserveInset)
        guard let provider = CGDataProvider(data: rgba as CFData),
              let image = CGImage(width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32,
                                  bytesPerRow: width * 4, space: CGColorSpaceCreateDeviceRGB(),
                                  bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue).union(.byteOrder32Big),
                                  provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent) else {
            fail("cannot create transparent glass bitmap")
        }
        return image
    }
}

private class PassThroughView: NSView {
    override var isFlipped: Bool { true }
    override func hitTest(_ point: NSPoint) -> NSView? { nil }
}

private final class TileView: PassThroughView {
    let picture = PassThroughView()
    private let effect: NSView
    private let content = PassThroughView()

    override init(frame: NSRect) {
        if #available(macOS 26.0, *) {
            let glass = NSGlassEffectView()
            glass.style = .clear
            glass.tintColor = nil
            glass.contentView = content
            effect = glass
        } else {
            let blur = NSVisualEffectView()
            blur.material = .hudWindow
            blur.blendingMode = .behindWindow
            blur.state = .active
            blur.addSubview(content)
            content.autoresizingMask = [.width, .height]
            effect = blur
        }
        super.init(frame: frame)
        wantsLayer = true
        layer!.masksToBounds = true
        layer!.cornerCurve = .continuous
        layer!.borderColor = NSColor.white.withAlphaComponent(0.14).cgColor
        layer!.borderWidth = 0.5
        content.addSubview(picture)
        picture.wantsLayer = true
        picture.layer!.opacity = 1
        picture.layer!.masksToBounds = true
        picture.layer!.cornerCurve = .continuous
        picture.layer!.contentsGravity = .resize
        effect.frame = bounds
        effect.autoresizingMask = [.width, .height]
        addSubview(effect)
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) is unavailable") }

    override func layout() {
        super.layout()
        picture.frame = content.bounds
    }

    func setRadius(_ radius: Double) {
        needsLayout = true
        layer!.cornerRadius = radius
        picture.layer!.cornerRadius = radius
        if #available(macOS 26.0, *), let glass = effect as? NSGlassEffectView { glass.cornerRadius = radius }
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
