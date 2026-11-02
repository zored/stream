import AppKit

let bundleID = "ru.yandex.desktop.music"
let companion = "http://127.0.0.1:8000/api/custom-variable/"
let port: UInt16 = 47115
let pollEvery = 0.25
let resendEvery = 10.0
let rescanEvery = 2.0
let pressGap = 1.5
let maxNodes = 20000

func log(_ message: String) {
    FileHandle.standardError.write("ymusic: \(message)\n".data(using: .utf8)!)
}

func fail(_ message: String) -> Never {
    log(message)
    exit(1)
}

func attr(_ element: AXUIElement, _ name: String) -> AnyObject? {
    var value: AnyObject?
    return AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success ? value : nil
}

func children(_ element: AXUIElement) -> [AXUIElement] {
    attr(element, kAXChildrenAttribute) as? [AXUIElement] ?? []
}

func hasClass(_ element: AXUIElement, _ prefix: String) -> Bool {
    (attr(element, "AXDOMClassList") as? [String] ?? []).contains { $0.hasPrefix(prefix) }
}

func find(_ root: AXUIElement, _ match: (AXUIElement) -> Bool) -> AXUIElement? {
    var queue = [root]
    var seen: Set<AXUIElement> = [root]
    var next = 0
    while next < queue.count, next < maxNodes {
        let element = queue[next]
        next += 1
        if match(element) { return element }
        for child in children(element) where seen.insert(child).inserted {
            queue.append(child)
        }
    }
    if next >= maxNodes { log("AX walk stopped at \(maxNodes) nodes") }
    return nil
}

let session: URLSession = {
    let config = URLSessionConfiguration.ephemeral
    config.httpMaximumConnectionsPerHost = 1
    config.timeoutIntervalForRequest = 2
    return URLSession(configuration: config)
}()
var sent: [String: String] = [:]

func setVariable(_ name: String, _ value: String) {
    if sent[name] == value { return }
    sent[name] = value
    var request = URLRequest(url: URL(string: companion + name + "/value")!)
    request.httpMethod = "POST"
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    request.httpBody = try! JSONSerialization.data(withJSONObject: value, options: .fragmentsAllowed)
    session.dataTask(with: request) { _, response, error in
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if error == nil && status < 300 { return }
        log("set \(name) failed: \(error?.localizedDescription ?? "") (HTTP \(status))")
        DispatchQueue.main.async { sent[name] = nil }
    }.resume()
}

func nowPlayingApp() -> String? {
    var result: String?
    let done = DispatchSemaphore(value: 0)
    session.dataTask(with: URL(string: companion + "np_app/value")!) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200, let data {
            result = String(decoding: data, as: UTF8.self)
        }
        done.signal()
    }.resume()
    done.wait()
    return result
}

enum Like: String {
    case liked, disliked, none, off, unknown
}

final class Player {
    var pid: pid_t = 0
    var app: AXUIElement?
    var bar: AXUIElement?
    var lastScan = Date.distantPast
    var lastPress = Date.distantPast
    var trusted = true

    func connect() -> Bool {
        guard let running = NSRunningApplication.runningApplications(withBundleIdentifier: bundleID).first else {
            pid = 0
            app = nil
            bar = nil
            return false
        }
        if running.processIdentifier != pid {
            pid = running.processIdentifier
            let element = AXUIElementCreateApplication(pid)
            AXUIElementSetAttributeValue(element, "AXManualAccessibility" as CFString, kCFBooleanTrue)
            app = element
            bar = nil
            lastScan = .distantPast
            log("found \(bundleID) pid \(pid)")
        }
        return true
    }

    func playerBar() -> AXUIElement? {
        if let bar, attr(bar, kAXRoleAttribute) != nil { return bar }
        bar = nil
        guard let app, Date().timeIntervalSince(lastScan) >= rescanEvery else { return nil }
        lastScan = Date()
        bar = find(app) { hasClass($0, "PlayerBar_root") }
        if bar == nil { log("player bar not found") }
        return bar
    }

    func button(_ bar: AXUIElement, like: Bool) -> AXUIElement? {
        find(bar) {
            like ? hasClass($0, "PlayerBarDesktop_likeButton")
                : attr($0, kAXDescriptionAttribute) as? String == "Не нравится"
        }
    }

    func state() -> Like {
        guard AXIsProcessTrusted() else {
            if trusted {
                log("no Accessibility permission: System Settings → Privacy & Security → Accessibility → ymusic")
                AXIsProcessTrustedWithOptions([kAXTrustedCheckOptionPrompt.takeUnretainedValue(): true] as CFDictionary)
            }
            trusted = false
            return .unknown
        }
        trusted = true
        guard connect() else { return .off }
        guard let bar = playerBar() else { return .unknown }
        guard let like = button(bar, like: true), let liked = attr(like, kAXValueAttribute) as? Int else { return .unknown }
        if liked == 1 { return .liked }
        if let dislike = button(bar, like: false), attr(dislike, kAXValueAttribute) as? Int == 1 { return .disliked }
        return .none
    }

    func press(like: Bool) -> String? {
        guard nowPlayingApp() == bundleID else { return "Now Playing is not \(bundleID)" }
        guard Date().timeIntervalSince(lastPress) >= pressGap else { return "previous press is still in flight" }
        guard connect(), let bar = playerBar() else { return "no player bar" }
        guard let target = button(bar, like: like) else { return "no \(like ? "like" : "dislike") button in the player bar" }
        let result = AXUIElementPerformAction(target, kAXPressAction as CFString)
        guard result == .success else { return "AXPress failed: \(result.rawValue)" }
        lastPress = Date()
        return nil
    }
}

let player = Player()

func poll() {
    setVariable("ym_like", player.state().rawValue)
}

func respond(_ client: Int32, _ status: String) {
    let response = "HTTP/1.1 \(status)\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
    _ = response.withCString { write(client, $0, strlen($0)) }
    close(client)
}

func handle(_ client: Int32) {
    var buffer = [UInt8](repeating: 0, count: 1024)
    let n = read(client, &buffer, buffer.count)
    guard n > 0 else { close(client); return }
    let parts = String(decoding: buffer[..<n], as: UTF8.self).split(separator: " ")
    let path = parts.count > 1 ? String(parts[1]) : ""
    guard parts.first == "POST", path == "/like" || path == "/dislike" else { return respond(client, "404 Not Found") }
    if let error = player.press(like: path == "/like") {
        log("\(path): \(error)")
        return respond(client, "409 Conflict")
    }
    log("\(path): pressed")
    poll()
    respond(client, "204 No Content")
}

func listenHTTP() {
    let server = socket(AF_INET, SOCK_STREAM, 0)
    var yes: Int32 = 1
    setsockopt(server, SOL_SOCKET, SO_REUSEADDR, &yes, socklen_t(MemoryLayout<Int32>.size))
    var address = sockaddr_in()
    address.sin_family = sa_family_t(AF_INET)
    address.sin_port = port.bigEndian
    address.sin_addr.s_addr = INADDR_LOOPBACK.bigEndian
    let bound = withUnsafePointer(to: &address) {
        $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(server, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) }
    }
    guard bound == 0 else { fail("can't bind 127.0.0.1:\(port): \(String(cString: strerror(errno)))") }
    guard listen(server, 16) == 0 else { fail("can't listen: \(String(cString: strerror(errno)))") }
    let source = DispatchSource.makeReadSource(fileDescriptor: server, queue: .main)
    source.setEventHandler {
        let client = accept(server, nil, nil)
        if client >= 0 { handle(client) }
    }
    source.resume()
    sources.append(source)
}

var sources: [DispatchSourceProtocol] = []

func every(_ seconds: Double, _ block: @escaping () -> Void) {
    let timer = DispatchSource.makeTimerSource(queue: .main)
    timer.schedule(deadline: .now(), repeating: seconds, leeway: .milliseconds(Int(seconds * 100)))
    timer.setEventHandler(handler: block)
    timer.resume()
    sources.append(timer)
}

listenHTTP()
every(pollEvery) { poll() }
every(resendEvery) { sent.removeAll() }
log("listening on 127.0.0.1:\(port)")
RunLoop.main.run()
