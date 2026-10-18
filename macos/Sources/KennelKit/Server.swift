import Foundation

/// Runs the bundled `kennel map` in "app" mode and reports the page URL.
public final class KennelServer {
    public private(set) var url: URL?
    private var process: Process?

    public init() {}

    /// Where the kennel binary lives: KENNEL_BIN, the app bundle, or the PATH.
    public static func binaryPath() -> String? {
        let env = ProcessInfo.processInfo.environment
        if let p = env["KENNEL_BIN"], FileManager.default.isExecutableFile(atPath: p) { return p }
        if let p = Bundle.main.path(forResource: "kennel", ofType: nil), FileManager.default.isExecutableFile(atPath: p) { return p }
        let home = NSHomeDirectory()
        for dir in (env["PATH"] ?? "").split(separator: ":").map(String.init) + [home + "/go/bin", "/opt/homebrew/bin", "/usr/local/bin"] {
            let p = dir + "/kennel"
            if FileManager.default.isExecutableFile(atPath: p) { return p }
        }
        return nil
    }

    public func start(demo: Bool, completion: @escaping (Result<URL, Error>) -> Void) {
        guard let bin = Self.binaryPath() else {
            completion(.failure(NSError(domain: "kennel", code: 1, userInfo: [NSLocalizedDescriptionKey: "kennel binary not found (set KENNEL_BIN or install kennel)"])))
            return
        }
        let p = Process()
        p.executableURL = URL(fileURLWithPath: bin)
        var args = ["map", "-no-open", "-port", "0", "-token", UUID().uuidString.lowercased()]
        if demo { args.insert("-demo", at: 0) }
        p.arguments = args
        var env = ProcessInfo.processInfo.environment
        env["KENNEL_OPENER"] = "app"
        // GUI apps get a minimal PATH; kennel shells out to claude, herdr and git.
        env["PATH"] = [env["PATH"] ?? "", NSHomeDirectory() + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin"].joined(separator: ":")
        p.environment = env
        let out = Pipe()
        p.standardOutput = out
        p.standardError = FileHandle.standardError
        var buffer = ""
        var done = false
        out.fileHandleForReading.readabilityHandler = { [weak self] h in
            let data = h.availableData
            guard !data.isEmpty, let s = String(data: data, encoding: .utf8) else { return }
            buffer += s
            for line in buffer.split(separator: "\n") {
                if !done, let u = serverURL(fromLine: String(line)) {
                    done = true
                    self?.url = u
                    DispatchQueue.main.async { completion(.success(u)) }
                }
            }
        }
        p.terminationHandler = { proc in
            if !done {
                done = true
                DispatchQueue.main.async {
                    completion(.failure(NSError(domain: "kennel", code: Int(proc.terminationStatus), userInfo: [NSLocalizedDescriptionKey: "kennel map exited (\(proc.terminationStatus))"])))
                }
            }
        }
        do {
            try p.run()
            process = p
        } catch {
            completion(.failure(error))
        }
    }

    public func stop() {
        process?.terminate()
        process = nil
    }
}
