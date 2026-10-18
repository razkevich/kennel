import AppKit
import SwiftTerm
import WebKit

public enum KennelApp {
    public static func main() {
        let app = NSApplication.shared
        let controller = AppController()
        app.delegate = controller
        app.run()
    }
}

/// One terminal tab: a window in the kennel tab group running a process.
final class TerminalTab: NSObject, LocalProcessTerminalViewDelegate {
    let window: NSWindow
    let view: LocalProcessTerminalView
    let session: String?
    weak var controller: AppController?

    init(window: NSWindow, view: LocalProcessTerminalView, session: String?) {
        self.window = window
        self.view = view
        self.session = session
    }

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}
    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {
        DispatchQueue.main.async { self.window.title = title.isEmpty ? "kennel" : title }
    }
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func processTerminated(source: TerminalView, exitCode: Int32?) {
        // Like Ghostty: when the program exits, its tab goes away.
        DispatchQueue.main.async { self.controller?.closeTab(self) }
    }

    /// Everything currently on screen and in scrollback, for tests and automation.
    @MainActor func text() -> String {
        view.selectAll()
        let s = view.getSelection() ?? ""
        view.selectNone()
        return s
    }
}

public final class AppController: NSObject, NSApplicationDelegate, NSWindowDelegate, WKScriptMessageHandler {
    let env = ProcessInfo.processInfo.environment
    lazy var demo = env["KENNEL_APP_DEMO"] == "1"
    lazy var fakeRun = env["KENNEL_APP_FAKE_RUN"] == "1"
    let server = KennelServer()
    let theme = GhosttyTheme.load()

    var kennelWindow: NSWindow!
    var web: WKWebView!
    var tabs: [TerminalTab] = []
    var statusItem: NSStatusItem?
    var counts: [String: Int] = [:]
    var automation: Automation?
    public private(set) var pageLoaded = false

    public func applicationDidFinishLaunching(_ notification: Notification) {
        let automated = env["KENNEL_APP_AUTOMATION"] != nil
        // Under automation the app never takes focus away from the person using the Mac.
        NSApp.setActivationPolicy(automated ? .accessory : .regular)
        buildMenu()
        buildKennelWindow()
        buildStatusItem()
        if let port = env["KENNEL_APP_AUTOMATION"].flatMap(UInt16.init) {
            automation = Automation(controller: self, port: port)
        }
        server.start(demo: demo) { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let url): self.web.load(URLRequest(url: url))
            case .failure(let err): self.showError(err.localizedDescription)
            }
        }
        if automated {
            kennelWindow.orderFrontRegardless()
        } else {
            kennelWindow.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
        }
    }

    public func applicationWillTerminate(_ notification: Notification) { server.stop() }
    public func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    // MARK: windows

    func buildKennelWindow() {
        let w = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1400, height: 880),
                         styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        w.title = "▼•ᴥ•▼ Kennel"
        w.tabbingIdentifier = "kennel"
        w.tabbingMode = .preferred
        w.delegate = self
        w.isReleasedWhenClosed = false
        w.center()
        w.setFrameAutosaveName("KennelMain")
        let cfg = WKWebViewConfiguration()
        cfg.userContentController.add(self, name: "kennel")
        cfg.preferences.setValue(true, forKey: "developerExtrasEnabled")
        web = KennelWebView(frame: w.contentView!.bounds, configuration: cfg)
        web.autoresizingMask = [.width, .height]
        web.setValue(false, forKey: "drawsBackground")
        web.navigationDelegate = self
        web.uiDelegate = self
        w.contentView!.addSubview(web)
        w.backgroundColor = NSColor(srgbRed: 0.961, green: 0.957, blue: 0.945, alpha: 1)
        w.appearance = NSAppearance(named: .aqua)
        kennelWindow = w
    }

    func showError(_ msg: String) {
        let html = "<body style='background:#1d2140;color:#f4f1ea;font:15px system-ui;padding:40px'><h2>▼-ᴥ-▼ kennel couldn't start</h2><p>\(msg)</p></body>"
        web.loadHTMLString(html, baseURL: nil)
    }

    /// The tab windows in on-screen order, kennel first.
    var orderedWindows: [NSWindow] { kennelWindow.tabbedWindows ?? [kennelWindow] }

    func tab(for window: NSWindow?) -> TerminalTab? { tabs.first { $0.window === window } }

    @discardableResult
    func openTab(_ launch: Launch?) -> TerminalTab {
        if let s = launch?.session, let existing = tabs.first(where: { $0.session == s }) {
            select(existing.window)
            return existing
        }
        let w = NSWindow(contentRect: kennelWindow.frame, styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        w.tabbingIdentifier = "kennel"
        w.tabbingMode = .preferred
        w.title = launch?.title ?? "shell"
        w.isReleasedWhenClosed = false
        w.delegate = self
        // A little breathing room around the grid, like Ghostty's window padding.
        let pad: CGFloat = 6
        let view = LocalProcessTerminalView(frame: w.contentView!.bounds.insetBy(dx: pad, dy: pad))
        view.autoresizingMask = [.width, .height]
        view.font = theme.font
        if theme.palette.count >= 16 {
            view.installColors((0..<16).map { i in
                let c = GhosttyTheme.color(theme.palette[i] ?? "#888888").usingColorSpace(.sRGB) ?? .gray
                return SwiftTerm.Color(red: UInt16(c.redComponent * 65535), green: UInt16(c.greenComponent * 65535), blue: UInt16(c.blueComponent * 65535))
            })
        }
        // Default colors go after the palette: installColors resets the color state.
        // The view needs its layer first, or the background setter has nothing to paint.
        view.wantsLayer = true
        view.nativeBackgroundColor = GhosttyTheme.color(theme.background)
        view.nativeForegroundColor = GhosttyTheme.color(theme.foreground)
        if let c = theme.cursor { view.caretColor = GhosttyTheme.color(c) }
        if let c = theme.selection { view.selectedTextBackgroundColor = GhosttyTheme.color(c) }
        w.backgroundColor = view.nativeBackgroundColor
        w.contentView!.wantsLayer = true
        w.contentView!.layer?.backgroundColor = view.nativeBackgroundColor.cgColor
        w.contentView!.addSubview(view)
        view.layer?.backgroundColor = view.nativeBackgroundColor.cgColor
        w.appearance = NSAppearance(named: .darkAqua)
        let t = TerminalTab(window: w, view: view, session: launch?.session)
        t.controller = self
        view.processDelegate = t
        tabs.append(t)

        let shell = env["SHELL"] ?? "/bin/zsh"
        var childEnv = ProcessInfo.processInfo.environment
        childEnv["TERM"] = "xterm-256color"
        childEnv["COLORTERM"] = "truecolor"
        childEnv["TERM_PROGRAM"] = "kennel"
        childEnv.removeValue(forKey: "KENNEL_APP_AUTOMATION")
        let envList = childEnv.map { "\($0.key)=\($0.value)" }
        let cwd = launch?.cwd ?? NSHomeDirectory()
        if let l = launch {
            view.startProcess(executable: fakeRun ? "/bin/sh" : shell, args: l.shellArgs(fake: fakeRun), environment: envList, execName: nil, currentDirectory: cwd)
        } else {
            view.startProcess(executable: shell, args: ["-l"], environment: envList, execName: "-" + (shell as NSString).lastPathComponent, currentDirectory: cwd)
        }
        let last = orderedWindows.last ?? kennelWindow!
        last.addTabbedWindow(w, ordered: .above)
        select(w)
        w.makeFirstResponder(view)
        return t
    }

    func select(_ w: NSWindow) {
        if NSApp.activationPolicy() == .regular {
            w.makeKeyAndOrderFront(nil)
        } else {
            w.tabGroup?.selectedWindow = w
            w.orderFrontRegardless()
        }
    }

    func closeTab(_ t: TerminalTab) {
        tabs.removeAll { $0 === t }
        t.window.close()
    }

    public func windowShouldClose(_ sender: NSWindow) -> Bool {
        // The kennel tab is pinned: closing it would leave terminals without their home.
        if sender === kennelWindow { NSSound.beep(); return false }
        if let t = tab(for: sender) {
            tabs.removeAll { $0 === t }
            t.view.terminate()
        }
        return true
    }

    var selectedWindow: NSWindow? { kennelWindow.tabGroup?.selectedWindow ?? kennelWindow }

    // MARK: menu actions

    @objc func newTab(_ sender: Any?) { openTab(nil) }
    @objc func closeCurrentTab(_ sender: Any?) {
        guard let w = selectedWindow else { return }
        if w === kennelWindow { NSSound.beep(); return }
        if let t = tab(for: w) { t.view.terminate(); closeTab(t) }
    }
    @objc func selectTabNumber(_ sender: NSMenuItem) {
        let ws = orderedWindows
        let i = sender.tag == 9 ? ws.count - 1 : sender.tag - 1
        if ws.indices.contains(i) { select(ws[i]) }
    }
    @objc func showKennel(_ sender: Any?) { select(kennelWindow) }
    @objc func nextTab(_ sender: Any?) { stepTab(1) }
    @objc func previousTab(_ sender: Any?) { stepTab(-1) }

    func stepTab(_ d: Int) {
        let ws = orderedWindows
        guard let cur = selectedWindow, let i = ws.firstIndex(where: { $0 === cur }), !ws.isEmpty else { return }
        select(ws[(i + d + ws.count) % ws.count])
    }
    @objc func reloadKennel(_ sender: Any?) { web.reload() }

    func buildMenu() {
        let main = NSMenu()
        func sub(_ title: String, _ items: [NSMenuItem]) {
            let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
            let m = NSMenu(title: title)
            items.forEach(m.addItem)
            item.submenu = m
            main.addItem(item)
        }
        func item(_ title: String, _ action: Selector?, _ key: String, _ mods: NSEvent.ModifierFlags = [.command], target: AnyObject? = nil) -> NSMenuItem {
            let i = NSMenuItem(title: title, action: action, keyEquivalent: key)
            i.keyEquivalentModifierMask = mods
            i.target = target
            return i
        }
        sub("Kennel", [
            item("About Kennel", #selector(NSApplication.orderFrontStandardAboutPanel(_:)), ""),
            .separator(),
            item("Hide Kennel", #selector(NSApplication.hide(_:)), "h"),
            item("Quit Kennel", #selector(NSApplication.terminate(_:)), "q"),
        ])
        sub("File", [
            item("New Tab", #selector(newTab(_:)), "t", target: self),
            item("Close Tab", #selector(closeCurrentTab(_:)), "w", target: self),
        ])
        sub("Edit", [
            item("Copy", #selector(NSText.copy(_:)), "c"),
            item("Paste", #selector(NSText.paste(_:)), "v"),
            item("Select All", #selector(NSText.selectAll(_:)), "a"),
        ])
        var tabItems: [NSMenuItem] = [
            item("Kennel", #selector(showKennel(_:)), "0", target: self),
            item("Reload Kennel", #selector(reloadKennel(_:)), "r", [.command, .shift], target: self),
            .separator(),
            item("Show Previous Tab", #selector(previousTab(_:)), "[", [.command, .shift], target: self),
            item("Show Next Tab", #selector(nextTab(_:)), "]", [.command, .shift], target: self),
            .separator(),
        ]
        for n in 1...9 {
            let i = item(n == 9 ? "Last Tab" : "Tab \(n)", #selector(selectTabNumber(_:)), "\(n)", target: self)
            i.tag = n
            tabItems.append(i)
        }
        sub("Window", tabItems + [.separator(), item("Minimize", #selector(NSWindow.performMiniaturize(_:)), "m")])
        NSApp.mainMenu = main
    }

    // MARK: status item

    func buildStatusItem() {
        let s = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        s.button?.title = "▼•ᴥ•▼"
        s.button?.target = self
        s.button?.action = #selector(statusClicked(_:))
        statusItem = s
    }

    var statusTitle: String { statusItem?.button?.title ?? "" }

    func updateStatus() {
        let waiting = counts["waiting"] ?? 0
        let working = counts["working"] ?? 0
        var t = waiting > 0 ? "▼°ᴥ°▼ \(waiting)" : "▼•ᴥ•▼"
        if working > 0 { t += " ⚡\(working)" }
        statusItem?.button?.title = t
    }

    @objc func statusClicked(_ sender: Any?) {
        NSApp.activate(ignoringOtherApps: true)
        select(kennelWindow)
    }

    // MARK: page bridge

    public func userContentController(_ ucc: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let type = body["type"] as? String else { return }
        switch type {
        case "run":
            guard let run = body["run"] as? [String: Any], let launch = Launch(message: run) else { return }
            if launch.focus, let s = launch.session, !tabs.contains(where: { $0.session == s }) {
                // Running in another terminal (herdr, Ghostty): opening it again would fork the conversation.
                let msg = "\(launch.title) is running in another terminal"
                web.evaluateJavaScript("window.kennel && window.kennel.toast(\(jsString(msg)), true)")
                return
            }
            openTab(launch)
        case "counts":
            if let c = body["counts"] as? [String: Int] { counts = c; updateStatus() }
        default:
            break
        }
    }
}

extension AppController: WKNavigationDelegate {
    public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) { pageLoaded = true }

    public func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        // Links out of the map (PRs, artifacts) open in the default browser.
        if let url = action.request.url, url.host != "127.0.0.1", url.scheme?.hasPrefix("http") == true {
            NSWorkspace.shared.open(url)
            decisionHandler(.cancel)
            return
        }
        decisionHandler(.allow)
    }

}

extension AppController: WKUIDelegate {
    public func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let a = NSAlert()
        a.messageText = message
        a.runModal()
        completionHandler()
    }

    public func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        if let answer = automation?.nextDialogAnswer() { completionHandler(answer != nil); return }
        let a = NSAlert()
        a.messageText = message
        a.addButton(withTitle: "OK")
        a.addButton(withTitle: "Cancel")
        completionHandler(a.runModal() == .alertFirstButtonReturn)
    }

    public func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String, defaultText: String?, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (String?) -> Void) {
        if let answer = automation?.nextDialogAnswer() { completionHandler(answer); return }
        let a = NSAlert()
        a.messageText = prompt
        let field = NSTextField(frame: NSRect(x: 0, y: 0, width: 320, height: 24))
        field.stringValue = defaultText ?? ""
        a.accessoryView = field
        a.addButton(withTitle: "OK")
        a.addButton(withTitle: "Cancel")
        a.window.initialFirstResponder = field
        completionHandler(a.runModal() == .alertFirstButtonReturn ? field.stringValue : nil)
    }

    public func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let url = navigationAction.request.url { NSWorkspace.shared.open(url) }
        return nil
    }
}

/// Clicks act right away, even when Kennel isn't the focused app: clicking a pup
/// shouldn't first need a click to focus the window.
final class KennelWebView: WKWebView {
    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
}

func jsString(_ s: String) -> String {
    let data = try? JSONSerialization.data(withJSONObject: [s])
    let arr = data.flatMap { String(data: $0, encoding: .utf8) } ?? "[\"\"]"
    return String(arr.dropFirst().dropLast())
}
