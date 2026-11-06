public struct HeldControls {
    private var controls: [Int: GridID] = [:]
    public init() {}
    public mutating func down(button: Int, id: GridID) -> Bool {
        guard controls[button] == nil else { return false }
        controls[button] = id
        return true
    }
    public mutating func up(button: Int) -> GridID? { controls.removeValue(forKey: button) }
    public mutating func releaseAll() -> [GridID] {
        let ids = Array(controls.values)
        controls.removeAll()
        return ids
    }
}

public struct WheelSteps {
    private var remainder = 0.0
    private var lastPrecise: Bool?
    public init() {}
    public mutating func consume(delta: Double, precise: Bool, inverted: Bool) -> Int {
        if lastPrecise != precise { remainder = 0; lastPrecise = precise }
        remainder += delta * (inverted ? -1 : 1) / (precise ? 10 : 1)
        let steps = Int(remainder.rounded(.towardZero))
        remainder -= Double(steps)
        return steps
    }
}
