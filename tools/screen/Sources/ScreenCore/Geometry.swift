import Foundation

public enum Geometry {
    public static let stripHeight = 270
    public static let swipeMin = 40.0
    public static let swipeMaxMs = 400.0

    public static func deviceY(side: Side, x: Double, width: Double) -> Int {
        let f = min(max(x / width, 0), 1)
        let y = Double(stripHeight) * (side == .left ? f : 1 - f)
        return Int(y.rounded())
    }

    public static func scale(distance: Double, small: Double, max: Double, range: Double) -> Double {
        let d = abs(distance)
        guard d < range else { return 1 }
        return 1 + (max / small - 1) * (1 + cos(Double.pi * d / range)) / 2
    }

    public static func swipe(dx: Double, dy: Double, ms: Double) -> String? {
        guard ms <= swipeMaxMs else { return nil }
        if abs(dx) >= abs(dy) {
            guard abs(dx) >= swipeMin else { return nil }
            return dx > 0 ? "r" : "l"
        }
        guard abs(dy) >= swipeMin else { return nil }
        return dy > 0 ? "d" : "u"
    }
}
