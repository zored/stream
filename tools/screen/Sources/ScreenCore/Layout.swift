public struct GridID: Hashable, Sendable, CustomStringConvertible {
    public let row: Int
    public let col: Int

    public init(_ row: Int, _ col: Int) {
        self.row = row
        self.col = col
    }

    public init?(id: String) {
        let parts = id.split(separator: "/")
        guard parts.count == 2, let r = Int(parts[0]), let c = Int(parts[1]) else { return nil }
        self.init(r, c)
    }

    public var id: String { "\(row)/\(col)" }
    public var description: String { id }
}

public enum Side: Sendable {
    case left, right
}

public enum Element: Hashable, Sendable {
    case tile(n: Int, grid: GridID)
    case strip(Side)
}

public enum Layout {
    public static let rows = 4
    public static let cols = 8

    public static let tiles: [GridID] = (2...5).flatMap { c in (0...2).map { r in GridID(r, c) } }

    public static let all: [GridID] = (0..<rows).flatMap { r in (0..<cols).map { c in GridID(r, c) } }

    public static let elements: [Element] =
        [.strip(.left)] + tiles.enumerated().map { .tile(n: $0.offset + 1, grid: $0.element) } + [.strip(.right)]

    public static func knob(forTile n: Int) -> GridID? {
        switch n {
        case 1...3: GridID(n - 1, 0)
        case 10...12: GridID(n - 10, 7)
        default: nil
        }
    }

    public static func button(forTile n: Int) -> GridID? {
        (1...8).contains(n) ? GridID(3, n - 1) : nil
    }

    public static func stripCells(_ side: Side) -> [GridID] {
        let c = side == .left ? 1 : 6
        return (0...2).map { GridID($0, c) }
    }

    public static func strip(of id: GridID) -> Side? {
        guard id.row <= 2 else { return nil }
        return id.col == 1 ? .left : id.col == 6 ? .right : nil
    }
}
