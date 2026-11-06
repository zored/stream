// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "screen",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "ScreenCore"),
        .executableTarget(name: "screen", dependencies: ["ScreenCore"], swiftSettings: [.swiftLanguageMode(.v5)]),
        .executableTarget(name: "screen-checks", dependencies: ["ScreenCore"]),
    ]
)
