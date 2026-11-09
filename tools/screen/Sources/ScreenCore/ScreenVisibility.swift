public enum ScreenVisibility {
    public static func shouldShow(devicePlugged: Bool, userHidden: Bool) -> Bool {
        !devicePlugged && !userHidden
    }
}

public enum RevealStep: Equatable, Sendable {
    case none, start, cancel
}

public enum Reveal {
    public static let edge = 2.0
    public static let peek = 10.0
    public static let band = peek + 4
    public static let delay = 1.2
    public static let retract = 0.08
    public static let peekHz = 60
    public static let travel = 4 + RowLayout.maxTile
    public static let peekSlide = (4 + RowLayout.tile - peek) / travel
    public static let edgeSlide = (4 + RowLayout.tile) / travel

    public static func step(distanceFromTop: Double, peeking: Bool) -> RevealStep {
        if peeking { return distanceFromTop > band ? .cancel : .none }
        return distanceFromTop <= edge ? .start : .none
    }
}
