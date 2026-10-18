// Renders the kennel pup (same pixel grid as the web map) into an .iconset.
import AppKit

let dog = [
  "..DD......DD..", ".DFFD....DFFD.", ".DFFFFFFFFFFD.", "..FFFFFFFFFF..",
  "..FFEFFFFEFF..", "..FFFFNNFFFF..", "..FFFWWWWFFF..", "...FFFTTFFF...",
  "..FFFFFFFFFF..", ".FFFFFFFFFFFFR", ".FFFFFFFFFFFF.", ".FF.FF..FF.FF.",
]
func c(_ h: UInt32) -> NSColor { NSColor(srgbRed: CGFloat((h >> 16) & 0xff) / 255, green: CGFloat((h >> 8) & 0xff) / 255, blue: CGFloat(h & 0xff) / 255, alpha: 1) }
let pal: [Character: NSColor] = ["F": c(0xfab387), "D": c(0xb07850), "R": c(0xfab387), "E": c(0x1e1e2e), "N": c(0x3b2f2f), "W": c(0xfff4e0), "T": c(0xff8fa3)]

let out = CommandLine.arguments[1]
try? FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)
for (size, name) in [(16, "16x16"), (32, "16x16@2x"), (32, "32x32"), (64, "32x32@2x"), (128, "128x128"), (256, "128x128@2x"), (256, "256x256"), (512, "256x256@2x"), (512, "512x512"), (1024, "512x512@2x")] {
    let s = CGFloat(size)
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    NSGraphicsContext.current?.imageInterpolation = .none
    let bg = NSBezierPath(roundedRect: NSRect(x: s * 0.06, y: s * 0.06, width: s * 0.88, height: s * 0.88), xRadius: s * 0.2, yRadius: s * 0.2)
    c(0x262b52).setFill(); bg.fill()
    c(0xa6e3a1).withAlphaComponent(0.9).setFill()
    NSBezierPath(rect: NSRect(x: s * 0.06, y: s * 0.06, width: s * 0.88, height: s * 0.2)).fill()
    let px = (s * 0.62) / 14
    let ox = (s - px * 14) / 2, oy = s * 0.22
    for (r, row) in dog.enumerated() {
        for (col, ch) in row.enumerated() {
            guard let color = pal[ch] else { continue }
            color.setFill()
            NSBezierPath(rect: NSRect(x: ox + CGFloat(col) * px, y: oy + CGFloat(11 - r) * px, width: ceil(px), height: ceil(px))).fill()
        }
    }
    NSGraphicsContext.restoreGraphicsState()
    try rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: "\(out)/icon_\(name).png"))
}
