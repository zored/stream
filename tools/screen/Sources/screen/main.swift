import AppKit
import ScreenCore

final class ScreenApp: NSObject, NSApplicationDelegate {
    private let connection = SatelliteConnection()
    private var panel: Panel!
    private var device: Device?
    private var userHidden = false
    private var plugged = false
    private var asleep = false
    private var monitor: Any?
    private var reveal: DispatchWorkItem?
    private var terminating = false
    private var cleanupComplete = false
    private var signals: [DispatchSourceSignal] = []

    func applicationDidFinishLaunching(_ notification: Notification) {
        panel = Panel(connection: connection)
        connection.onFrame = { [weak self] id, data, image in
            self?.panel.frame(id: id, data: data, image: image)
            if CommandLine.arguments.contains("--probe") { log("KEY-STATE \(id.id)") }
        }
        panel.onStripDoubleClick = { [weak self] in self?.hideByUser() }
        for number in [SIGINT, SIGTERM] {
            signal(number, SIG_IGN)
            let source = DispatchSource.makeSignalSource(signal: number, queue: .main)
            source.setEventHandler { NSApp.terminate(nil) }
            source.resume()
            signals.append(source)
        }
        if CommandLine.arguments.contains("--probe") {
            connection.connect()
            DispatchQueue.main.asyncAfter(deadline: .now() + 5) { NSApp.terminate(nil) }
        } else {
            let workspace = NSWorkspace.shared.notificationCenter
            workspace.addObserver(forName: NSWorkspace.willSleepNotification, object: nil, queue: .main) { [weak self] _ in
                guard let self else { return }
                log("sleep")
                self.asleep = true
                self.panel.releaseAll()
                self.connection.disconnect()
            }
            workspace.addObserver(forName: NSWorkspace.didWakeNotification, object: nil, queue: .main) { [weak self] _ in
                guard let self else { return }
                log("wake")
                self.asleep = false
                if !self.plugged { self.connection.connect() }
            }
            device = Device { [weak self] in self?.deviceChanged($0) }
        }
    }

    private func deviceChanged(_ isPlugged: Bool) {
        log("device plugged=\(isPlugged)")
        plugged = isPlugged
        userHidden = false
        stopReveal()
        if ScreenVisibility.shouldShow(devicePlugged: isPlugged, userHidden: userHidden) {
            if !asleep { connection.connect() }
            panel.show()
        } else {
            panel.releaseAll()
            panel.hide()
            connection.disconnect()
        }
    }

    private func hideByUser() {
        panel.releaseAll()
        panel.hide()
        userHidden = true
        stopReveal()
        monitor = NSEvent.addGlobalMonitorForEvents(matching: .mouseMoved) { [weak self] _ in self?.cursorMoved() }
        guard monitor != nil else { fail("cannot install global mouse monitor") }
    }

    private func cursorMoved() {
        guard userHidden, let screen = NSScreen.screens.first else { return }
        let point = NSEvent.mouseLocation
        let inside = point.x >= screen.frame.minX && point.x < screen.frame.maxX && point.y <= screen.frame.maxY
        let distance = inside ? Double(screen.frame.maxY - point.y) : .infinity
        switch Reveal.step(distanceFromTop: distance, peeking: reveal != nil) {
        case .start:
            panel.peek()
            let work = DispatchWorkItem { [weak self] in
                guard let self else { return }
                self.userHidden = false
                self.stopReveal()
                self.panel.show()
            }
            reveal = work
            DispatchQueue.main.asyncAfter(deadline: .now() + Reveal.delay, execute: work)
        case .cancel:
            reveal?.cancel()
            reveal = nil
            panel.retract()
        case .none: break
        }
    }

    private func stopReveal() {
        reveal?.cancel()
        reveal = nil
        removeMonitor()
    }

    private func removeMonitor() {
        if let monitor { NSEvent.removeMonitor(monitor) }
        monitor = nil
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if cleanupComplete { return .terminateNow }
        guard !terminating else { return .terminateCancel }
        terminating = true
        stopReveal()
        panel?.releaseAll()
        connection.disconnect { [weak self] in
            self?.cleanupComplete = true
            sender.terminate(nil)
        }
        return .terminateCancel
    }
}

let application = NSApplication.shared
let delegate = ScreenApp()
application.setActivationPolicy(.accessory)
application.delegate = delegate
application.run()
