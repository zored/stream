#import <AppKit/AppKit.h>
#include "mediakey_darwin.h"

static int post(int key, int down) {
    int state = down ? 0xa : 0xb;
    NSEvent *event = [NSEvent otherEventWithType:NSEventTypeSystemDefined
                                        location:NSZeroPoint
                                   modifierFlags:(NSEventModifierFlags)(state << 8)
                                       timestamp:0
                                    windowNumber:0
                                         context:nil
                                         subtype:8
                                           data1:(key << 16) | (state << 8)
                                           data2:-1];
    CGEventRef cg = event.CGEvent;
    if (cg == NULL) return 0;
    CGEventPost(kCGHIDEventTap, cg);
    return 1;
}

int mediakey_post(int key) {
    @autoreleasepool {
        return post(key, 1) && post(key, 0);
    }
}
