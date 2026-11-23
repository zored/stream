int wifi_power(void);
int wifi_set_power(int on);
void wifi_disassociate(void);
int wifi_ssid(char **ssid);
char *wifi_list(int fresh, char **err);
char *wifi_join(const char *ssid);
