import Foundation

let volumeStep = 6

let scripts: [String: String] = [
    "volup": "set volume output volume ((output volume of (get volume settings)) + \(volumeStep))",
    "voldown": "set volume output volume ((output volume of (get volume settings)) - \(volumeStep))",
    "mute": "set volume output muted (not (output muted of (get volume settings)))",
]

let mediaCommands: [String: Int32] = ["play": 2, "next": 4, "prev": 5]

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

func runScript(_ source: String) {
    var error: NSDictionary?
    NSAppleScript(source: source)!.executeAndReturnError(&error)
    if let error { fail("mediakey: AppleScript failed: \(error)") }
}

func sendMediaCommand(_ command: Int32) {
    typealias SendCommand = @convention(c) (Int32, CFDictionary?) -> Bool
    guard let handle = dlopen("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote", RTLD_NOW),
          let symbol = dlsym(handle, "MRMediaRemoteSendCommand") else {
        fail("mediakey: MediaRemote unavailable")
    }
    guard unsafeBitCast(symbol, to: SendCommand.self)(command, nil) else {
        fail("mediakey: MediaRemote rejected command \(command)")
    }
}

let name = CommandLine.arguments.count == 2 ? CommandLine.arguments[1] : ""
if let script = scripts[name] {
    runScript(script)
} else if let command = mediaCommands[name] {
    sendMediaCommand(command)
} else {
    fail("usage: mediakey \((Array(scripts.keys) + Array(mediaCommands.keys)).sorted().joined(separator: "|"))")
}
