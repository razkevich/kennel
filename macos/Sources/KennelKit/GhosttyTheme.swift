import AppKit

/// Terminal look borrowed from the user's Ghostty config, so tabs feel like Ghostty.
public struct GhosttyTheme: Equatable {
    public var fontFamily: String?
    public var fontSize: CGFloat = 13
    public var background: String = "#1e1e2e"
    public var foreground: String = "#cdd6f4"
    public var cursor: String?
    public var selection: String?
    public var palette: [Int: String] = [:]

    public init() {}

    /// Parses Ghostty's `key = value` config. Theme files use the same syntax;
    /// a `theme` line is resolved with `themeLoader` and applied before later keys.
    public static func parse(_ text: String, themeLoader: (String) -> String? = { _ in nil }) -> GhosttyTheme {
        var t = GhosttyTheme()
        t.apply(text, themeLoader: themeLoader)
        return t
    }

    mutating func apply(_ text: String, themeLoader: (String) -> String?) {
        for raw in text.split(whereSeparator: \.isNewline) {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("#") { continue }
            guard let eq = line.firstIndex(of: "=") else { continue }
            let key = line[..<eq].trimmingCharacters(in: .whitespaces)
            var value = line[line.index(after: eq)...].trimmingCharacters(in: .whitespaces)
            if value.hasPrefix("\""), value.hasSuffix("\""), value.count >= 2 { value = String(value.dropFirst().dropLast()) }
            switch key {
            case "font-family": if fontFamily == nil, !value.isEmpty { fontFamily = value }
            case "font-size": if let v = Double(value) { fontSize = CGFloat(v) }
            case "background": background = Self.hex(value)
            case "foreground": foreground = Self.hex(value)
            case "cursor-color": cursor = Self.hex(value)
            case "selection-background": selection = Self.hex(value)
            case "palette":
                let parts = value.split(separator: "=", maxSplits: 1).map { $0.trimmingCharacters(in: .whitespaces) }
                if parts.count == 2, let n = Int(parts[0]) { palette[n] = Self.hex(parts[1]) }
            case "theme":
                if let text = themeLoader(Self.darkVariant(value)) { apply(text, themeLoader: { _ in nil }) }
            default: continue
            }
        }
    }

    /// `theme = light:A,dark:B` picks the dark one: kennel is a dark app.
    static func darkVariant(_ v: String) -> String {
        for part in v.split(separator: ",") {
            let p = part.trimmingCharacters(in: .whitespaces)
            if p.hasPrefix("dark:") { return String(p.dropFirst(5)) }
        }
        if let first = v.split(separator: ",").first {
            let p = first.trimmingCharacters(in: .whitespaces)
            return p.hasPrefix("light:") ? String(p.dropFirst(6)) : p
        }
        return v
    }

    static func hex(_ v: String) -> String { v.hasPrefix("#") ? v : "#" + v }

    /// Loads the user's Ghostty config and its theme from the installed Ghostty.app.
    public static func load() -> GhosttyTheme {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let candidates = [
            home + "/.config/ghostty/config",
            home + "/Library/Application Support/com.mitchellh.ghostty/config",
        ]
        let themeDirs = [
            home + "/.config/ghostty/themes",
            "/Applications/Ghostty.app/Contents/Resources/ghostty/themes",
        ]
        let loader: (String) -> String? = { name in
            for dir in themeDirs {
                if let s = try? String(contentsOfFile: dir + "/" + name, encoding: .utf8) { return s }
            }
            return nil
        }
        var t = GhosttyTheme()
        for path in candidates {
            if let s = try? String(contentsOfFile: path, encoding: .utf8) { t.apply(s, themeLoader: loader) }
        }
        return t
    }

    public var font: NSFont {
        if let name = fontFamily, let f = NSFont(name: name, size: fontSize) { return f }
        return NSFont.monospacedSystemFont(ofSize: fontSize, weight: .regular)
    }

    public static func color(_ hex: String) -> NSColor {
        var s = hex.trimmingCharacters(in: .whitespaces)
        if s.hasPrefix("#") { s.removeFirst() }
        guard s.count == 6, let v = UInt32(s, radix: 16) else { return .textColor }
        return NSColor(srgbRed: CGFloat((v >> 16) & 0xff) / 255, green: CGFloat((v >> 8) & 0xff) / 255, blue: CGFloat(v & 0xff) / 255, alpha: 1)
    }
}
