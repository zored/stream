#import <CoreWLAN/CoreWLAN.h>
#import <dlfcn.h>
#import <objc/message.h>
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

// Private CoreWiFi: names are not redacted without Location, airportd supplies the saved password.
static const NSTimeInterval wifi_hotspot_recent = 30 * 24 * 3600;

static id cwf_open(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        dlopen("/System/Library/PrivateFrameworks/CoreWiFi.framework/CoreWiFi", RTLD_NOW);
    });
    Class c = NSClassFromString(@"CWFInterface");
    if (!c || !NSClassFromString(@"CWFScanParameters") || !NSClassFromString(@"CWFAssocParameters")) return nil;
    id i = [[c alloc] init];
    ((void (*)(id, SEL))objc_msgSend)(i, sel_registerName("activate"));
    return i;
}

static void cwf_close(id i) {
    ((void (*)(id, SEL))objc_msgSend)(i, sel_registerName("invalidate"));
}

static NSArray *cwf_scan(id i, NSString *ssid, BOOL cache, NSError **err) {
    id p = [[NSClassFromString(@"CWFScanParameters") alloc] init];
    ((void (*)(id, SEL, BOOL))objc_msgSend)(p, sel_registerName("setIncludeMatchingKnownNetworkProfiles:"), YES);
    if (cache) ((void (*)(id, SEL, BOOL))objc_msgSend)(p, sel_registerName("setCacheOnly:"), YES);
    else if (ssid) ((void (*)(id, SEL, id))objc_msgSend)(p, sel_registerName("setSSID:"), ssid);
    return ((id (*)(id, SEL, id, NSError **))objc_msgSend)(i, sel_registerName("performScanWithParameters:error:"), p, err);
}

static NSArray *cwf_known(id i) {
    return ((id (*)(id, SEL, id))objc_msgSend)(i, sel_registerName("knownNetworkProfilesWithProperties:"), nil);
}

static char *cwf_error(NSString *what, NSError *e) {
    return strdup([NSString stringWithFormat:@"%@: %@", what, e ? e.description : @"failed"].UTF8String);
}

int wifi_ssid(char **ssid) {
    @autoreleasepool {
        *ssid = NULL;
        id i = cwf_open();
        if (!i) return 0;
        NSString *name = [i valueForKey:@"networkName"];
        cwf_close(i);
        if (name.length) *ssid = strdup(name.UTF8String);
        return 1;
    }
}

char *wifi_list(int fresh, char **err) {
    @autoreleasepool {
        *err = NULL;
        id i = cwf_open();
        if (!i) {
            *err = strdup("CoreWiFi: no CWFInterface");
            return NULL;
        }
        NSError *e = nil;
        NSArray *found = cwf_scan(i, nil, !fresh, &e);
        NSArray *known = cwf_known(i);
        cwf_close(i);
        if (!found) {
            *err = cwf_error(@"CoreWiFi scan", e);
            return NULL;
        }
        NSMutableSet *hotspots = [NSMutableSet set], *nets = [NSMutableSet set];
        for (id n in found) {
            id profile = [n valueForKey:@"matchingKnownNetworkProfile"];
            NSString *name = [n valueForKey:@"networkName"];
            if (!profile || !name.length) continue;
            [[[profile valueForKey:@"isPersonalHotspot"] boolValue] ? hotspots : nets addObject:name];
        }
        for (id profile in known) {
            NSString *name = [profile valueForKey:@"networkName"];
            NSDate *joined = [profile valueForKey:@"lastJoinedAt"];
            if (name.length && joined && [[profile valueForKey:@"isPersonalHotspot"] boolValue] && -joined.timeIntervalSinceNow < wifi_hotspot_recent)
                [hotspots addObject:name];
        }
        SEL order = @selector(localizedCaseInsensitiveCompare:);
        NSArray *all = [[hotspots.allObjects sortedArrayUsingSelector:order] arrayByAddingObjectsFromArray:[nets.allObjects sortedArrayUsingSelector:order]];
        return strdup([all componentsJoinedByString:@"\n"].UTF8String);
    }
}

char *wifi_join(const char *ssid) {
    @autoreleasepool {
        NSString *name = @(ssid);
        id i = cwf_open();
        if (!i) return strdup("CoreWiFi: no CWFInterface");
        id profile = nil;
        for (id p in cwf_known(i)) {
            if ([[p valueForKey:@"networkName"] isEqual:name]) profile = p;
        }
        if (!profile) {
            cwf_close(i);
            return strdup("CoreWiFi: not a known network");
        }
        char *failure = NULL;
        int attempts = [[profile valueForKey:@"isPersonalHotspot"] boolValue] ? 2 : 4;
        for (int attempt = 0; attempt < attempts; attempt++) {
            NSError *e = nil;
            id best = nil;
            for (id n in cwf_scan(i, name, attempt == 0, &e)) {
                if ([[n valueForKey:@"networkName"] isEqual:name] && (!best || [[n valueForKey:@"RSSI"] integerValue] > [[best valueForKey:@"RSSI"] integerValue])) best = n;
            }
            free(failure);
            if (!best) {
                failure = e ? cwf_error(@"CoreWiFi scan", e) : strdup("CoreWiFi: not in range");
                continue;
            }
            id p = [[NSClassFromString(@"CWFAssocParameters") alloc] init];
            ((void (*)(id, SEL, id))objc_msgSend)(p, sel_registerName("setScanResult:"), best);
            ((void (*)(id, SEL, id))objc_msgSend)(p, sel_registerName("setKnownNetworkProfile:"), profile);
            e = nil;
            if (((BOOL (*)(id, SEL, id, NSError **))objc_msgSend)(i, sel_registerName("associateWithParameters:error:"), p, &e)) {
                cwf_close(i);
                return NULL;
            }
            failure = cwf_error(@"CoreWiFi associate", e);
        }
        cwf_close(i);
        return failure;
    }
}
