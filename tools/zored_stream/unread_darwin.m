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

char *unread_badges(void) {
    NSMutableString *out = [NSMutableString string];
    @autoreleasepool {
        AXUIElementRef dock = app(@"com.apple.dock");
        if (dock) {
            for (id l in kids(dock))
                for (id k in kids((__bridge AXUIElementRef)l)) {
                    AXUIElementRef item = (__bridge AXUIElementRef)k;
                    NSString *badge = str(item, CFSTR("AXStatusLabel"));
                    NSString *title = str(item, kAXTitleAttribute);
                    if (badge && title) [out appendFormat:@"%@\t%@\n", clean(title), clean(badge)];
                }
            CFRelease(dock);
        }
    }
    return strdup(out.UTF8String);
}
