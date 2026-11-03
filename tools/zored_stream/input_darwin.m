#import <Foundation/Foundation.h>
#include <Carbon/Carbon.h>
#include "input_darwin.h"
#include "_cgo_export.h"

static NSArray *tis_layouts(void) {
    NSDictionary *filter = @{
        (__bridge NSString *)kTISPropertyInputSourceCategory: (__bridge NSString *)kTISCategoryKeyboardInputSource,
        (__bridge NSString *)kTISPropertyInputSourceIsSelectCapable: @YES,
        (__bridge NSString *)kTISPropertyInputSourceIsEnabled: @YES,
    };
    return CFBridgingRelease(TISCreateInputSourceList((__bridge CFDictionaryRef)filter, false));
}

static NSString *tis_id(TISInputSourceRef s) {
    return (__bridge NSString *)TISGetInputSourceProperty(s, kTISPropertyInputSourceID);
}

int tis_current(char *out, int size) {
    @autoreleasepool {
        TISInputSourceRef s = TISCopyCurrentKeyboardInputSource();
        if (!s) return -1;
        NSArray *langs = (__bridge NSArray *)TISGetInputSourceProperty(s, kTISPropertyInputSourceLanguages);
        NSString *lang = langs.count ? langs[0] : tis_id(s);
        int ok = [lang getCString:out maxLength:size encoding:NSUTF8StringEncoding];
        CFRelease(s);
        return ok ? 0 : -1;
    }
}

int tis_next(void) {
    @autoreleasepool {
        NSArray *list = tis_layouts();
        if (list.count < 2) return -1;
        TISInputSourceRef cur = TISCopyCurrentKeyboardInputSource();
        NSString *curID = cur ? tis_id(cur) : nil;
        if (cur) CFRelease(cur);
        NSUInteger i = 0;
        for (; i < list.count; i++) {
            if ([tis_id((__bridge TISInputSourceRef)list[i]) isEqualToString:curID]) break;
        }
        TISInputSourceRef next = (__bridge TISInputSourceRef)list[(i + 1) % list.count];
        return TISSelectInputSource(next);
    }
}

static void tis_changed(CFNotificationCenterRef c, void *o, CFNotificationName n, const void *obj, CFDictionaryRef info) {
    goInputChanged();
}

void tis_watch(void) {
    CFNotificationCenterAddObserver(CFNotificationCenterGetDistributedCenter(), NULL, tis_changed,
        kTISNotifySelectedKeyboardInputSourceChanged, NULL, CFNotificationSuspensionBehaviorDeliverImmediately);
    CFRunLoopRun();
}
