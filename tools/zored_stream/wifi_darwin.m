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


void wifi_disassociate(void) {
    @autoreleasepool {
        [wifi_iface() disassociate];
    }
}
