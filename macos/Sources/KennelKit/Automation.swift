import AppKit
import Network
import WebKit

/// A tiny HTTP control port for end-to-end UI tests. It exists only when the app is
/// started with KENNEL_APP_AUTOMATION=<port>, and listens on loopback.
///
///   GET  /state                      tabs, selection, status item, page loaded
///   POST /key       {key, mods}      press a shortcut through the real menus
///   POST /select    {index}          select a tab
///   POST /type      {text}           type into the selected terminal
///   GET  /text?index=N               a terminal tab's text
///   POST /js        {code}           evaluate JavaScript in the kennel page
///   POST /click     {x, y}           click inside the kennel page (page coordinates)
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
                     "bg": c.tab(for: w).map { t in t.view.layer?.backgroundColor.flatMap { NSColor(cgColor: $0)?.usingColorSpace(.sRGB).map { String(format: "#%02x%02x%02x", Int($0.redComponent*255), Int($0.greenComponent*255), Int($0.blueComponent*255)) } } ?? "none" } ?? ""]
                },
                "status": c.statusTitle,
                "pageLoaded": c.pageLoaded,
                "url": c.server.url?.absoluteString ?? "",
            ])
        case ("POST", "/key"):
            let key = r.json["key"] as? String ?? ""
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
            let ev = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: flags, timestamp: 0,
                                      windowNumber: c.selectedWindow?.windowNumber ?? 0, context: nil,
                                      characters: key, charactersIgnoringModifiers: key, isARepeat: false, keyCode: 0)!
            let handled = NSApp.mainMenu?.performKeyEquivalent(with: ev) ?? false
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { reply(200, ["handled": handled]) }
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
            reply(200, ["text": t.text()])
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
