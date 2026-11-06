import Foundation

public struct HoverMotion {
    private struct Spring {
        var value: Double
        var target: Double
        var velocity = 0.0

        var isSettled: Bool { abs(value - target) < 0.01 && abs(velocity) < 0.5 }

        mutating func advance(seconds: Double) {
            let displacement = value - target
            let decay = exp(-70 * seconds)
            let coefficient = velocity + 70 * displacement
            value = target + (displacement + coefficient * seconds) * decay
            velocity = (velocity - 70 * coefficient * seconds) * decay
        }
    }

    public private(set) var frames: [ElementFrame]
    private var target: [ElementFrame]
    private var springs: [[Spring]]
    public private(set) var isSettled = true

    public init(frames: [ElementFrame]) {
        self.frames = frames
        target = frames
        springs = frames.map { frame in
            [frame.x, frame.width, frame.height, frame.scale].map { Spring(value: $0, target: $0) }
        }
    }

    public mutating func retarget(_ target: [ElementFrame]) {
        precondition(frames.map(\.element) == target.map(\.element), "hover requires matching elements")
        self.target = target
        for i in target.indices {
            let values = [target[i].x, target[i].width, target[i].height, target[i].scale]
            for j in values.indices { springs[i][j].target = values[j] }
        }
        isSettled = frames == target && springs.allSatisfy { $0.allSatisfy { $0.velocity == 0 } }
    }

    public mutating func advance(seconds: Double) {
        guard !isSettled, seconds > 0 else { return }
        precondition(seconds.isFinite, "hover elapsed time must be finite")
        for i in springs.indices {
            for j in springs[i].indices { springs[i][j].advance(seconds: seconds) }
        }
        isSettled = springs.allSatisfy { $0.allSatisfy(\.isSettled) }
        if isSettled {
            frames = target
            for i in springs.indices {
                for j in springs[i].indices {
                    springs[i][j].value = springs[i][j].target
                    springs[i][j].velocity = 0
                }
            }
        } else {
            frames = target.indices.map { i in
                ElementFrame(element: target[i].element, x: springs[i][0].value, width: springs[i][1].value,
                             height: springs[i][2].value, scale: springs[i][3].value)
            }
        }
    }
}
