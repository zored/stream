#import <Foundation/Foundation.h>
#import <AudioToolbox/AudioServices.h>
#import <CoreAudio/CoreAudio.h>
#include <arpa/inet.h>
#include <dlfcn.h>
#include <netinet/in.h>
#include <sys/socket.h>

static NSString *const CompanionURL = @"http://127.0.0.1:8000";
static const uint16_t Port = 47113;
static const double VisibleSeconds = 1.0;
static const double AnimationHz = 30;
static const double Easing = 0.3;
static const double FadeIn = 0.6;
static const double FadeOut = 0.25;
static const double DragPercentPerPixel = 0.4;

typedef void (*GetNowPlayingInfo)(dispatch_queue_t, void (^)(CFDictionaryRef));
typedef void (*GetIsPlaying)(dispatch_queue_t, void (^)(Boolean));
typedef void (*GetNowPlayingClient)(dispatch_queue_t, void (^)(id));
typedef CFStringRef (*GetClientBundleIdentifier)(id);

static NSURLSession *session;
static NSMutableDictionary<NSString *, NSString *> *sent;
static GetNowPlayingInfo getNowPlayingInfo;
static GetIsPlaying getIsPlaying;
static GetNowPlayingClient getNowPlayingClient;
static GetClientBundleIdentifier getClientBundleIdentifier;

static int volume = -1;
static bool muted;
static double shown = -1;
static double alpha;
static dispatch_source_t animation;
static bool animating;
static double dragRemainder;
static double visibleUntil;
static double lastTick;
static int lastDirection;

static void die(NSString *message) {
    fprintf(stderr, "streamd: %s\n", message.UTF8String);
    exit(1);
}

static void warn(NSString *message) {
    fprintf(stderr, "streamd: %s\n", message.UTF8String);
}

static void setVariable(NSString *name, NSString *value) {
    if ([sent[name] isEqualToString:value]) return;
    sent[name] = value;
    NSString *url = [NSString stringWithFormat:@"%@/api/custom-variable/%@/value", CompanionURL, name];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:[NSURL URLWithString:url]];
    request.HTTPMethod = @"POST";
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    request.HTTPBody = [NSJSONSerialization dataWithJSONObject:value options:NSJSONWritingFragmentsAllowed error:nil];
    [[session dataTaskWithRequest:request completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        NSInteger status = [(NSHTTPURLResponse *)response statusCode];
        if (!error && status < 300) return;
        warn([NSString stringWithFormat:@"set %@ failed: %@ (HTTP %ld)", name, error.localizedDescription ?: @"", (long)status]);
        dispatch_async(dispatch_get_main_queue(), ^{ [sent removeObjectForKey:name]; });
    }] resume];
}

static AudioObjectID outputDevice(void) {
    AudioObjectID device = kAudioObjectUnknown;
    UInt32 size = sizeof device;
    AudioObjectPropertyAddress address = {kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    OSStatus status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address, 0, NULL, &size, &device);
    if (status != noErr) die([NSString stringWithFormat:@"no default output device (%d)", status]);
    return device;
}

static AudioObjectPropertyAddress volumeAddress = {kAudioHardwareServiceDeviceProperty_VirtualMainVolume, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};
static AudioObjectPropertyAddress muteAddress = {kAudioDevicePropertyMute, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};

static void readAudio(int *outVolume, bool *outMuted) {
    AudioObjectID device = outputDevice();
    Float32 scalar = 0;
    UInt32 size = sizeof scalar;
    OSStatus status = AudioObjectGetPropertyData(device, &volumeAddress, 0, NULL, &size, &scalar);
    if (status != noErr) die([NSString stringWithFormat:@"can't read volume (%d)", status]);
    UInt32 mute = 0;
    size = sizeof mute;
    if (AudioObjectHasProperty(device, &muteAddress)) {
        status = AudioObjectGetPropertyData(device, &muteAddress, 0, NULL, &size, &mute);
        if (status != noErr) die([NSString stringWithFormat:@"can't read mute (%d)", status]);
    }
    *outVolume = (int)lround(scalar * 100);
    *outMuted = mute != 0;
}

static void writeVolume(int value) {
    Float32 scalar = value / 100.0f;
    OSStatus status = AudioObjectSetPropertyData(outputDevice(), &volumeAddress, 0, NULL, sizeof scalar, &scalar);
    if (status != noErr) warn([NSString stringWithFormat:@"can't set volume (%d)", status]);
}

static void writeMute(bool value) {
    AudioObjectID device = outputDevice();
    if (!AudioObjectHasProperty(device, &muteAddress)) return warn(@"output device has no mute");
    UInt32 mute = value;
    OSStatus status = AudioObjectSetPropertyData(device, &muteAddress, 0, NULL, sizeof mute, &mute);
    if (status != noErr) warn([NSString stringWithFormat:@"can't set mute (%d)", status]);
}

static double now(void) {
    return CFAbsoluteTimeGetCurrent();
}

static void startAnimation(void) {
    if (animating) return;
    animating = true;
    dispatch_resume(animation);
}

static void reveal(void) {
    visibleUntil = now() + VisibleSeconds;
    startAnimation();
}

static void setVolume(int value) {
    volume = MAX(0, MIN(100, value));
    writeVolume(volume);
    if (muted && value > 0) {
        muted = false;
        writeMute(false);
    }
    reveal();
}

static void drag(double pixels) {
    dragRemainder += pixels * DragPercentPerPixel;
    int amount = (int)dragRemainder;
    if (amount == 0) return;
    dragRemainder -= amount;
    setVolume(volume + amount);
}

static void step(int direction) {
    double t = now();
    double dt = direction == lastDirection ? t - lastTick : 1;
    lastTick = t;
    lastDirection = direction;
    int amount = dt > 0.2 ? 1 : dt > 0.1 ? 2 : dt > 0.05 ? 3 : 4;
    setVolume(volume + direction * amount);
}

static void toggleMute(void) {
    muted = !muted;
    writeMute(muted);
    reveal();
}

static void pollAudio(void) {
    int v;
    bool m;
    readAudio(&v, &m);
    if (v == volume && m == muted) return;
    bool first = volume < 0;
    volume = v;
    muted = m;
    if (first) shown = m ? 0 : v;
    else reveal();
}

static void pushVolume(void) {
    setVariable(@"volume_anim", [NSString stringWithFormat:@"%.1f", shown]);
    setVariable(@"volume", muted ? @"MUTE" : [NSString stringWithFormat:@"%d", volume]);
    setVariable(@"volume_alpha", [NSString stringWithFormat:@"%.0f", alpha]);
}

static void animate(void) {
    double target = muted ? 0 : volume;
    shown += (target - shown) * Easing;
    if (fabs(target - shown) < 0.3) shown = target;
    bool visible = now() < visibleUntil;
    double targetAlpha = visible ? 100 : 0;
    alpha += (targetAlpha - alpha) * (targetAlpha > alpha ? FadeIn : FadeOut);
    if (fabs(targetAlpha - alpha) < 1) alpha = targetAlpha;
    pushVolume();
    if (!visible && shown == target && alpha == 0) {
        animating = false;
        dispatch_suspend(animation);
    }
}

static NSString *stringValue(CFDictionaryRef info, NSString *key) {
    id value = info ? ((__bridge NSDictionary *)info)[key] : nil;
    return [value isKindOfClass:NSString.class] ? value : @"";
}

static void pollNowPlaying(void) {
    getNowPlayingInfo(dispatch_get_main_queue(), ^(CFDictionaryRef info) {
        NSString *title = stringValue(info, @"kMRMediaRemoteNowPlayingInfoTitle");
        NSString *artist = stringValue(info, @"kMRMediaRemoteNowPlayingInfoArtist");
        setVariable(@"np_title", title);
        setVariable(@"np_artist", artist);
        setVariable(@"np_track", artist.length && title.length ? [NSString stringWithFormat:@"%@ — %@", artist, title] : title);
    });
    getIsPlaying(dispatch_get_main_queue(), ^(Boolean playing) {
        setVariable(@"np_playing", playing ? @"1" : @"0");
    });
    getNowPlayingClient(dispatch_get_main_queue(), ^(id client) {
        NSString *bundle = client ? (__bridge NSString *)getClientBundleIdentifier(client) : nil;
        setVariable(@"np_app", bundle ?: @"");
    });
}

static void respond(int client, NSString *status) {
    NSString *response = [NSString stringWithFormat:@"HTTP/1.1 %@\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", status];
    write(client, response.UTF8String, strlen(response.UTF8String));
    close(client);
}

static void handle(int client) {
    char buffer[1024];
    ssize_t n = read(client, buffer, sizeof buffer - 1);
    if (n <= 0) return (void)close(client);
    buffer[n] = 0;
    NSArray<NSString *> *parts = [[NSString stringWithUTF8String:buffer] componentsSeparatedByString:@" "];
    NSURLComponents *url = [NSURLComponents componentsWithString:parts.count > 1 ? parts[1] : @""];
    NSString *path = url.path ?: @"";
    NSString *dy = [url.queryItems filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"name == 'dy'"]].firstObject.value;
    if ([path isEqualToString:@"/volume/drag"] && dy) drag(dy.doubleValue);
    else if ([path isEqualToString:@"/volume/up"]) step(1);
    else if ([path isEqualToString:@"/volume/down"]) step(-1);
    else if ([path isEqualToString:@"/volume/mute"]) toggleMute();
    else return respond(client, @"404 Not Found");
    respond(client, @"204 No Content");
}

static void listenHTTP(void) {
    int server = socket(AF_INET, SOCK_STREAM, 0);
    int yes = 1;
    setsockopt(server, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof yes);
    struct sockaddr_in address = {.sin_family = AF_INET, .sin_port = htons(Port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (bind(server, (struct sockaddr *)&address, sizeof address) != 0) die([NSString stringWithFormat:@"can't bind 127.0.0.1:%d: %s", Port, strerror(errno)]);
    if (listen(server, 16) != 0) die([NSString stringWithFormat:@"can't listen: %s", strerror(errno)]);
    dispatch_source_t source = dispatch_source_create(DISPATCH_SOURCE_TYPE_READ, server, 0, dispatch_get_main_queue());
    dispatch_source_set_event_handler(source, ^{
        int client = accept(server, NULL, NULL);
        if (client >= 0) handle(client);
    });
    dispatch_resume(source);
    CFRetain((__bridge CFTypeRef)source);
}

static dispatch_source_t timer(double seconds, void (^block)(void)) {
    dispatch_source_t source = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0, dispatch_get_main_queue());
    dispatch_source_set_timer(source, DISPATCH_TIME_NOW, (uint64_t)(seconds * NSEC_PER_SEC), (uint64_t)(seconds * NSEC_PER_SEC / 10));
    dispatch_source_set_event_handler(source, block);
    return source;
}

static void every(double seconds, void (^block)(void)) {
    dispatch_source_t source = timer(seconds, block);
    dispatch_resume(source);
    CFRetain((__bridge CFTypeRef)source);
}

void streamd_main(void *perl, void *cv) {
    void *mediaRemote = dlopen("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote", RTLD_NOW);
    if (!mediaRemote) die(@"MediaRemote unavailable");
    getNowPlayingInfo = (GetNowPlayingInfo)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingInfo");
    getIsPlaying = (GetIsPlaying)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingApplicationIsPlaying");
    getNowPlayingClient = (GetNowPlayingClient)dlsym(mediaRemote, "MRMediaRemoteGetNowPlayingClient");
    getClientBundleIdentifier = (GetClientBundleIdentifier)dlsym(mediaRemote, "MRNowPlayingClientGetBundleIdentifier");
    if (!getNowPlayingInfo || !getIsPlaying || !getNowPlayingClient || !getClientBundleIdentifier) die(@"MediaRemote symbols missing");

    NSURLSessionConfiguration *config = NSURLSessionConfiguration.ephemeralSessionConfiguration;
    config.HTTPMaximumConnectionsPerHost = 1;
    config.timeoutIntervalForRequest = 2;
    session = [NSURLSession sessionWithConfiguration:config];
    sent = [NSMutableDictionary dictionary];

    animation = timer(1 / AnimationHz, ^{ animate(); });
    pollAudio();
    pushVolume();
    listenHTTP();
    every(0.25, ^{ pollAudio(); });
    every(0.5, ^{ pollNowPlaying(); });
    every(10, ^{
        [sent removeAllObjects];
        pushVolume();
    });
    fprintf(stderr, "streamd: listening on 127.0.0.1:%d\n", Port);
    dispatch_main();
}
