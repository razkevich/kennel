import CoreGraphics

/// How a split lays out its two sides: `.horizontal` puts them side by side (⌘D),
/// `.vertical` stacks them (⌘⇧D). Same naming as Ghostty.
public enum SplitAxis: Equatable, Sendable { case horizontal, vertical }

public enum SplitDirection: Equatable, Sendable {
    case left, right, up, down

    var axis: SplitAxis { self == .left || self == .right ? .horizontal : .vertical }
    var forward: Bool { self == .right || self == .down }
}

/// A tab's panes as a binary tree, like Ghostty's: leaves are pane ids, and each split
/// gives `ratio` of its space to the first side.
public indirect enum SplitTree: Equatable, Sendable {
    case leaf(Int)
    case split(SplitAxis, CGFloat, SplitTree, SplitTree)

    public var leaves: [Int] {
        switch self {
        case .leaf(let id): return [id]
        case .split(_, _, let a, let b): return a.leaves + b.leaves
        }
    }

    public func contains(_ id: Int) -> Bool { leaves.contains(id) }

    /// Splits `id`, putting `newID` after it (right or below) — or before it for left/up.
    public func splitting(_ id: Int, with newID: Int, _ dir: SplitDirection) -> SplitTree {
        switch self {
        case .leaf(let x) where x == id:
            return dir.forward ? .split(dir.axis, 0.5, self, .leaf(newID)) : .split(dir.axis, 0.5, .leaf(newID), self)
        case .leaf:
            return self
        case .split(let ax, let r, let a, let b):
            return .split(ax, r, a.splitting(id, with: newID, dir), b.splitting(id, with: newID, dir))
        }
    }

    /// The tree without `id`; its sibling takes the freed space. Nil when nothing is left.
    public func removing(_ id: Int) -> SplitTree? {
        switch self {
        case .leaf(let x): return x == id ? nil : self
        case .split(let ax, let r, let a, let b):
            let na = a.removing(id), nb = b.removing(id)
            switch (na, nb) {
            case (nil, nil): return nil
            case (nil, let x?), (let x?, nil): return x
            case (let x?, let y?): return .split(ax, r, x, y)
            }
        }
    }

    /// Each pane's frame inside `rect`, leaving `divider` points between sides.
    public func frames(in rect: CGRect, divider: CGFloat = 1) -> [Int: CGRect] {
        var out: [Int: CGRect] = [:]
        walk(rect, divider, path: []) { node, r, _ in
            if case .leaf(let id) = node { out[id] = r }
        }
        return out
    }

    /// Every split's divider rectangle, with the path that addresses that split.
    public func dividers(in rect: CGRect, divider: CGFloat = 1) -> [(path: [Bool], axis: SplitAxis, rect: CGRect, bounds: CGRect)] {
        var out: [(path: [Bool], axis: SplitAxis, rect: CGRect, bounds: CGRect)] = []
        walk(rect, divider, path: []) { node, r, path in
            if case .split(let ax, let ratio, _, _) = node {
                let (a, _) = Self.divide(r, ax, ratio, divider)
                let d = ax == .horizontal
                    ? CGRect(x: a.maxX, y: r.minY, width: divider, height: r.height)
                    : CGRect(x: r.minX, y: a.maxY, width: r.width, height: divider)
                out.append((path, ax, d, r))
            }
        }
        return out
    }

    private func walk(_ rect: CGRect, _ divider: CGFloat, path: [Bool], _ visit: (SplitTree, CGRect, [Bool]) -> Void) {
        visit(self, rect, path)
        if case .split(let ax, let ratio, let a, let b) = self {
            let (ra, rb) = Self.divide(rect, ax, ratio, divider)
            a.walk(ra, divider, path: path + [false], visit)
            b.walk(rb, divider, path: path + [true], visit)
        }
    }

    // Rects are in a flipped (top-left origin) space, like the container view.
    static func divide(_ r: CGRect, _ ax: SplitAxis, _ ratio: CGFloat, _ divider: CGFloat) -> (CGRect, CGRect) {
        if ax == .horizontal {
            let w = ((r.width - divider) * ratio).rounded()
            return (CGRect(x: r.minX, y: r.minY, width: w, height: r.height),
                    CGRect(x: r.minX + w + divider, y: r.minY, width: r.width - w - divider, height: r.height))
        }
        let h = ((r.height - divider) * ratio).rounded()
        return (CGRect(x: r.minX, y: r.minY, width: r.width, height: h),
                CGRect(x: r.minX, y: r.minY + h + divider, width: r.width, height: r.height - h - divider))
    }

    /// The pane next to `id` in a direction: the closest one that overlaps it across the axis.
    public func neighbor(of id: Int, _ dir: SplitDirection, in rect: CGRect) -> Int? {
        let fs = frames(in: rect, divider: 0)
        guard let f = fs[id] else { return nil }
        var best: (id: Int, dist: CGFloat, off: CGFloat)?
        // Leaf order makes ties (two panes equally close) resolve to the first one.
        for other in leaves where other != id {
            let g = fs[other]!
            let dist: CGFloat, overlaps: Bool, off: CGFloat
            switch dir {
            case .right: dist = g.minX - f.maxX; overlaps = g.maxY > f.minY && g.minY < f.maxY; off = abs(g.midY - f.midY)
            case .left: dist = f.minX - g.maxX; overlaps = g.maxY > f.minY && g.minY < f.maxY; off = abs(g.midY - f.midY)
            case .down: dist = g.minY - f.maxY; overlaps = g.maxX > f.minX && g.minX < f.maxX; off = abs(g.midX - f.midX)
            case .up: dist = f.minY - g.maxY; overlaps = g.maxX > f.minX && g.minX < f.maxX; off = abs(g.midX - f.midX)
            }
            guard dist >= -0.5, overlaps else { continue }
            if best == nil || dist < best!.dist - 0.5 || (abs(dist - best!.dist) <= 0.5 && off < best!.off) {
                best = (other, dist, off)
            }
        }
        return best?.id
    }

    /// The pane before or after `id`, wrapping around, like Ghostty's goto_split previous/next.
    public func cycle(from id: Int, by step: Int) -> Int? {
        let l = leaves
        guard let i = l.firstIndex(of: id), !l.isEmpty else { return nil }
        return l[((i + step) % l.count + l.count) % l.count]
    }

    /// Moves the divider of the closest split around `id` along `dir`'s axis by `delta` (0..1 of that split).
    public func resizing(_ id: Int, _ dir: SplitDirection, by delta: CGFloat) -> SplitTree {
        guard let path = closestSplit(to: id, axis: dir.axis) else { return self }
        return adjusting(path) { $0 + (dir.forward ? delta : -delta) }
    }

    private func closestSplit(to id: Int, axis: SplitAxis, path: [Bool] = []) -> [Bool]? {
        guard case .split(let ax, _, let a, let b) = self else { return nil }
        if a.contains(id) { return a.closestSplit(to: id, axis: axis, path: path + [false]) ?? (ax == axis ? path : nil) }
        if b.contains(id) { return b.closestSplit(to: id, axis: axis, path: path + [true]) ?? (ax == axis ? path : nil) }
        return nil
    }

    public func settingRatio(at path: [Bool], _ ratio: CGFloat) -> SplitTree { adjusting(path) { _ in ratio } }

    private func adjusting(_ path: [Bool], _ f: (CGFloat) -> CGFloat) -> SplitTree {
        guard case .split(let ax, let r, let a, let b) = self else { return self }
        guard let head = path.first else { return .split(ax, min(0.9, max(0.1, f(r))), a, b) }
        let rest = Array(path.dropFirst())
        return head ? .split(ax, r, a, b.adjusting(rest, f)) : .split(ax, r, a.adjusting(rest, f), b)
    }

    /// Gives every pane an equal share along each axis, like Ghostty's equalize_splits.
    public func equalized() -> SplitTree {
        guard case .split(let ax, _, let a, let b) = self else { return self }
        let wa = a.weight(ax), wb = b.weight(ax)
        return .split(ax, CGFloat(wa) / CGFloat(wa + wb), a.equalized(), b.equalized())
    }

    private func weight(_ axis: SplitAxis) -> Int {
        guard case .split(let ax, _, let a, let b) = self else { return 1 }
        return ax == axis ? a.weight(axis) + b.weight(axis) : max(a.weight(axis), b.weight(axis))
    }
}
