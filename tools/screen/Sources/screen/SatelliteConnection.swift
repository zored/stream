import Foundation
import CoreGraphics
import Network
import ScreenCore

final class SatelliteConnection {
    var onFrame: (GridID, Data, CGImage) -> Void = { _, _, _ in }
    private var connection: NWConnection?
    private var buffer = Data()
    private var heartbeat: DispatchSourceTimer?

    func connect() {
        guard connection == nil else { return }
        buffer.removeAll()
        let socket = NWConnection(host: "127.0.0.1", port: 16622, using: .tcp)
        connection = socket
        socket.stateUpdateHandler = { [weak self, weak socket] state in
            guard let self, let socket, self.connection === socket else { return }
            switch state {
            case .ready:
                let timer = DispatchSource.makeTimerSource(queue: .main)
                timer.schedule(deadline: .now() + 2, repeating: 2)
                timer.setEventHandler { [weak self] in self?.send("PING screen\n") }
                self.heartbeat = timer
                timer.resume()
                self.receive(socket)
            case .failed(let error), .waiting(let error): fail("Satellite: \(error)")
            case .cancelled: fail("Satellite cancelled unexpectedly")
            default: break
            }
        }
        socket.start(queue: .main)
    }

    private func receive(_ socket: NWConnection) {
        socket.receive(minimumIncompleteLength: 1, maximumLength: 1_048_576) { [weak self, weak socket] data, _, complete, error in
            guard let self, let socket, self.connection === socket else { return }
            if let error { fail("Satellite receive: \(error)") }
            if let data { self.buffer.append(data) }
            while let newline = self.buffer.firstIndex(of: 10) {
                let bytes = self.buffer[..<newline]
                guard let line = String(data: bytes, encoding: .utf8) else { fail("Satellite invalid UTF-8") }
                self.buffer.removeSubrange(...newline)
                self.handle(line.trimmingCharacters(in: .newlines))
            }
            guard self.buffer.count < 2_097_152 else { fail("Satellite line exceeds 2 MiB") }
            if complete { fail("Satellite closed") }
            self.receive(socket)
        }
    }

    private func handle(_ line: String) {
        switch Satellite.parse(line) {
        case .begin: send(Satellite.addDevice())
        case .ping(let payload): send(Satellite.pong(payload))
        case .error(let message): fail("Satellite ERROR: \(message)")
        case .keyState(let id, let data):
            let width: Int
            let height: Int
            switch Satellite.preset(id) {
            case "tile": width = Satellite.tileBitmap; height = width
            case "strip": width = Satellite.stripBitmap.w; height = Satellite.stripBitmap.h
            default: fail("Unexpected bitmap control \(id)")
            }
            guard data.count == width * height * 3 else { fail("Bad bitmap \(id): \(data.count), expected \(width * height * 3)") }
            guard let image = Self.image(data, width: width, height: height) else { fail("Cannot create bitmap \(id)") }
            onFrame(id, data, image)
        default: break
        }
    }

    static func image(_ data: Data, width: Int, height: Int) -> CGImage? {
        guard data.count == width * height * 3, let provider = CGDataProvider(data: data as CFData) else { return nil }
        return CGImage(width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 24,
                       bytesPerRow: width * 3, space: CGColorSpaceCreateDeviceRGB(),
                       bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.none.rawValue), provider: provider,
                       decode: nil, shouldInterpolate: true, intent: .defaultIntent)
    }

    private func send(_ line: String) {
        guard let socket = connection else { return }
        socket.send(content: Data(line.utf8), completion: .contentProcessed { [weak self, weak socket] error in
            guard let self, let socket, self.connection === socket else { return }
            if let error { fail("Satellite send: \(error)") }
        })
    }

    func press(_ id: GridID, down: Bool) { send(Satellite.press(id, down: down)) }
    func rotate(_ id: GridID, right: Bool) { send(Satellite.rotate(id, right: right)) }

    func disconnect(done: @escaping () -> Void = {}) {
        guard let socket = connection else { DispatchQueue.main.async(execute: done); return }
        connection = nil
        heartbeat?.cancel()
        heartbeat = nil
        socket.send(content: Data(Satellite.removeDevice.utf8), completion: .contentProcessed { error in
            if let error { log("Satellite remove: \(error)") }
            socket.cancel()
            done()
        })
    }
}
