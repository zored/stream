#include <ApplicationServices/ApplicationServices.h>

enum { AX_BAR, AX_LIKE, AX_DISLIKE };
enum { AX_CMD = 1, AX_SHIFT = 2 };

pid_t ax_app_pid(const char *bundle);
int ax_trusted(int prompt);
AXUIElementRef ax_app(pid_t pid);
int ax_alive(AXUIElementRef element);
AXUIElementRef ax_find(AXUIElementRef root, int kind, int max_nodes, int *truncated);
AXUIElementRef ax_find_attr(AXUIElementRef root, const char *role, const char *attr, const char *text, int prefix,
                            int max_nodes, int *truncated);
char *ax_string(AXUIElementRef element, const char *attr);
AXUIElementRef ax_focused_window(AXUIElementRef app);
void ax_key(pid_t pid, int keycode, int flags);
int ax_int_value(AXUIElementRef element, int *out);
int ax_press(AXUIElementRef element);
int ax_focus(AXUIElementRef element);
enum { AX_CARD_QUESTION, AX_CARD_BOX, AX_CARD_PERMISSION };
char *ax_card_at(AXUIElementRef anchor, int kind);
void ax_card_anchors(AXUIElementRef window, AXUIElementRef *anchors);
int ax_card_press(AXUIElementRef window, int kind, const char *text);
void ax_release(AXUIElementRef element);
int ax_front_watch(void);
pid_t ax_front_pid(void);
