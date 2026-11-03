typedef struct {
    unsigned int id;
    char name[128];
    unsigned int transport;
    int input;
    int output;
} audio_device;

int bt_connected(const char *address);
int bt_connect(const char *address);
int bt_disconnect(const char *address);
int audio_devices(audio_device *out, int max);
int audio_default(int input, unsigned int *id);
int audio_set_default(int input, unsigned int id);
