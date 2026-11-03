#import <CoreWLAN/CoreWLAN.h>
#import <CoreLocation/CoreLocation.h>
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

char *wifi_ssid(void) {
    @autoreleasepool {
        NSString *s = wifi_iface().ssid;
        return s ? strdup(s.UTF8String) : NULL;
    }
}

char *wifi_known(void) {
    @autoreleasepool {
        NSMutableString *out = [NSMutableString string];
        for (CWNetworkProfile *p in wifi_iface().configuration.networkProfiles) {
            if (p.ssid) [out appendFormat:@"%@\n", p.ssid];
        }
        return strdup(out.UTF8String);
    }
}

char *wifi_scan(void) {
    @autoreleasepool {
        NSError *err = nil;
        NSSet<CWNetwork *> *found = [wifi_iface() scanForNetworksWithName:nil error:&err];
        if (!found) return NULL;
        NSMutableString *out = [NSMutableString string];
        for (CWNetwork *n in found) {
            if (n.ssid) [out appendFormat:@"%@\t%ld\t%d\n", n.ssid, (long)n.rssiValue, ![n supportsSecurity:kCWSecurityNone]];
        }
        return strdup(out.UTF8String);
    }
}

static CLLocationManager *wifi_location;

void wifi_request_location(void) {
    wifi_location = [[CLLocationManager alloc] init];
    [wifi_location requestAlwaysAuthorization];
}
