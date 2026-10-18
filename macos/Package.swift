// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Kennel",
    platforms: [.macOS(.v13)],
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm", from: "1.20.0"),
    ],
    targets: [
        .target(name: "KennelKit", dependencies: ["SwiftTerm"]),
        .executableTarget(name: "Kennel", dependencies: ["KennelKit"]),
        .testTarget(name: "KennelKitTests", dependencies: ["KennelKit"]),
    ]
)
