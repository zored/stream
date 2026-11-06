import Foundation

func log(_ message: String) {
    let line = "\(ISO8601DateFormatter().string(from: Date())) \(message)\n"
    FileHandle.standardError.write(Data(line.utf8))
}

func fail(_ message: String) -> Never {
    log(message)
    exit(1)
}
