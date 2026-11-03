#import <AppKit/AppKit.h>
#include "unread_darwin.h"

static NSString *str(AXUIElementRef e, CFStringRef attr) {
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(e, attr, &v) != kAXErrorSuccess || !v) return nil;
    if (CFGetTypeID(v) != CFStringGetTypeID()) {
        CFRelease(v);
        return nil;
    }
    return (__bridge_transfer NSString *)v;
}

static NSArray *kids(AXUIElementRef e) {
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(e, kAXChildrenAttribute, &v) != kAXErrorSuccess || !v) return @[];
    return (__bridge_transfer NSArray *)v;
}

static AXUIElementRef app(NSString *bundle) {
    NSRunningApplication *a = [NSRunningApplication runningApplicationsWithBundleIdentifier:bundle].firstObject;
    return a ? AXUIElementCreateApplication(a.processIdentifier) : NULL;
}

static NSString *clean(NSString *s) {
    return [[s componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet] componentsJoinedByString:@" "];
}

static void banners(AXUIElementRef e, int depth, NSMutableString *out) {
    if (depth > 8) return;
    for (id k in kids(e)) {
        AXUIElementRef c = (__bridge AXUIElementRef)k;
        NSString *sub = str(c, kAXSubroleAttribute);
        if ([sub isEqualToString:@"AXNotificationCenterBanner"] || [sub isEqualToString:@"AXNotificationCenterAlert"]) {
            NSString *title = nil;
            for (id t in kids(c))
                if ([str((__bridge AXUIElementRef)t, kAXIdentifierAttribute) isEqualToString:@"title"])
                    title = str((__bridge AXUIElementRef)t, kAXValueAttribute);
            NSString *ident = str(c, kAXIdentifierAttribute);
            if (title && ident) [out appendFormat:@"banner\t%@\t%@\n", ident, clean(title)];
            continue;
        }
        banners(c, depth + 1, out);
    }
}

char *unread_scan(void) {
    NSMutableString *out = [NSMutableString string];
    @autoreleasepool {
        AXUIElementRef dock = app(@"com.apple.dock");
        if (dock) {
            for (id l in kids(dock))
                for (id k in kids((__bridge AXUIElementRef)l)) {
                    AXUIElementRef item = (__bridge AXUIElementRef)k;
                    NSString *badge = str(item, CFSTR("AXStatusLabel"));
                    NSString *title = str(item, kAXTitleAttribute);
                    if (badge && title) [out appendFormat:@"badge\t%@\t%@\n", clean(title), clean(badge)];
                }
            CFRelease(dock);
        }
        AXUIElementRef nc = app(@"com.apple.notificationcenterui");
        if (nc) {
            banners(nc, 0, out);
            CFRelease(nc);
        }
    }
    return strdup(out.UTF8String);
}
