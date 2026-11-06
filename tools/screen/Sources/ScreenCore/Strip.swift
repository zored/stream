import Foundation

public enum Strip {
    public static func compose(_ cells: [Data?], cellW: Int, cellH: Int, side: Side) -> Data {
        precondition(cellW > 0 && cellH > 0 && cells.count == 3)
        precondition(cells.allSatisfy { $0 == nil || $0!.count == cellW * cellH * 3 })
        let height = cellH * 3
        var output = Data(count: height * cellW * 3)
        for y in 0..<height {
            guard let cell = cells[y / cellH] else { continue }
            for x in 0..<cellW {
                let u = side == .left ? y : height - 1 - y
                let v = side == .left ? cellW - 1 - x : x
                let source = ((y % cellH) * cellW + x) * 3
                let dest = (v * height + u) * 3
                output.replaceSubrange(dest..<dest + 3, with: cell[source..<source + 3])
            }
        }
        return output
    }
}
