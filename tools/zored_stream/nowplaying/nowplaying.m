#import <Foundation/Foundation.h>
#include <dlfcn.h>
#include <unistd.h>

typedef void (*GetNowPlayingInfo)(dispatch_queue_t, void (^)(CFDictionaryRef));
typedef void (*GetIsPlaying)(dispatch_queue_t, void (^)(Boolean));
typedef void (*GetNowPlayingClient)(dispatch_queue_t, void (^)(id));
typedef CFStringRef (*GetClientBundleIdentifier)(id);

static const double PollSeconds = 0.5;

static GetNowPlayingInfo getNowPlayingInfo;
static GetIsPlaying getIsPlaying;
static GetNowPlayingClient getNowPlayingClient;
static GetClientBundleIdentifier getClientBundleIdentifier;
static NSMutableDictionary *state;
static NSData *last;
static dispatch_source_t pollTimer;
static dispatch_source_t parentExit;

static void die(const char *message) {
    fprintf(stderr, "nowplaying: %s\n", message);
    exit(1);
}

static NSString *stringValue(CFDictionaryRef info, NSString *key) {
    id value = info ? ((__bridge NSDictionary *)info)[key] : nil;
    return [value isKindOfClass:NSString.class] ? value : @"";
}

static void emit(void) {
    NSError *error;
    NSData *json = [NSJSONSerialization dataWithJSONObject:state options:NSJSONWritingSortedKeys error:&error];
    if (!json) die(error.localizedDescription.UTF8String);
    if ([json isEqualToData:last]) return;
    last = json;
    fwrite(json.bytes, 1, json.length, stdout);
    fputc('\n', stdout);
    if (fflush(stdout) != 0) die("stdout closed");
}

static void poll(void) {
    dispatch_queue_t main = dispatch_get_main_queue();
    dispatch_group_t group = dispatch_group_create();
    dispatch_group_enter(group);
    getNowPlayingInfo(main, ^(CFDictionaryRef info) {
        state[@"title"] = stringValue(info, @"kMRMediaRemoteNowPlayingInfoTitle");
        state[@"artist"] = stringValue(info, @"kMRMediaRemoteNowPlayingInfoArtist");
        dispatch_group_leave(group);
    });
    dispatch_group_enter(group);
    getIsPlaying(main, ^(Boolean playing) {
        state[@"playing"] = playing ? @YES : @NO;
        dispatch_group_leave(group);
    });
    dispatch_group_enter(group);
    getNowPlayingClient(main, ^(id client) {
        NSString *bundle = client ? (__bridge NSString *)getClientBundleIdentifier(client) : nil;
        state[@"app"] = bundle ?: @"";
        dispatch_group_leave(group);
    });
    dispatch_group_notify(group, main, ^{ emit(); });
}

void nowplaying_main(void *perl, void *cv) {
    void *mediaRemote = dlopen("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote", RTLD_NOW);
    if (!mediaRemote) die("MediaRemote unavailable");
    getNowPlayingInfo = (GetNowPlayingInfo)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingInfo");
    getIsPlaying = (GetIsPlaying)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingApplicationIsPlaying");
    getNowPlayingClient = (GetNowPlayingClient)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingClient");
    getClientBundleIdentifier = (GetClientBundleIdentifier)dlsym(mediaRemote, "MRNowPlayingClientGetBundleIdentifier");
    if (!getNowPlayingInfo || !getIsPlaying || !getNowPlayingClient || !getClientBundleIdentifier) die("MediaRemote symbols missing");

    state = [NSMutableDictionary dictionaryWithDictionary:@{@"title": @"", @"artist": @"", @"playing": @NO, @"app": @""}];
    dispatch_queue_t main = dispatch_get_main_queue();
    parentExit = dispatch_source_create(DISPATCH_SOURCE_TYPE_PROC, getppid(), DISPATCH_PROC_EXIT, main);
    dispatch_source_set_event_handler(parentExit, ^{ exit(0); });
    dispatch_resume(parentExit);
    pollTimer = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0, main);
    dispatch_source_set_timer(pollTimer, DISPATCH_TIME_NOW, (uint64_t)(PollSeconds * NSEC_PER_SEC), (uint64_t)(PollSeconds * NSEC_PER_SEC / 10));
    dispatch_source_set_event_handler(pollTimer, ^{ poll(); });
    dispatch_resume(pollTimer);
    dispatch_main();
}
