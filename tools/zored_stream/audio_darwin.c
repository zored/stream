#include <AudioToolbox/AudioServices.h>
#include <dispatch/dispatch.h>
#include "audio_darwin.h"

extern void goAudioChanged(void);

static AudioObjectPropertyAddress volumeAddress = {kAudioHardwareServiceDeviceProperty_VirtualMainVolume, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};
static AudioObjectPropertyAddress muteAddress = {kAudioDevicePropertyMute, kAudioDevicePropertyScopeOutput, kAudioObjectPropertyElementMain};
static AudioObjectPropertyAddress defaultAddress = {kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};

static dispatch_queue_t queue;
static AudioObjectID watched = kAudioObjectUnknown;
static AudioObjectPropertyListenerBlock changed;

static OSStatus outputDevice(AudioObjectID *device) {
	UInt32 size = sizeof *device;
	return AudioObjectGetPropertyData(kAudioObjectSystemObject, &defaultAddress, 0, NULL, &size, device);
}

OSStatus audio_read(Float32 *volume, UInt32 *mute) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	UInt32 size = sizeof *volume;
	status = AudioObjectGetPropertyData(device, &volumeAddress, 0, NULL, &size, volume);
	if (status != noErr) return status;
	*mute = 0;
	if (!AudioObjectHasProperty(device, &muteAddress)) return noErr;
	size = sizeof *mute;
	return AudioObjectGetPropertyData(device, &muteAddress, 0, NULL, &size, mute);
}

OSStatus audio_write_volume(Float32 volume) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	return AudioObjectSetPropertyData(device, &volumeAddress, 0, NULL, sizeof volume, &volume);
}

OSStatus audio_write_mute(UInt32 mute) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr) return status;
	if (!AudioObjectHasProperty(device, &muteAddress)) return kAudioHardwareUnknownPropertyError;
	return AudioObjectSetPropertyData(device, &muteAddress, 0, NULL, sizeof mute, &mute);
}

static OSStatus watchDevice(void) {
	AudioObjectID device;
	OSStatus status = outputDevice(&device);
	if (status != noErr || device == watched) return status;
	if (watched != kAudioObjectUnknown) {
		AudioObjectRemovePropertyListenerBlock(watched, &volumeAddress, queue, changed);
		if (AudioObjectHasProperty(watched, &muteAddress)) AudioObjectRemovePropertyListenerBlock(watched, &muteAddress, queue, changed);
	}
	watched = device;
	status = AudioObjectAddPropertyListenerBlock(device, &volumeAddress, queue, changed);
	if (status != noErr || !AudioObjectHasProperty(device, &muteAddress)) return status;
	return AudioObjectAddPropertyListenerBlock(device, &muteAddress, queue, changed);
}

OSStatus audio_watch(void) {
	queue = dispatch_queue_create("zored_stream.audio", DISPATCH_QUEUE_SERIAL);
	changed = ^(UInt32 n, const AudioObjectPropertyAddress *a) { goAudioChanged(); };
	OSStatus status = watchDevice();
	if (status != noErr) return status;
	return AudioObjectAddPropertyListenerBlock(kAudioObjectSystemObject, &defaultAddress, queue,
		^(UInt32 n, const AudioObjectPropertyAddress *a) {
			if (watchDevice() != noErr) watched = kAudioObjectUnknown;
			goAudioChanged();
		});
}
