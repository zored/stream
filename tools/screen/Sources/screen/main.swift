import AppKit
import ScreenCore

final class ScreenApp: NSObject, NSApplicationDelegate {
    private let connection = SatelliteConnection()
    private var panel: Panel!
    private var device: Device?
    private var userHidden = false
    private var monitor: Any?
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
            device = Device { [weak self] in self?.deviceChanged($0) }
        }
    }

    private func deviceChanged(_ isPlugged: Bool) {
        if ScreenVisibility.shouldShow(devicePlugged: isPlugged, userHidden: userHidden) {
            connection.connect()
            panel.show()
        }
    }

    private func hideByUser() {
        panel.releaseAll()
        panel.hide()
        userHidden = true
        removeMonitor()
        monitor = NSEvent.addGlobalMonitorForEvents(matching: .mouseMoved) { [weak self] _ in
            guard let self, self.userHidden,
                  let screen = NSScreen.screens.first else { return }
            let point = NSEvent.mouseLocation
            guard point.x >= screen.frame.minX, point.x < screen.frame.maxX,
                  point.y >= screen.frame.maxY - 2, point.y <= screen.frame.maxY else { return }
            self.userHidden = false
            self.removeMonitor()
            self.panel.show()
        }
        guard monitor != nil else { fail("cannot install global mouse monitor") }
    }

    private func removeMonitor() {
        if let monitor { NSEvent.removeMonitor(monitor) }
        monitor = nil
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if cleanupComplete { return .terminateNow }
        guard !terminating else { return .terminateCancel }
        terminating = true
        removeMonitor()
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
