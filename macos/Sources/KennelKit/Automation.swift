import AppKit
import Network
import WebKit

/// A tiny HTTP control port for end-to-end UI tests. It exists only when the app is
/// started with KENNEL_APP_AUTOMATION=<port>, and listens on loopback.
///
///   GET  /state                      tabs, selection, status item, page loaded
///   POST /key       {key, mods}      press a shortcut through the real menus
///   POST /keydown   {key, mods}      a real key press in the selected window (return, a, …)
///   POST /select    {index}          select a tab
///   POST /type      {text}           type into the selected terminal
///   GET  /text?index=N[&pane=K]      a terminal tab's text (its focused split, or split K in order)
///   POST /js        {code}           evaluate JavaScript in the kennel page
///   POST /click     {x, y}           click inside the kennel page (page coordinates)
///   POST /drag      {x, y, toX, toY} drag inside the kennel page with real mouse events
///   POST /snapshot  {index, path}    save a PNG of a tab
///   POST /dialogs   {answers}        queue answers for the page's confirm/prompt dialogs
@MainActor
public final class Automation {
    weak var controller: AppController?
    private var listener: NWListener?
    private var answers: [String?] = []

    init(controller: AppController, port: UInt16) {
        self.controller = controller
        let params = NWParameters.tcp
        params.requiredLocalEndpoint = NWEndpoint.hostPort(host: "127.0.0.1", port: NWEndpoint.Port(rawValue: port)!)
        listener = try? NWListener(using: params)
        listener?.newConnectionHandler = { [weak self] conn in MainActor.assumeIsolated { self?.serve(conn) } }
        listener?.start(queue: .main)
    }

    /// The next queued dialog answer: nil when the queue is empty, .some(nil) for "Cancel".
    func nextDialogAnswer() -> String?? {
        if answers.isEmpty { return nil }
        return .some(answers.removeFirst())
    }

    private func serve(_ conn: NWConnection) {
        conn.start(queue: .main)
        var data = Data()
        func readMore() {
            conn.receive(minimumIncompleteLength: 1, maximumLength: 1 << 20) { [weak self] chunk, _, done, _ in
                MainActor.assumeIsolated {
                if let chunk { data.append(chunk) }
                if let req = Request(data) {
                    self?.handle(req) { status, body in
                        let payload = (try? JSONSerialization.data(withJSONObject: body, options: [.fragmentsAllowed])) ?? Data("null".utf8)
                        var head = "HTTP/1.1 \(status) OK\r\nContent-Type: application/json\r\nContent-Length: \(payload.count)\r\nConnection: close\r\n\r\n"
                        if status != 200 { head = head.replacingOccurrences(of: " OK", with: " ERR") }
                        conn.send(content: Data(head.utf8) + payload, completion: .contentProcessed { _ in conn.cancel() })
                    }
                } else if !done {
                    readMore()
                } else {
                    conn.cancel()
                }
                }
            }
        }
        readMore()
    }

    struct Request {
        var method: String
        var path: String
        var query: [String: String]
        var json: [String: Any]

        init?(_ data: Data) {
            guard let headEnd = data.range(of: Data("\r\n\r\n".utf8)) else { return nil }
            let head = String(decoding: data[..<headEnd.lowerBound], as: UTF8.self)
            let lines = head.components(separatedBy: "\r\n")
            let parts = lines.first?.split(separator: " ") ?? []
            guard parts.count >= 2 else { return nil }
            var length = 0
            for l in lines.dropFirst() where l.lowercased().hasPrefix("content-length:") {
                length = Int(l.split(separator: ":")[1].trimmingCharacters(in: .whitespaces)) ?? 0
            }
            let body = data[headEnd.upperBound...]
            guard body.count >= length else { return nil }
            method = String(parts[0])
            let comps = URLComponents(string: String(parts[1]))
            path = comps?.path ?? "/"
            query = Dictionary((comps?.queryItems ?? []).map { ($0.name, $0.value ?? "") }, uniquingKeysWith: { a, _ in a })
            json = (try? JSONSerialization.jsonObject(with: Data(body.prefix(length)))) as? [String: Any] ?? [:]
        }
    }

    private func handle(_ r: Request, reply: @escaping (Int, Any) -> Void) {
        guard let c = controller else { return reply(500, ["error": "no controller"]) }
        switch (r.method, r.path) {
        case ("GET", "/state"):
            let windows = c.orderedWindows
            reply(200, [
                "tabs": windows.enumerated().map { i, w in
                    ["index": i, "title": w.title, "kind": w === c.kennelWindow ? "kennel" : "terminal",
                     "session": c.tab(for: w)?.session ?? "", "selected": w === c.selectedWindow, "window": w.windowNumber,
                     "panes": c.tab(for: w).map { t in t.orderedPanes.map { p in
                         ["id": p.id, "session": p.session ?? "", "focused": p.id == t.focusedID, "hidden": p.holder.isHidden,
                          "frame": [p.holder.frame.minX, p.holder.frame.minY, p.holder.frame.width, p.holder.frame.height],
                          "font": p.view.font.pointSize] as [String: Any] } } ?? [],
                     "zoomed": c.tab(for: w)?.zoomedID != nil,
                     "pageZoom": w === c.kennelWindow ? c.web.pageZoom : 0,
                     "bg": c.tab(for: w).map { t in t.view.layer?.backgroundColor.flatMap { NSColor(cgColor: $0)?.usingColorSpace(.sRGB).map { String(format: "#%02x%02x%02x", Int($0.redComponent*255), Int($0.greenComponent*255), Int($0.blueComponent*255)) } } ?? "none" } ?? ""]
                },
                "status": c.statusTitle,
                "kennelDark": c.kennelDark,
                "pageLoaded": c.pageLoaded,
                "url": c.server.url?.absoluteString ?? "",
            ])
        case ("POST", "/key"):
            let names = ["up": NSUpArrowFunctionKey, "down": NSDownArrowFunctionKey, "left": NSLeftArrowFunctionKey, "right": NSRightArrowFunctionKey]
            var key = r.json["key"] as? String ?? ""
            if let code = names[key] { key = String(Character(UnicodeScalar(code)!)) } else if key == "enter" { key = "\r" }
            var flags: NSEvent.ModifierFlags = []
            for m in r.json["mods"] as? [String] ?? ["cmd"] {
                switch m {
                case "cmd": flags.insert(.command)
                case "shift": flags.insert(.shift)
                case "alt": flags.insert(.option)
                case "ctrl": flags.insert(.control)
                default: break
                }
            }
            // Like a real keyboard, Shift changes the character ("D", "{").
            if flags.contains(.shift), let shifted = shiftedKey(key) { key = shifted }
            let ev = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: flags, timestamp: 0,
                                      windowNumber: c.selectedWindow?.windowNumber ?? 0, context: nil,
                                      characters: key, charactersIgnoringModifiers: key, isARepeat: false, keyCode: virtualKeyCode(key))!
            let handled = NSApp.mainMenu?.performKeyEquivalent(with: ev) ?? false
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { reply(200, ["handled": handled]) }
        case ("POST", "/keydown"):
            let key = r.json["key"] as? String ?? ""
            var flags: NSEvent.ModifierFlags = []
            for m in r.json["mods"] as? [String] ?? [] {
                switch m {
                case "shift": flags.insert(.shift)
                case "alt": flags.insert(.option)
                case "ctrl": flags.insert(.control)
                case "cmd": flags.insert(.command)
                default: break
                }
            }
            let chars = key == "return" ? "\r" : key
            guard let w = c.selectedWindow, let ev = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: flags, timestamp: ProcessInfo.processInfo.systemUptime,
                                                                     windowNumber: w.windowNumber, context: nil, characters: chars, charactersIgnoringModifiers: chars,
                                                                     isARepeat: false, keyCode: virtualKeyCode(chars)) else { return reply(400, ["error": "no window"]) }
            NSApp.sendEvent(ev)
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { reply(200, ["ok": true]) }
        case ("POST", "/select"):
            let i = r.json["index"] as? Int ?? 0
            let ws = c.orderedWindows
            guard ws.indices.contains(i) else { return reply(400, ["error": "no tab \(i)"]) }
            c.select(ws[i])
            reply(200, ["ok": true])
        case ("POST", "/type"):
            guard let t = c.tab(for: c.selectedWindow) else { return reply(400, ["error": "selected tab isn't a terminal"]) }
            let text = (r.json["text"] as? String ?? "").replacingOccurrences(of: "\n", with: "\r")
            t.view.send(txt: text)
            reply(200, ["ok": true])
        case ("GET", "/text"):
            let i = Int(r.query["index"] ?? "") ?? -1
            let ws = c.orderedWindows
            guard ws.indices.contains(i), let t = c.tab(for: ws[i]) else { return reply(400, ["error": "no terminal at \(i)"]) }
            let panes = t.orderedPanes
            let k = Int(r.query["pane"] ?? "")
            guard let pane = k.map({ panes.indices.contains($0) ? panes[$0] : nil }) ?? t.focused else { return reply(400, ["error": "no pane"]) }
            reply(200, ["text": pane.text()])
        case ("POST", "/js"):
            let code = r.json["code"] as? String ?? "null"
            c.web.evaluateJavaScript("JSON.stringify((function(){ return \(code) })())") { value, err in
                if let err { return reply(400, ["error": err.localizedDescription]) }
                let s = value as? String ?? "null"
                let parsed = (try? JSONSerialization.jsonObject(with: Data(s.utf8), options: [.fragmentsAllowed])) ?? NSNull()
                reply(200, ["value": parsed])
            }
        case ("POST", "/jsasync"):
            let code = r.json["code"] as? String ?? "null"
            c.web.callAsyncJavaScript("return JSON.stringify(await (async () => { \(code) })())", arguments: [:], in: nil, in: .page) { result in
                switch result {
                case .success(let v):
                    let s = v as? String ?? "null"
                    reply(200, ["value": (try? JSONSerialization.jsonObject(with: Data(s.utf8), options: [.fragmentsAllowed])) ?? NSNull()])
                case .failure(let e):
                    reply(400, ["error": e.localizedDescription])
                }
            }
        case ("POST", "/click"):
            let x = r.json["x"] as? Double ?? 0, y = r.json["y"] as? Double ?? 0
            let double = r.json["double"] as? Bool ?? false
            c.select(c.kennelWindow)
            // Real mouse events through AppKit hit-testing, so the page sees genuine clicks.
            let w = c.kennelWindow!
            let pt = c.web.convert(NSPoint(x: x, y: y), to: nil)
            func click(_ n: Int) {
                for type in [NSEvent.EventType.leftMouseDown, .leftMouseUp] {
                    if let e = NSEvent.mouseEvent(with: type, location: pt, modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime,
                                                  windowNumber: w.windowNumber, context: nil, eventNumber: 0, clickCount: n, pressure: 1) {
                        w.sendEvent(e)
                    }
                }
            }
            click(1)
            // A person's double-click has a short gap; WebKit drops back-to-back synthetic pairs.
            if double { DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) { click(2) } }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { reply(200, ["ok": true]) }
        case ("POST", "/drag"):
            c.select(c.kennelWindow)
            let w = c.kennelWindow!
            let from = NSPoint(x: r.json["x"] as? Double ?? 0, y: r.json["y"] as? Double ?? 0)
            let to = NSPoint(x: r.json["toX"] as? Double ?? 0, y: r.json["toY"] as? Double ?? 0)
            let steps = 10
            func send(_ type: NSEvent.EventType, _ p: NSPoint) {
                if let e = NSEvent.mouseEvent(with: type, location: c.web.convert(p, to: nil), modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime,
                                              windowNumber: w.windowNumber, context: nil, eventNumber: 0, clickCount: 1, pressure: 1) {
                    w.sendEvent(e)
                }
            }
            send(.leftMouseDown, from)
            for i in 1...steps {
                let t = Double(i) / Double(steps)
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.03 * Double(i)) {
                    send(.leftMouseDragged, NSPoint(x: from.x + (to.x - from.x) * t, y: from.y + (to.y - from.y) * t))
                }
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.03 * Double(steps + 1)) { send(.leftMouseUp, to) }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.03 * Double(steps + 1) + 0.5) { reply(200, ["ok": true]) }
        case ("POST", "/dialogs"):
            answers = (r.json["answers"] as? [Any] ?? []).map { $0 as? String }
            reply(200, ["ok": true])
        case ("POST", "/snapshot"):
            let i = r.json["index"] as? Int ?? 0
            let path = r.json["path"] as? String ?? NSTemporaryDirectory() + "kennel-tab.png"
            let ws = c.orderedWindows
            guard ws.indices.contains(i) else { return reply(400, ["error": "no tab \(i)"]) }
            let w = ws[i]
            c.select(w)
            // screencapture shows exactly what's on screen (layer snapshots miss terminal backgrounds).
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) {
                let p = Process()
                p.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture")
                p.arguments = ["-x", "-o", "-l\(w.windowNumber)", path]
                do {
                    try p.run()
                    p.waitUntilExit()
                    if p.terminationStatus == 0 && FileManager.default.fileExists(atPath: path) { return reply(200, ["path": path]) }
                } catch {}
                // Without screen-recording permission, fall back to rendering the view.
                if w === c.kennelWindow {
                    c.web.takeSnapshot(with: nil) { image, _ in
                        try? image?.pngData()?.write(to: URL(fileURLWithPath: path))
                        reply(200, ["path": path, "fallback": true])
                    }
                } else if let v = w.contentView, let png = layerPNG(v) {
                    try? png.write(to: URL(fileURLWithPath: path))
                    reply(200, ["path": path, "fallback": true])
                } else {
                    reply(500, ["error": "snapshot failed"])
                }
            }
        case ("POST", "/quit"):
            reply(200, ["ok": true])
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { NSApp.terminate(nil) }
        default:
            reply(404, ["error": "unknown \(r.method) \(r.path)"])
        }
    }
}

/// Renders a view's layer tree, which (unlike cacheDisplay) includes layer backgrounds.
@MainActor func layerPNG(_ v: NSView) -> Data? {
    v.wantsLayer = true
    guard let layer = v.layer else { return nil }
    let scale = v.window?.backingScaleFactor ?? 2
    let w = Int(v.bounds.width * scale), h = Int(v.bounds.height * scale)
    guard let ctx = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0,
                              space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { return nil }
    ctx.scaleBy(x: scale, y: scale)
    layer.render(in: ctx)
    guard let img = ctx.makeImage() else { return nil }
    return NSBitmapImageRep(cgImage: img).representation(using: .png, properties: [:])
}

extension NSImage {
    func pngData() -> Data? {
        guard let tiff = tiffRepresentation, let rep = NSBitmapImageRep(data: tiff) else { return nil }
        return rep.representation(using: .png, properties: [:])
    }
}

/// The ANSI virtual key code for a character, so synthetic shortcuts look like real keystrokes.
func virtualKeyCode(_ key: String) -> UInt16 {
    let codes: [Character: UInt16] = [
        "a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14,
        "r": 15, "y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "+": 24, "9": 25, "7": 26,
        "-": 27, "_": 27, "8": 28, "0": 29, "]": 30, "}": 30, "{": 33, "<": 43, "o": 31, "u": 32, "[": 33, "i": 34, "p": 35, "\r": 36, "l": 37, "j": 38, "k": 40,
        ",": 43, "n": 45, "m": 46,
        Character(UnicodeScalar(NSLeftArrowFunctionKey)!): 123, Character(UnicodeScalar(NSRightArrowFunctionKey)!): 124,
        Character(UnicodeScalar(NSDownArrowFunctionKey)!): 125, Character(UnicodeScalar(NSUpArrowFunctionKey)!): 126,
    ]
    guard let c = key.lowercased().first else { return 0 }
    return codes[c] ?? 0
}
