public enum Motion {
    public static let hoverDuration = 0.16
    public static let visibilityDuration = 0.18

    public static func value(from: Double, to: Double, progress: Double) -> Double {
        if progress <= 0 { return from }
        if progress >= 1 { return to }
        let eased = progress * progress * (3 - 2 * progress)
        return from + (to - from) * eased
    }

    public static func frames(from: [ElementFrame], to: [ElementFrame], progress: Double) -> [ElementFrame] {
        precondition(from.map(\.element) == to.map(\.element), "motion requires matching elements")
        if progress <= 0 { return from }
        if progress >= 1 { return to }
        return zip(from, to).map { start, end in
            ElementFrame(
                element: end.element,
                x: value(from: start.x, to: end.x, progress: progress),
                width: value(from: start.width, to: end.width, progress: progress),
                height: value(from: start.height, to: end.height, progress: progress),
                scale: value(from: start.scale, to: end.scale, progress: progress)
            )
        }
    }
}
