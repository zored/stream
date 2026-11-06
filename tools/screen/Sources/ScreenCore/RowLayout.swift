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

    public static func frames(cursorX: Double?, exclusion: ClosedRange<Double>? = nil, availableWidth: Double? = nil) -> [ElementFrame] {
        if let exclusion { return splitFrames(cursorX: cursorX, exclusion: exclusion, availableWidth: availableWidth) }
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

    private static func splitFrames(cursorX: Double?, exclusion: ClosedRange<Double>, availableWidth: Double?) -> [ElementFrame] {
        let elements = Layout.elements
        let base = elements.map(baseSize)
        let anchors = [exclusion.lowerBound - 8, exclusion.upperBound + 8]
        let groups = [0..<7, 7..<14]
        var baseScales = Array(repeating: 1.0, count: 14)
        var gapScales = [1.0, 1.0]
        var room = [Double.infinity, Double.infinity]
        for wing in groups.indices {
            let group = groups[wing]
            let span = group.reduce(0.0) { $0 + base[$1].w } + group.dropLast().reduce(0.0) { $0 + gapAfter($1) }
            if let availableWidth {
                room[wing] = max(0, availableWidth / 2 - 8 + (wing == 0 ? anchors[wing] : -anchors[wing]))
                gapScales[wing] = min(1, room[wing] / span)
                for i in group { baseScales[i] = gapScales[wing] }
            }
        }
        func positions(_ scales: [Double]) -> [Double] {
            var xs = Array(repeating: 0.0, count: 14)
            for wing in groups.indices {
                let group = groups[wing]
                let span = group.reduce(0.0) { $0 + base[$1].w * scales[$1] }
                    + group.dropLast().reduce(0.0) { $0 + gapAfter($1) * gapScales[wing] }
                var x = wing == 0 ? anchors[wing] - span : anchors[wing]
                for i in group {
                    xs[i] = x
                    x += base[i].w * scales[i] + gapAfter(i) * gapScales[wing]
                }
            }
            return xs
        }
        let restingX = positions(baseScales)
        let cursor = cursorX.flatMap { exclusion.contains($0) ? nil : $0 }
        var scales = elements.indices.map { i -> Double in
            guard let cursor else { return baseScales[i] }
            let half = base[i].w * baseScales[i] / 2
            let center = restingX[i] + half
            let edge: Double = if case .strip = elements[i] { half } else { 0 }
            return baseScales[i] * Geometry.scale(distance: max(0, abs(cursor - center) - edge), small: tile, max: maxTile, range: range)
        }
        for wing in groups.indices {
            let group = groups[wing]
            let baseSpan = group.reduce(0.0) { $0 + base[$1].w * baseScales[$1] }
                + group.dropLast().reduce(0.0) { $0 + gapAfter($1) * gapScales[wing] }
            let excess = group.reduce(0.0) { $0 + base[$1].w * (scales[$1] - baseScales[$1]) }
            if excess > 0 {
                let fraction = min(1, max(0, room[wing] - baseSpan) / excess)
                for i in group { scales[i] = baseScales[i] + (scales[i] - baseScales[i]) * fraction }
            }
        }
        let xs = positions(scales)
        return elements.indices.map {
            ElementFrame(element: elements[$0], x: xs[$0], width: base[$0].w * scales[$0], height: base[$0].h * scales[$0], scale: scales[$0])
        }
    }

    public static func width(_ f: [ElementFrame]) -> Double {
        guard let a = f.first, let b = f.last else { return 0 }
        return 2 * max(abs(a.x), abs(b.x + b.width))
    }

    public static func height(_ f: [ElementFrame]) -> Double {
        f.map(\.height).max() ?? 0
    }

    public static func hit(_ f: [ElementFrame], x: Double) -> Element? {
        f.first { x >= $0.x && x < $0.x + $0.width }?.element
    }
}
