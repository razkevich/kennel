import AppKit
import SwiftTerm

/// One terminal surface: a process in a view. A tab holds one or more of these as splits.
final class Pane: NSObject, LocalProcessTerminalViewDelegate {
    let id: Int
    let view: LocalProcessTerminalView
    let holder: PaneView
    let session: String?
    var title: String
    var cwd: String
    weak var tab: TerminalTab?

    init(id: Int, view: LocalProcessTerminalView, holder: PaneView, session: String?, title: String, cwd: String) {
        self.id = id
        self.view = view
        self.holder = holder
        self.session = session
        self.title = title
        self.cwd = cwd
    }

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}
    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {
        DispatchQueue.main.async {
            self.title = title.isEmpty ? "kennel" : title
            self.tab?.paneTitleChanged(self)
        }
    }
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {
        // OSC 7 reports file://host/path; new splits and tabs start there, like Ghostty.
        guard let d = directory else { return }
        let path = URL(string: d)?.path ?? d
        if !path.isEmpty { DispatchQueue.main.async { self.cwd = path } }
    }
    func processTerminated(source: TerminalView, exitCode: Int32?) {
        // Like Ghostty: when the program exits, its split (or tab) goes away.
        DispatchQueue.main.async { self.tab?.controller?.closePane(self, terminate: false) }
    }

    /// Everything currently on screen and in scrollback, for tests and automation.
    @MainActor func text() -> String {
        view.selectAll()
        let s = view.getSelection() ?? ""
        view.selectNone()
        return s
    }
}

/// A pane's box: window padding around the terminal, and a veil that dims it while
/// another split has focus (Ghostty's unfocused-split-opacity).
final class PaneView: NSView {
    let veil = Veil()

    init(terminal: NSView, background: NSColor, padding: CGFloat, dim: CGFloat) {
        super.init(frame: NSRect(x: 0, y: 0, width: 400, height: 300))
        wantsLayer = true
        layer?.backgroundColor = background.cgColor
        terminal.frame = bounds.insetBy(dx: padding, dy: padding)
        terminal.autoresizingMask = [.width, .height]
        addSubview(terminal)
        veil.frame = bounds
        veil.autoresizingMask = [.width, .height]
        veil.wantsLayer = true
        veil.layer?.backgroundColor = background.withAlphaComponent(1 - dim).cgColor
        veil.isHidden = true
        addSubview(veil)
    }

    required init?(coder: NSCoder) { fatalError() }

    final class Veil: NSView {
        override func hitTest(_ point: NSPoint) -> NSView? { nil }
    }
}

/// Lays a tab's panes out from its split tree and lets you drag the dividers.
final class SplitContainerView: NSView {
    weak var tab: TerminalTab?
    var dividerWidth: CGFloat = 1
    var dividerColor: NSColor = .separatorColor
    private let grab: CGFloat = 4

    override var isFlipped: Bool { true }

    override func layout() {
        super.layout()
        tab?.layoutPanes()
    }

    private var dividers: [(path: [Bool], axis: SplitAxis, rect: CGRect, bounds: CGRect)] {
        guard let tab, tab.zoomedID == nil else { return [] }
        return tab.tree.dividers(in: bounds, divider: dividerWidth)
    }

    private func divider(at p: NSPoint) -> (path: [Bool], axis: SplitAxis, rect: CGRect, bounds: CGRect)? {
        dividers.first { $0.rect.insetBy(dx: $0.axis == .horizontal ? -grab : 0, dy: $0.axis == .vertical ? -grab : 0).contains(p) }
    }

    override func draw(_ dirtyRect: NSRect) {
        dividerColor.setFill()
        for d in dividers { d.rect.fill() }
    }

    override func hitTest(_ point: NSPoint) -> NSView? {
        let p = convert(point, from: superview)
        return divider(at: p) != nil ? self : super.hitTest(point)
    }

    override func resetCursorRects() {
        for d in dividers {
            addCursorRect(d.rect.insetBy(dx: d.axis == .horizontal ? -grab : 0, dy: d.axis == .vertical ? -grab : 0),
                          cursor: d.axis == .horizontal ? .resizeLeftRight : .resizeUpDown)
        }
    }

    override func mouseDown(with event: NSEvent) {
        guard let tab, let d = divider(at: convert(event.locationInWindow, from: nil)) else { return }
        while let e = window?.nextEvent(matching: [.leftMouseDragged, .leftMouseUp]) {
            let p = convert(e.locationInWindow, from: nil)
            let ratio = d.axis == .horizontal ? (p.x - d.bounds.minX) / d.bounds.width : (p.y - d.bounds.minY) / d.bounds.height
            tab.tree = tab.tree.settingRatio(at: d.path, ratio)
            tab.layoutPanes()
            if e.type == .leftMouseUp { break }
        }
        window?.invalidateCursorRects(for: self)
    }
}

/// One terminal tab: a window in the kennel tab group holding a tree of split panes.
@MainActor
final class TerminalTab: NSObject {
    let window: NSWindow
    let container: SplitContainerView
    var tree: SplitTree
    private(set) var panes: [Int: Pane] = [:]
    private(set) var focusedID: Int
    var zoomedID: Int? { didSet { layoutPanes(); updateVeils() } }
    var fontSize: CGFloat
    weak var controller: AppController?
    private var observation: NSKeyValueObservation?

    init(window: NSWindow, first: Pane, fontSize: CGFloat, controller: AppController) {
        self.window = window
        self.tree = .leaf(first.id)
        self.focusedID = first.id
        self.fontSize = fontSize
        self.controller = controller
        container = SplitContainerView(frame: window.contentView!.bounds)
        super.init()
        container.autoresizingMask = [.width, .height]
        container.tab = self
        window.contentView!.addSubview(container)
        adopt(first)
        observation = window.observe(\.firstResponder, options: [.new]) { [weak self] w, _ in
            MainActor.assumeIsolated {
                guard let self, let r = w.firstResponder as? NSView,
                      let p = self.panes.values.first(where: { r.isDescendant(of: $0.holder) }), p.id != self.focusedID else { return }
                self.focus(p.id, makeFirstResponder: false)
            }
        }
    }

    var focused: Pane { panes[focusedID]! }
    var view: LocalProcessTerminalView { focused.view }
    var orderedPanes: [Pane] { tree.leaves.compactMap { panes[$0] } }
    var session: String? { focused.session ?? orderedPanes.compactMap(\.session).first }
    func pane(session s: String) -> Pane? { panes.values.first { $0.session == s } }

    private func adopt(_ p: Pane) {
        p.tab = self
        panes[p.id] = p
        container.addSubview(p.holder)
    }

    func split(with p: Pane, _ dir: SplitDirection) {
        adopt(p)
        tree = tree.splitting(focusedID, with: p.id, dir)
        zoomedID = nil
        focus(p.id)
    }

    /// Removes a pane; returns false when it was the last one and the tab should close.
    func remove(_ p: Pane) -> Bool {
        guard let rest = tree.removing(p.id) else { return false }
        let next = tree.cycle(from: p.id, by: -1) ?? rest.leaves[0]
        tree = rest
        panes[p.id] = nil
        p.holder.removeFromSuperview()
        if zoomedID == p.id { zoomedID = nil }
        layoutPanes()
        if p.id == focusedID { focus(rest.contains(next) ? next : rest.leaves[0]) } else { updateVeils() }
        return true
    }

    func focus(_ id: Int, makeFirstResponder: Bool = true) {
        guard panes[id] != nil else { return }
        focusedID = id
        if zoomedID != nil && zoomedID != id { zoomedID = id }
        if makeFirstResponder { window.makeFirstResponder(view) }
        window.title = focused.title
        updateVeils()
    }

    func paneTitleChanged(_ p: Pane) { if p.id == focusedID { window.title = p.title } }

    func layoutPanes() {
        let r = container.bounds
        if let z = zoomedID, let zp = panes[z] {
            for p in panes.values { p.holder.isHidden = p.id != z }
            zp.holder.frame = r
        } else {
            for (id, f) in tree.frames(in: r, divider: container.dividerWidth) {
                panes[id]?.holder.isHidden = false
                panes[id]?.holder.frame = f
            }
        }
        container.needsDisplay = true
        window.invalidateCursorRects(for: container)
    }

    func updateVeils() {
        let several = panes.count > 1 && zoomedID == nil
        for p in panes.values { p.holder.veil.isHidden = !several || p.id == focusedID }
    }
}
