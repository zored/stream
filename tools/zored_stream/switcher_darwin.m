#import <AppKit/AppKit.h>
#include "switcher_darwin.h"

extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *window);
extern CGError _SLPSSetFrontProcessWithOptions(ProcessSerialNumber *psn, CGWindowID window, uint32_t mode);
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

enum { SLPS_USER_GENERATED = 0x200 };

static void add_app(NSMutableString *out, NSMutableSet *seen, int pid) {
    if (pid <= 0 || [seen containsObject:@(pid)]) return;
    [seen addObject:@(pid)];
    NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    if (!app || app.activationPolicy != NSApplicationActivationPolicyRegular) return;
    [out appendFormat:@"%d\t%@\n", pid, app.localizedName ?: @""];
}

static void add_owners(NSMutableString *out, NSMutableSet *seen, CGWindowListOption option) {
    NSArray *windows = CFBridgingRelease(CGWindowListCopyWindowInfo(option | kCGWindowListExcludeDesktopElements, kCGNullWindowID));
    for (NSDictionary *w in windows) {
        if ([w[(id)kCGWindowLayer] intValue] == 0) add_app(out, seen, [w[(id)kCGWindowOwnerPID] intValue]);
    }
}

char *sw_apps(int front) {
    @autoreleasepool {
        NSMutableString *out = [NSMutableString string];
        NSMutableSet *seen = [NSMutableSet set];
        add_app(out, seen, front);
        add_owners(out, seen, kCGWindowListOptionOnScreenOnly);
        add_owners(out, seen, kCGWindowListOptionAll);
        return strdup(out.UTF8String);
    }
}

void *sw_icon(int pid, double turn, int px, int *len) {
    @autoreleasepool {
        NSImage *icon = [NSRunningApplication runningApplicationWithProcessIdentifier:pid].icon;
        NSRect rect = NSMakeRect(0, 0, px, px);
        CGImageRef image = [icon CGImageForProposedRect:&rect context:nil hints:nil];
        if (!image) return NULL;
        CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
        CGContextRef ctx = CGBitmapContextCreate(NULL, px, px, 8, 0, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
        CGColorSpaceRelease(cs);
        CGContextTranslateCTM(ctx, px / 2.0, px / 2.0);
        CGContextRotateCTM(ctx, -turn * M_PI / 180);
        CGContextDrawImage(ctx, CGRectMake(-px / 2.0, -px / 2.0, px, px), image);
        CGImageRef turned = CGBitmapContextCreateImage(ctx);
        CGContextRelease(ctx);
        NSMutableData *data = [NSMutableData data];
        CGImageDestinationRef dst = CGImageDestinationCreateWithData((__bridge CFMutableDataRef)data, CFSTR("public.png"), 1, NULL);
        CGImageDestinationAddImage(dst, turned, NULL);
        BOOL ok = CGImageDestinationFinalize(dst);
        CFRelease(dst);
        CGImageRelease(turned);
        if (!ok) return NULL;
        *len = (int)data.length;
        void *bytes = malloc(data.length);
        memcpy(bytes, data.bytes, data.length);
        return bytes;
    }
}

static int kept_pid;
static CFMutableArrayRef kept;

static NSString *string_attr(AXUIElementRef element, CFStringRef attr) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, attr, &value) != kAXErrorSuccess || !value) return nil;
    id object = CFBridgingRelease(value);
    return [object isKindOfClass:NSString.class] ? object : nil;
}

char *sw_windows(int pid) {
    @autoreleasepool {
        if (!kept) kept = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
        CFArrayRemoveAllValues(kept);
        kept_pid = pid;
        AXUIElementRef app = AXUIElementCreateApplication(pid);
        AXUIElementSetMessagingTimeout(app, 0.5);
        CFTypeRef windows = NULL;
        AXError error = AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, &windows);
        CFRelease(app);
        if (error != kAXErrorSuccess || !windows) return NULL;
        NSMutableString *out = [NSMutableString string];
        for (id window in (NSArray *)CFBridgingRelease(windows)) {
            AXUIElementRef w = (__bridge AXUIElementRef)window;
            NSString *subrole = string_attr(w, kAXSubroleAttribute);
            if (![subrole isEqual:(id)kAXStandardWindowSubrole] && ![subrole isEqual:(id)kAXDialogSubrole]) continue;
            NSString *title = string_attr(w, kAXTitleAttribute) ?: @"";
            title = [[title componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet] componentsJoinedByString:@" "];
            CFArrayAppendValue(kept, w);
            [out appendFormat:@"%@\n", title];
        }
        return strdup(out.UTF8String);
    }
}

int sw_raise(int pid, int index) {
    @autoreleasepool {
        ProcessSerialNumber psn;
        if (GetProcessForPID(pid, &psn) != noErr) return -1;
        AXUIElementRef w = NULL;
        CGWindowID window = 0;
        if (index >= 0) {
            if (pid != kept_pid || index >= CFArrayGetCount(kept)) return -2;
            w = (AXUIElementRef)CFArrayGetValueAtIndex(kept, index);
            AXUIElementSetAttributeValue(w, kAXMinimizedAttribute, kCFBooleanFalse);
            if (_AXUIElementGetWindow(w, &window) != kAXErrorSuccess) return -3;
        }
        if (_SLPSSetFrontProcessWithOptions(&psn, window, SLPS_USER_GENERATED) != kCGErrorSuccess) return -4;
        return !w || AXUIElementPerformAction(w, kAXRaiseAction) == kAXErrorSuccess ? 0 : -5;
    }
}
