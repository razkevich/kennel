import Foundation

/// A launch request from the kennel page: run `command` in a terminal tab.
public struct Launch: Equatable {
    public var cwd: String
    public var title: String
    public var command: String
    public var session: String?
    public var focus: Bool

    public init(cwd: String, title: String, command: String, session: String? = nil, focus: Bool = false) {
        self.cwd = cwd
        self.title = title
        self.command = command
        self.session = session
        self.focus = focus
    }

    public init?(message: [String: Any]) {
        guard let cmd = message["command"] as? String, !cmd.isEmpty else { return nil }
        command = cmd
        cwd = message["cwd"] as? String ?? NSHomeDirectory()
        title = message["title"] as? String ?? "kennel"
        let s = message["session"] as? String ?? ""
        session = s.isEmpty ? nil : s
        focus = message["focus"] as? Bool ?? false
    }

    /// The shell arguments for running this launch. In test mode nothing real runs:
    /// the tab prints what it would have run and then echoes its input.
    public func shellArgs(fake: Bool) -> [String] {
        if fake {
            return ["-c", "printf '%s\\n' \(shellQuote("would run: " + command)); exec cat"]
        }
        return ["-l", "-c", command]
    }
}

public func shellQuote(_ s: String) -> String {
    "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
}

/// Pulls the page URL out of `kennel map` output.
public func serverURL(fromLine line: String) -> URL? {
    guard let r = line.range(of: "http://127.0.0.1:") else { return nil }
    let tail = line[r.lowerBound...].split(separator: " ").first.map(String.init) ?? ""
    return URL(string: tail)
}
