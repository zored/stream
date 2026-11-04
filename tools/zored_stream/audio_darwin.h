#include <CoreAudio/CoreAudio.h>

OSStatus audio_read(Float32 *volume, UInt32 *mute);
OSStatus audio_write_volume(Float32 volume);
OSStatus audio_write_mute(UInt32 mute);
OSStatus audio_watch(void);
