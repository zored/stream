public struct ElementFrame: Equatable, Sendable {
    public let element: Element
    public let x: Double
    public let width: Double
    public let height: Double
    public let scale: Double
}

public enum RowLayout {
    public static let tile = 40.0
    public static let maxTile = 80.0
    public static let strip = (w: 215.0, h: 40.0)
    public static let range = 200.0
    public static let gap = 3.0
    public static let groupGap = 6.0

    static func baseSize(_ e: Element) -> (w: Double, h: Double) {
        if case .strip = e { return strip }
        return (tile, tile)
    }

    static func gapAfter(_ i: Int) -> Double {
        i == 0 || i % 3 == 0 ? gap + groupGap : gap
    }

    static func place(_ sizes: [Double]) -> [Double] {
        var xs: [Double] = []
        var x = 0.0
        for (i, w) in sizes.enumerated() {
            xs.append(x)
            x += w + (i < sizes.count - 1 ? gapAfter(i) : 0)
        }
        return xs.map { $0 - x / 2 }
    }

    public static func frames(cursorX: Double?) -> [ElementFrame] {
        let els = Layout.elements
        let base = els.map(baseSize)
        let baseX = place(base.map(\.w))
        let scales = els.indices.map { i -> Double in
            guard let c = cursorX else { return 1 }
            let half = base[i].w / 2
            let center = baseX[i] + half
            let edge: Double = if case .strip = els[i] { half } else { 0 }
            let d = max(0, abs(c - center) - edge)
            return Geometry.scale(distance: d, small: tile, max: maxTile, range: range)
        }
        let xs = place(els.indices.map { base[$0].w * scales[$0] })
        return els.indices.map {
            ElementFrame(element: els[$0], x: xs[$0], width: base[$0].w * scales[$0], height: base[$0].h * scales[$0], scale: scales[$0])
        }
    }

    public static func width(_ f: [ElementFrame]) -> Double {
        guard let a = f.first, let b = f.last else { return 0 }
        return b.x + b.width - a.x
    }

    public static func height(_ f: [ElementFrame]) -> Double {
        f.map(\.height).max() ?? 0
    }

    public static func hit(_ f: [ElementFrame], x: Double) -> Element? {
        f.first { x >= $0.x && x < $0.x + $0.width }?.element
    }
}
