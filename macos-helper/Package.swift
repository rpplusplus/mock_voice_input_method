// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "VoicePasteHelper",
    platforms: [
        .macOS(.v13)
    ],
    targets: [
        .executableTarget(
            name: "VoicePasteHelper"
        )
    ]
)
