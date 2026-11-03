#import <CoreWLAN/CoreWLAN.h>
#include "wifi_darwin.h"

static CWInterface *wifi_iface(void) {
    return [[CWWiFiClient sharedWiFiClient] interface];
}

int wifi_power(void) {
    @autoreleasepool {
        CWInterface *i = wifi_iface();
        return i ? i.powerOn : -1;
    }
}

int wifi_set_power(int on) {
    @autoreleasepool {
        CWInterface *i = wifi_iface();
        return i && [i setPower:on error:nil];
    }
}

int wifi_visible(const char *ssid) {
    @autoreleasepool {
        NSError *err = nil;
        NSSet *found = [wifi_iface() scanForNetworksWithName:[NSString stringWithUTF8String:ssid] error:&err];
        if (err) return err.code == EBUSY ? -2 : -1;
        return found.count > 0;
    }
}
