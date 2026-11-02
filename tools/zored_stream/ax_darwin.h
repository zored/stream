#include <ApplicationServices/ApplicationServices.h>

enum { AX_BAR, AX_LIKE, AX_DISLIKE };

pid_t ax_app_pid(const char *bundle);
int ax_trusted(int prompt);
AXUIElementRef ax_app(pid_t pid);
int ax_alive(AXUIElementRef element);
AXUIElementRef ax_find(AXUIElementRef root, int kind, int max_nodes, int *truncated);
int ax_int_value(AXUIElementRef element, int *out);
int ax_press(AXUIElementRef element);
void ax_release(AXUIElementRef element);
