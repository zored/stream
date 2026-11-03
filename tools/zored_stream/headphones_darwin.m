#import <IOBluetooth/IOBluetooth.h>
#include <CoreAudio/CoreAudio.h>
#include "headphones_darwin.h"

static IOBluetoothDevice *bt_device(const char *address) {
    return [IOBluetoothDevice deviceWithAddressString:[NSString stringWithUTF8String:address]];
}

int bt_connected(const char *address) {
    @autoreleasepool {
        IOBluetoothDevice *d = bt_device(address);
        return d ? d.isConnected : -1;
    }
}

int bt_connect(const char *address) {
    @autoreleasepool {
        IOBluetoothDevice *d = bt_device(address);
        return d ? [d openConnection] : kIOReturnNotFound;
    }
}

int bt_disconnect(const char *address) {
    @autoreleasepool {
        IOBluetoothDevice *d = bt_device(address);
        return d ? [d closeConnection] : kIOReturnNotFound;
    }
}

static int has_streams(AudioObjectID id, AudioObjectPropertyScope scope) {
    AudioObjectPropertyAddress a = {kAudioDevicePropertyStreams, scope, kAudioObjectPropertyElementMain};
    UInt32 size = 0;
    return AudioObjectGetPropertyDataSize(id, &a, 0, NULL, &size) == noErr && size > 0;
}

int audio_devices(audio_device *out, int max) {
    AudioObjectPropertyAddress a = {kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr) return -1;
    int n = size / sizeof(AudioObjectID);
    AudioObjectID ids[n];
    if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, ids) != noErr) return -1;
    int count = 0;
    for (int i = 0; i < n && count < max; i++) {
        audio_device *d = &out[count];
        d->id = ids[i];
        CFStringRef name = NULL;
        UInt32 s = sizeof name;
        AudioObjectPropertyAddress na = {kAudioObjectPropertyName, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
        if (AudioObjectGetPropertyData(ids[i], &na, 0, NULL, &s, &name) != noErr || !name) continue;
        CFStringGetCString(name, d->name, sizeof d->name, kCFStringEncodingUTF8);
        CFRelease(name);
        AudioObjectPropertyAddress ta = {kAudioDevicePropertyTransportType, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
        s = sizeof d->transport;
        d->transport = 0;
        AudioObjectGetPropertyData(ids[i], &ta, 0, NULL, &s, &d->transport);
        d->input = has_streams(ids[i], kAudioObjectPropertyScopeInput);
        d->output = has_streams(ids[i], kAudioObjectPropertyScopeOutput);
        count++;
    }
    return count;
}

static AudioObjectPropertySelector default_selector(int input) {
    return input ? kAudioHardwarePropertyDefaultInputDevice : kAudioHardwarePropertyDefaultOutputDevice;
}

int audio_default(int input, unsigned int *id) {
    AudioObjectPropertyAddress a = {default_selector(input), kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    AudioObjectID device = 0;
    UInt32 size = sizeof device;
    OSStatus status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, &device);
    *id = device;
    return status;
}

int audio_set_default(int input, unsigned int id) {
    AudioObjectPropertyAddress a = {default_selector(input), kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    AudioObjectID device = id;
    return AudioObjectSetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, sizeof device, &device);
}
