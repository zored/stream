import Foundation

public enum GlassBitmap {
    public static func rgba(_ rgb: Data, width: Int, height: Int, preserveInset: Double? = nil) -> Data {
        precondition(width > 0 && height > 0 && rgb.count == width * height * 3)
        if let inset = preserveInset { precondition(inset.isFinite && inset >= 0 && inset < 0.5) }
        let insetX = preserveInset.map { Int((Double(width) * $0).rounded(.down)) }
        let insetY = preserveInset.map { Int((Double(height) * $0).rounded(.down)) }
        var rgba = Data(count: width * height * 4)
        rgb.withUnsafeBytes { source in
            rgba.withUnsafeMutableBytes { destination in
                let pixels = source.bindMemory(to: UInt8.self)
                let output = destination.bindMemory(to: UInt8.self)
                for y in 0..<height {
                    for x in 0..<width {
                        let index = y * width + x
                        let from = index * 3
                        let to = index * 4
                        let peak = Int(max(pixels[from], pixels[from + 1], pixels[from + 2]))
                        let preserve: Bool
                        if let insetX, let insetY {
                            preserve = x >= insetX && x < width - insetX && y >= insetY && y < height - insetY
                        } else { preserve = false }
                        let alpha = preserve ? 255 : min(255, max(0, ((peak - 8) * 255 + 8) / 16))
                        for channel in 0..<3 {
                            output[to + channel] = UInt8((Int(pixels[from + channel]) * alpha + 127) / 255)
                        }
                        output[to + 3] = UInt8(alpha)
                    }
                }
            }
        }
        return rgba
    }
}
