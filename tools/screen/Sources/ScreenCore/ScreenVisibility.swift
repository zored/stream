public enum ScreenVisibility {
    public static func shouldShow(devicePlugged: Bool, userHidden: Bool) -> Bool {
        !userHidden
    }
}
