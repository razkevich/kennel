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

public final class AppController: NSObject, NSApplicationDelegate, NSWindowDelegate, WKScriptMessageHandler {
    let env = ProcessInfo.processInfo.environment
    lazy var demo = env["KENNEL_APP_DEMO"] == "1"
    lazy var fakeRun = env["KENNEL_APP_FAKE_RUN"] == "1"
    let server = KennelServer()
    var theme = GhosttyTheme.load()
    private var nextPaneID = 1

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

    var selectedTab: TerminalTab? { tab(for: selectedWindow) }

    @discardableResult
    func openTab(_ launch: Launch?) -> TerminalTab {
        if let s = launch?.session, let existing = tabs.first(where: { $0.pane(session: s) != nil }) {
            select(existing.window)
            if let p = existing.pane(session: s) { existing.focus(p.id) }
            return existing
        }
        let w = NSWindow(contentRect: kennelWindow.frame, styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        w.tabbingIdentifier = "kennel"
        w.tabbingMode = .preferred
        w.title = launch?.title ?? "shell"
        w.isReleasedWhenClosed = false
        w.delegate = self
        let bg = GhosttyTheme.color(theme.background)
        w.backgroundColor = bg
        w.appearance = NSAppearance(named: .darkAqua)
        // New tabs start where the current terminal is, like Ghostty's window-inherit-working-directory.
        let pane = makePane(launch, cwd: launch?.cwd ?? selectedTab?.focused.cwd, fontSize: theme.fontSize)
        let t = TerminalTab(window: w, first: pane, fontSize: theme.fontSize, controller: self)
        styleContainer(t.container)
        tabs.append(t)
        let last = orderedWindows.last ?? kennelWindow!
        last.addTabbedWindow(w, ordered: .above)
        select(w)
        t.layoutPanes()
        t.focus(pane.id)
        return t
    }

    func styleContainer(_ c: SplitContainerView) {
        c.wantsLayer = true
        let bg = GhosttyTheme.color(theme.background)
        c.layer?.backgroundColor = bg.cgColor
        c.dividerColor = theme.splitDividerColor.map(GhosttyTheme.color) ?? bg.blended(withFraction: 0.35, of: .white) ?? .gray
        c.needsDisplay = true
    }

    /// A terminal running `launch` (or a login shell) with the Ghostty look.
    func makePane(_ launch: Launch?, cwd: String?, fontSize: CGFloat) -> Pane {
        let view = LocalProcessTerminalView(frame: NSRect(x: 0, y: 0, width: 400, height: 300))
        applyTheme(to: view, fontSize: fontSize)
        let holder = PaneView(terminal: view, background: view.nativeBackgroundColor, padding: 6, dim: theme.unfocusedSplitOpacity)
        let dir = cwd ?? NSHomeDirectory()
        let p = Pane(id: nextPaneID, view: view, holder: holder, session: launch?.session, title: launch?.title ?? "shell", cwd: dir)
        nextPaneID += 1
        view.processDelegate = p

        let shell = env["SHELL"] ?? "/bin/zsh"
        var childEnv = ProcessInfo.processInfo.environment
        childEnv["TERM"] = "xterm-256color"
        childEnv["COLORTERM"] = "truecolor"
        childEnv["TERM_PROGRAM"] = "kennel"
        childEnv.removeValue(forKey: "KENNEL_APP_AUTOMATION")
        let envList = childEnv.map { "\($0.key)=\($0.value)" }
        if let l = launch {
            view.startProcess(executable: fakeRun ? "/bin/sh" : shell, args: l.shellArgs(fake: fakeRun), environment: envList, execName: nil, currentDirectory: dir)
        } else {
            view.startProcess(executable: shell, args: ["-l"], environment: envList, execName: "-" + (shell as NSString).lastPathComponent, currentDirectory: dir)
        }
        return p
    }

    func applyTheme(to view: LocalProcessTerminalView, fontSize: CGFloat) {
        var t = theme
        t.fontSize = fontSize
        view.font = t.font
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
        view.layer?.backgroundColor = view.nativeBackgroundColor.cgColor
        if let c = theme.cursor { view.caretColor = GhosttyTheme.color(c) }
        if let c = theme.selection { view.selectedTextBackgroundColor = GhosttyTheme.color(c) }
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

    func closePane(_ p: Pane, terminate: Bool) {
        if terminate { p.view.terminate() }
        guard let t = p.tab, tabs.contains(where: { $0 === t }) else { return }
        if !t.remove(p) { closeTab(t) }
    }

    public func windowShouldClose(_ sender: NSWindow) -> Bool {
        // The kennel tab is pinned: closing it would leave terminals without their home.
        if sender === kennelWindow { NSSound.beep(); return false }
        if let t = tab(for: sender) {
            tabs.removeAll { $0 === t }
            t.orderedPanes.forEach { $0.view.terminate() }
        }
        return true
    }

    var selectedWindow: NSWindow? { kennelWindow.tabGroup?.selectedWindow ?? kennelWindow }

    // MARK: menu actions

    @objc func newTab(_ sender: Any?) { openTab(nil) }
    /// ⌘W closes the focused split (and its tab with the last one), like Ghostty's close_surface.
    @objc func closeCurrent(_ sender: Any?) {
        guard let t = selectedTab else { NSSound.beep(); return }
        closePane(t.focused, terminate: true)
    }
    @objc func closeCurrentTab(_ sender: Any?) {
        guard let t = selectedTab else { NSSound.beep(); return }
        t.orderedPanes.forEach { $0.view.terminate() }
        closeTab(t)
    }

    // MARK: splits

    func split(_ dir: SplitDirection) {
        guard let t = selectedTab else { NSSound.beep(); return }
        t.split(with: makePane(nil, cwd: t.focused.cwd, fontSize: t.fontSize), dir)
    }
    @objc func splitRight(_ sender: Any?) { split(.right) }
    @objc func splitDown(_ sender: Any?) { split(.down) }
    @objc func splitLeft(_ sender: Any?) { split(.left) }
    @objc func splitUp(_ sender: Any?) { split(.up) }
    @objc func previousSplit(_ sender: Any?) { if let t = selectedTab, let id = t.tree.cycle(from: t.focusedID, by: -1) { t.focus(id) } }
    @objc func nextSplit(_ sender: Any?) { if let t = selectedTab, let id = t.tree.cycle(from: t.focusedID, by: 1) { t.focus(id) } }
    @objc func gotoSplit(_ sender: NSMenuItem) {
        guard let t = selectedTab, let dir = Self.directions[sender.tag] else { return }
        if let id = t.tree.neighbor(of: t.focusedID, dir, in: t.container.bounds) { t.focus(id) } else { NSSound.beep() }
    }
    @objc func resizeSplit(_ sender: NSMenuItem) {
        guard let t = selectedTab, let dir = Self.directions[sender.tag] else { return }
        let span = dir.axis == .horizontal ? t.container.bounds.width : t.container.bounds.height
        t.tree = t.tree.resizing(t.focusedID, dir, by: 10 / max(span, 1))
        t.layoutPanes()
    }
    @objc func equalizeSplits(_ sender: Any?) { if let t = selectedTab { t.tree = t.tree.equalized(); t.layoutPanes() } }
    @objc func toggleSplitZoom(_ sender: Any?) {
        guard let t = selectedTab, t.panes.count > 1 else { NSSound.beep(); return }
        t.zoomedID = t.zoomedID == nil ? t.focusedID : nil
        t.focus(t.focusedID)
    }
    static let directions: [Int: SplitDirection] = [1: .up, 2: .down, 3: .left, 4: .right]

    // MARK: terminal actions

    /// Font size per tab, like Ghostty; in the Kennel tab the same keys zoom the page.
    func setFontSize(_ f: (CGFloat) -> CGFloat) {
        if selectedWindow === kennelWindow {
            web.pageZoom = min(2, max(0.5, f(web.pageZoom * 10) / 10))
            return
        }
        guard let t = selectedTab else { return }
        t.fontSize = min(48, max(6, f(t.fontSize)))
        for p in t.orderedPanes { applyTheme(to: p.view, fontSize: t.fontSize) }
    }
    @objc func increaseFontSize(_ sender: Any?) { setFontSize { $0 + 1 } }
    @objc func decreaseFontSize(_ sender: Any?) { setFontSize { $0 - 1 } }
    @objc func resetFontSize(_ sender: Any?) {
        if selectedWindow === kennelWindow { web.pageZoom = 1; return }
        setFontSize { _ in theme.fontSize }
    }
    @objc func clearScreen(_ sender: Any?) {
        guard let t = selectedTab else { return }
        t.view.feed(text: "\u{1b}[H\u{1b}[2J\u{1b}[3J")
        // Ask the shell to redraw its prompt on the clean screen.
        t.view.send(txt: "\u{0c}")
    }
    @objc func toggleFullScreen(_ sender: Any?) { selectedWindow?.toggleFullScreen(nil) }
    @objc func reloadConfig(_ sender: Any?) {
        theme = GhosttyTheme.load()
        for t in tabs {
            styleContainer(t.container)
            t.window.backgroundColor = GhosttyTheme.color(theme.background)
            for p in t.orderedPanes { applyTheme(to: p.view, fontSize: t.fontSize) }
        }
    }
    @objc func openConfig(_ sender: Any?) {
        let path = NSHomeDirectory() + "/.config/ghostty/config"
        if !FileManager.default.fileExists(atPath: path) {
            try? FileManager.default.createDirectory(atPath: (path as NSString).deletingLastPathComponent, withIntermediateDirectories: true)
            FileManager.default.createFile(atPath: path, contents: Data("# Ghostty config, also read by Kennel\n".utf8))
        }
        NSWorkspace.shared.open(URL(fileURLWithPath: path))
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
            // Shifted keys are spelled as the character Shift produces ("D", "{"): with a Shift mask
            // instead, AppKit also fires the unshifted shortcut's neighbor (⌘D for ⌘⇧D, ⌘⇧[ for ⌘[).
            var key = key, mods = mods
            if mods.contains(.shift), let shifted = shiftedKey(key) { key = shifted; mods.remove(.shift) }
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
        func arrow(_ c: Int) -> String { String(Character(UnicodeScalar(c)!)) }
        func tagged(_ i: NSMenuItem, _ tag: Int) -> NSMenuItem { i.tag = tag; return i }
        let arrows = [(1, "Up", NSUpArrowFunctionKey), (2, "Down", NSDownArrowFunctionKey), (3, "Left", NSLeftArrowFunctionKey), (4, "Right", NSRightArrowFunctionKey)]
        sub("File", [
            item("New Tab", #selector(newTab(_:)), "t", target: self),
            .separator(),
            item("Split Right", #selector(splitRight(_:)), "d", target: self),
            item("Split Down", #selector(splitDown(_:)), "d", [.command, .shift], target: self),
            item("Split Left", #selector(splitLeft(_:)), "", target: self),
            item("Split Up", #selector(splitUp(_:)), "", target: self),
            .separator(),
            item("Close", #selector(closeCurrent(_:)), "w", target: self),
            item("Close Tab", #selector(closeCurrentTab(_:)), "w", [.command, .option], target: self),
        ])
        func find(_ title: String, _ action: NSTextFinder.Action, _ key: String, _ mods: NSEvent.ModifierFlags = [.command]) -> NSMenuItem {
            tagged(item(title, #selector(NSResponder.performTextFinderAction(_:)), key, mods), action.rawValue)
        }
        sub("Edit", [
            item("Copy", #selector(NSText.copy(_:)), "c"),
            item("Paste", #selector(NSText.paste(_:)), "v"),
            item("Select All", #selector(NSText.selectAll(_:)), "a"),
            .separator(),
            item("Clear Screen", #selector(clearScreen(_:)), "k", target: self),
            .separator(),
            find("Find…", .showFindInterface, "f"),
            find("Find Next", .nextMatch, "g"),
            find("Find Previous", .previousMatch, "g", [.command, .shift]),
            find("Use Selection for Find", .setSearchString, "e"),
            find("Hide Find Bar", .hideFindInterface, ""),
        ])
        sub("View", [
            item("Increase Font Size", #selector(increaseFontSize(_:)), "="),
            item("Increase Font Size", #selector(increaseFontSize(_:)), "+"),
            item("Decrease Font Size", #selector(decreaseFontSize(_:)), "-"),
            item("Reset Font Size", #selector(resetFontSize(_:)), "0"),
            .separator(),
            item("Zoom Split", #selector(toggleSplitZoom(_:)), "\r", [.command, .shift]),
            item("Toggle Full Screen", #selector(toggleFullScreen(_:)), "\r"),
            .separator(),
            item("Open Ghostty Config", #selector(openConfig(_:)), ","),
            item("Reload Config", #selector(reloadConfig(_:)), ",", [.command, .shift]),
        ].map { $0.target = $0.target ?? self; return $0 })
        var splitItems: [NSMenuItem] = [
            item("Select Previous Split", #selector(previousSplit(_:)), "[", target: self),
            item("Select Next Split", #selector(nextSplit(_:)), "]", target: self),
        ]
        for (tag, name, key) in arrows { splitItems.append(tagged(item("Select Split \(name)", #selector(gotoSplit(_:)), arrow(key), [.command, .option], target: self), tag)) }
        splitItems.append(.separator())
        for (tag, name, key) in arrows { splitItems.append(tagged(item("Resize Split \(name)", #selector(resizeSplit(_:)), arrow(key), [.command, .control], target: self), tag)) }
        splitItems.append(item("Equalize Splits", #selector(equalizeSplits(_:)), "=", [.command, .control], target: self))
        var tabItems: [NSMenuItem] = [
            item("Kennel", #selector(showKennel(_:)), "k", [.command, .shift], target: self),
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
        tabItems += [.separator()] + splitItems
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

/// What Shift turns a key into on a US keyboard, or nil for keys it doesn't change (arrows, Return).
func shiftedKey(_ key: String) -> String? {
    guard key.count == 1, let c = key.first else { return nil }
    if c.isLetter { return key.uppercased() }
    let map: [Character: String] = ["[": "{", "]": "}", "=": "+", "-": "_", ",": "<", ".": ">", "/": "?", ";": ":", "'": "\"", "\\": "|", "`": "~",
                                    "1": "!", "2": "@", "3": "#", "4": "$", "5": "%", "6": "^", "7": "&", "8": "*", "9": "(", "0": ")"]
    return map[c]
}
