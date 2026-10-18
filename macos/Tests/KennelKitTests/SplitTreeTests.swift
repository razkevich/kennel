import CoreGraphics
import Testing
@testable import KennelKit

let box = CGRect(x: 0, y: 0, width: 1000, height: 600)

@Test func splitRightThenDownMakesAnL() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right).splitting(2, with: 3, .down)
    #expect(t.leaves == [1, 2, 3])
    let f = t.frames(in: box, divider: 0)
    #expect(f[1] == CGRect(x: 0, y: 0, width: 500, height: 600))
    #expect(f[2] == CGRect(x: 500, y: 0, width: 500, height: 300))
    #expect(f[3] == CGRect(x: 500, y: 300, width: 500, height: 300))
}

@Test func splittingLeftOrUpPutsTheNewPaneFirst() {
    #expect(SplitTree.leaf(1).splitting(1, with: 2, .left).leaves == [2, 1])
    #expect(SplitTree.leaf(1).splitting(1, with: 2, .up).leaves == [2, 1])
}

@Test func closingAPaneGivesItsSpaceToTheSibling() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right).splitting(2, with: 3, .down)
    let r = t.removing(2)
    #expect(r == .split(.horizontal, 0.5, .leaf(1), .leaf(3)))
    #expect(r?.removing(1) == .leaf(3))
    #expect(SplitTree.leaf(3).removing(3) == nil)
}

@Test func neighborsFollowGeometry() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right).splitting(2, with: 3, .down)
    #expect(t.neighbor(of: 1, .right, in: box) == 2)
    #expect(t.neighbor(of: 3, .left, in: box) == 1)
    #expect(t.neighbor(of: 2, .down, in: box) == 3)
    #expect(t.neighbor(of: 3, .up, in: box) == 2)
    #expect(t.neighbor(of: 1, .left, in: box) == nil)
    #expect(t.cycle(from: 3, by: 1) == 1)
    #expect(t.cycle(from: 1, by: -1) == 3)
}

@Test func resizeMovesTheClosestMatchingDivider() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right).splitting(2, with: 3, .down)
    // From pane 3, left/right moves the outer (horizontal) divider; up/down the inner one.
    #expect(t.resizing(3, .left, by: 0.1) == .split(.horizontal, 0.4, .leaf(1), .split(.vertical, 0.5, .leaf(2), .leaf(3))))
    #expect(t.resizing(3, .down, by: 0.1) == .split(.horizontal, 0.5, .leaf(1), .split(.vertical, 0.6, .leaf(2), .leaf(3))))
    #expect(t.resizing(1, .up, by: 0.1) == t)
    #expect(t.resizing(1, .right, by: 0.9) == t.settingRatio(at: [], 0.9))
}

@Test func equalizeSharesSpaceByPaneCount() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right).splitting(2, with: 3, .right).resizing(1, .right, by: 0.3)
    let e = t.equalized()
    let f = e.frames(in: CGRect(x: 0, y: 0, width: 900, height: 100), divider: 0)
    #expect(f[1]!.width == 300 && f[2]!.width == 300 && f[3]!.width == 300)
}

@Test func dividersSitBetweenSides() {
    let t = SplitTree.leaf(1).splitting(1, with: 2, .right)
    let d = t.dividers(in: box, divider: 2)
    #expect(d.count == 1)
    #expect(d[0].rect == CGRect(x: 499, y: 0, width: 2, height: 600))
    #expect(d[0].path == [])
}
