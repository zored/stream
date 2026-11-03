#import <AppKit/AppKit.h>
#include "windows_darwin.h"

extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *out);

static NSSet *ax_window_ids(int pid) {
    NSMutableSet *out = [NSMutableSet set];
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    CFTypeRef windows = NULL;
    if (AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, &windows) == kAXErrorSuccess && windows) {
        for (CFIndex i = 0; i < CFArrayGetCount(windows); i++) {
            CGWindowID wid = 0;
            if (_AXUIElementGetWindow((AXUIElementRef)CFArrayGetValueAtIndex(windows, i), &wid) == kAXErrorSuccess) [out addObject:@(wid)];
        }
        CFRelease(windows);
    }
    CFRelease(app);
    return out;
}

int windows_list(int *pids, unsigned int *ids, int max) {
    @autoreleasepool {
        NSMutableDictionary<NSNumber *, NSSet *> *byPid = [NSMutableDictionary dictionary];
        CFArrayRef info = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
        if (!info) return -1;
        int n = 0;
        for (NSDictionary *w in (__bridge NSArray *)info) {
            if (n >= max) break;
            if ([w[(id)kCGWindowLayer] intValue] != 0 || [w[(id)kCGWindowAlpha] doubleValue] <= 0) continue;
            NSNumber *pid = w[(id)kCGWindowOwnerPID];
            if (!byPid[pid]) byPid[pid] = ax_window_ids(pid.intValue);
            if (![byPid[pid] containsObject:w[(id)kCGWindowNumber]]) continue;
            pids[n] = pid.intValue;
            ids[n] = [w[(id)kCGWindowNumber] unsignedIntValue];
            n++;
        }
        CFRelease(info);
        return n;
    }
}

int windows_raise(int pid, unsigned int id) {
    @autoreleasepool {
        AXUIElementRef app = AXUIElementCreateApplication(pid);
        CFTypeRef windows = NULL;
        AXError error = AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, &windows);
        CFRelease(app);
        if (error != kAXErrorSuccess || !windows) return 1;
        int result = 2;
        for (CFIndex i = 0; i < CFArrayGetCount(windows); i++) {
            AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windows, i);
            CGWindowID wid = 0;
            if (_AXUIElementGetWindow(w, &wid) != kAXErrorSuccess || wid != id) continue;
            AXUIElementSetAttributeValue(w, kAXMainAttribute, kCFBooleanTrue);
            result = AXUIElementPerformAction(w, kAXRaiseAction) == kAXErrorSuccess ? 0 : 3;
            break;
        }
        CFRelease(windows);
        if (result) return result;
        NSRunningApplication *running = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
        return [running activateWithOptions:0] ? 0 : 4;
    }
}
