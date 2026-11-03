#import <AppKit/AppKit.h>
#include "ax_darwin.h"
#include <unistd.h>

pid_t ax_app_pid(const char *bundle) {
    @autoreleasepool {
        NSArray<NSRunningApplication *> *apps =
            [NSRunningApplication runningApplicationsWithBundleIdentifier:[NSString stringWithUTF8String:bundle]];
        return apps.count ? apps.firstObject.processIdentifier : 0;
    }
}

int ax_trusted(int prompt) {
    if (!prompt) return AXIsProcessTrusted();
    const void *keys[] = {kAXTrustedCheckOptionPrompt};
    const void *values[] = {kCFBooleanTrue};
    CFDictionaryRef options = CFDictionaryCreate(NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    int trusted = AXIsProcessTrustedWithOptions(options);
    CFRelease(options);
    return trusted;
}

AXUIElementRef ax_app(pid_t pid) {
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    AXUIElementSetAttributeValue(app, CFSTR("AXManualAccessibility"), kCFBooleanTrue);
    return app;
}

int ax_alive(AXUIElementRef element) {
    CFTypeRef role = NULL;
    AXError error = AXUIElementCopyAttributeValue(element, kAXRoleAttribute, &role);
    if (role) CFRelease(role);
    return error == kAXErrorSuccess;
}

static int has_class(AXUIElementRef element, CFStringRef prefix) {
    CFTypeRef classes = NULL;
    if (AXUIElementCopyAttributeValue(element, CFSTR("AXDOMClassList"), &classes) != kAXErrorSuccess || !classes) return 0;
    int found = 0;
    if (CFGetTypeID(classes) == CFArrayGetTypeID()) {
        for (CFIndex i = 0; i < CFArrayGetCount(classes) && !found; i++) {
            CFTypeRef name = CFArrayGetValueAtIndex(classes, i);
            found = CFGetTypeID(name) == CFStringGetTypeID() && CFStringHasPrefix(name, prefix);
        }
    }
    CFRelease(classes);
    return found;
}

static int has_description(AXUIElementRef element, CFStringRef expected) {
    CFTypeRef description = NULL;
    if (AXUIElementCopyAttributeValue(element, kAXDescriptionAttribute, &description) != kAXErrorSuccess || !description) return 0;
    int equal = CFGetTypeID(description) == CFStringGetTypeID() && CFStringCompare(description, expected, 0) == kCFCompareEqualTo;
    CFRelease(description);
    return equal;
}

static int matches(AXUIElementRef element, int kind) {
    static CFStringRef dislike;
    if (!dislike) dislike = CFStringCreateWithCString(NULL, "Не нравится", kCFStringEncodingUTF8);
    switch (kind) {
    case AX_BAR: return has_class(element, CFSTR("PlayerBar_root"));
    case AX_LIKE: return has_class(element, CFSTR("PlayerBarDesktop_likeButton"));
    default: return has_description(element, dislike);
    }
}

typedef int (*ax_match)(AXUIElementRef element, const void *context);

static AXUIElementRef bfs(AXUIElementRef root, ax_match match, const void *context, int max_nodes, int *truncated) {
    CFMutableArrayRef queue = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
    CFMutableSetRef seen = CFSetCreateMutable(NULL, 0, &kCFTypeSetCallBacks);
    CFArrayAppendValue(queue, root);
    CFSetAddValue(seen, root);
    AXUIElementRef found = NULL;
    *truncated = 0;
    for (CFIndex next = 0; next < CFArrayGetCount(queue); next++) {
        if (next >= max_nodes) {
            *truncated = 1;
            break;
        }
        AXUIElementRef element = (AXUIElementRef)CFArrayGetValueAtIndex(queue, next);
        if (match(element, context)) {
            found = (AXUIElementRef)CFRetain(element);
            break;
        }
        CFTypeRef children = NULL;
        if (AXUIElementCopyAttributeValue(element, kAXChildrenAttribute, &children) != kAXErrorSuccess || !children) continue;
        if (CFGetTypeID(children) == CFArrayGetTypeID()) {
            for (CFIndex i = 0; i < CFArrayGetCount(children); i++) {
                CFTypeRef child = CFArrayGetValueAtIndex(children, i);
                if (CFSetContainsValue(seen, child)) continue;
                CFSetAddValue(seen, child);
                CFArrayAppendValue(queue, child);
            }
        }
        CFRelease(children);
    }
    CFRelease(queue);
    CFRelease(seen);
    return found;
}

static int match_kind(AXUIElementRef element, const void *context) {
    return matches(element, *(const int *)context);
}

AXUIElementRef ax_find(AXUIElementRef root, int kind, int max_nodes, int *truncated) {
    return bfs(root, match_kind, &kind, max_nodes, truncated);
}

typedef struct {
    CFStringRef role, attr, text;
    int prefix;
} attr_query;

static CFStringRef copy_string(AXUIElementRef element, CFStringRef attr) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, attr, &value) != kAXErrorSuccess || !value) return NULL;
    if (CFGetTypeID(value) != CFStringGetTypeID()) {
        CFRelease(value);
        return NULL;
    }
    return value;
}

static int match_attr(AXUIElementRef element, const void *context) {
    const attr_query *q = context;
    CFStringRef role = copy_string(element, kAXRoleAttribute);
    int ok = role && CFEqual(role, q->role);
    if (role) CFRelease(role);
    if (!ok) return 0;
    CFStringRef value = copy_string(element, q->attr);
    if (!value) return 0;
    ok = q->prefix ? CFStringHasPrefix(value, q->text) : CFEqual(value, q->text);
    CFRelease(value);
    return ok;
}

AXUIElementRef ax_find_attr(AXUIElementRef root, const char *role, const char *attr, const char *text, int prefix,
                            int max_nodes, int *truncated) {
    attr_query q = {
        CFStringCreateWithCString(NULL, role, kCFStringEncodingUTF8),
        CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8),
        CFStringCreateWithCString(NULL, text, kCFStringEncodingUTF8),
        prefix,
    };
    AXUIElementRef found = bfs(root, match_attr, &q, max_nodes, truncated);
    CFRelease(q.role);
    CFRelease(q.attr);
    CFRelease(q.text);
    return found;
}

char *ax_string(AXUIElementRef element, const char *attr) {
    CFStringRef name = CFStringCreateWithCString(NULL, attr, kCFStringEncodingUTF8);
    CFStringRef value = copy_string(element, name);
    CFRelease(name);
    if (!value) return NULL;
    CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength(value), kCFStringEncodingUTF8) + 1;
    char *out = malloc(size);
    if (!CFStringGetCString(value, out, size, kCFStringEncodingUTF8)) {
        free(out);
        out = NULL;
    }
    CFRelease(value);
    return out;
}

AXUIElementRef ax_focused_window(AXUIElementRef app) {
    CFTypeRef window = NULL;
    if (AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute, &window) != kAXErrorSuccess) return NULL;
    return window;
}

void ax_key(pid_t pid, int keycode, int flags) {
    CGEventFlags mask = 0;
    if (flags & AX_CMD) mask |= kCGEventFlagMaskCommand;
    if (flags & AX_SHIFT) mask |= kCGEventFlagMaskShift;
    for (int down = 1; down >= 0; down--) {
        CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down);
        CGEventSetFlags(event, mask);
        CGEventPostToPid(pid, event);
        CFRelease(event);
    }
}

int ax_int_value(AXUIElementRef element, int *out) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, kAXValueAttribute, &value) != kAXErrorSuccess || !value) return 0;
    int ok = CFGetTypeID(value) == CFNumberGetTypeID() && CFNumberGetValue(value, kCFNumberIntType, out);
    CFRelease(value);
    return ok;
}

int ax_press(AXUIElementRef element) {
    return AXUIElementPerformAction(element, kAXPressAction);
}

int ax_focus(AXUIElementRef element) {
    return AXUIElementSetAttributeValue(element, kAXFocusedAttribute, kCFBooleanTrue);
}

void ax_release(AXUIElementRef element) {
    CFRelease(element);
}

static void card_walk(AXUIElementRef element, int depth, int *budget, void (^visit)(AXUIElementRef, NSString *)) {
    if (depth > 40 || --*budget < 0) return;
    CFStringRef role = copy_string(element, kAXRoleAttribute);
    visit(element, role ? (__bridge NSString *)role : @"");
    if (role) CFRelease(role);
    CFTypeRef children = NULL;
    if (AXUIElementCopyAttributeValue(element, kAXChildrenAttribute, &children) != kAXErrorSuccess || !children) return;
    if (CFGetTypeID(children) == CFArrayGetTypeID()) {
        for (CFIndex i = 0; i < CFArrayGetCount(children); i++)
            card_walk((AXUIElementRef)CFArrayGetValueAtIndex(children, i), depth + 1, budget, visit);
    }
    CFRelease(children);
}

static NSString *card_attr(AXUIElementRef element, CFStringRef attr) {
    CFStringRef value = copy_string(element, attr);
    return value ? (__bridge NSString *)CFAutorelease(value) : @"";
}

static int is_option(AXUIElementRef element, NSString *role) {
    return [role isEqualToString:@"AXCheckBox"] && [card_attr(element, kAXSubroleAttribute) isEqualToString:@"AXToggleButton"] &&
           card_attr(element, kAXDescriptionAttribute).length == 0 && card_attr(element, kAXTitleAttribute).length > 0;
}

static int is_box_option(AXUIElementRef element, NSString *role) {
    return [role isEqualToString:@"AXButton"] && card_attr(element, kAXDescriptionAttribute).length == 0 &&
           card_attr(element, kAXTitleAttribute).length > 0;
}

static int card_option(int kind, AXUIElementRef element, NSString *role) {
    return kind == AX_CARD_QUESTION ? is_option(element, role) : is_box_option(element, role);
}

static AXUIElementRef card_root(AXUIElementRef window, int kind) {
    int truncated = 0;
    AXUIElementRef node =
        kind == AX_CARD_QUESTION     ? ax_find_attr(window, "AXButton", "AXTitle", "Skip", 0, 20000, &truncated)
        : kind == AX_CARD_PERMISSION ? ax_find_attr(window, "AXStaticText", "AXValue", "Permission requested", 1, 20000, &truncated)
                                     : ax_find_attr(window, "AXGroup", "AXDescription", "Suggested next", 1, 20000, &truncated);
    for (int level = 0; node && level < 6; level++) {
        CFTypeRef parent = NULL;
        AXUIElementCopyAttributeValue(node, kAXParentAttribute, &parent);
        CFRelease(node);
        node = parent;
        if (!node) break;
        __block int options = 0;
        int budget = 2000;
        card_walk(node, 0, &budget, ^(AXUIElementRef e, NSString *role) { options += card_option(kind, e, role); });
        if (options > 0) return node;
    }
    if (node) CFRelease(node);
    return NULL;
}

char *ax_card(AXUIElementRef window, int kind) {
    @autoreleasepool {
        AXUIElementRef root = card_root(window, kind);
        if (!root) return NULL;
        NSMutableString *out = [NSMutableString string];
        int budget = 2000;
        card_walk(root, 0, &budget, ^(AXUIElementRef e, NSString *role) {
            NSString *title = card_attr(e, kAXTitleAttribute), *desc = card_attr(e, kAXDescriptionAttribute);
            if (card_option(kind, e, role)) {
                int value = 0;
                ax_int_value(e, &value);
                [out appendFormat:@"opt\t%@\t%d\n", title, value];
            } else if ([role isEqualToString:@"AXButton"]) {
                [out appendFormat:@"btn\t%@\n", title.length ? title : desc];
            } else if ([role isEqualToString:@"AXPopUpButton"]) {
                [out appendFormat:@"pop\t%@\n", title.length ? title : desc];
            } else if ([role isEqualToString:@"AXStaticText"]) {
                NSString *value = card_attr(e, kAXValueAttribute);
                if ([value hasSuffix:@" selected"] || [value rangeOfString:@"^[0-9]+ of [0-9]+$" options:NSRegularExpressionSearch].location != NSNotFound)
                    [out appendFormat:@"txt\t%@\n", value];
            }
        });
        CFRelease(root);
        return strdup(out.UTF8String);
    }
}

int ax_card_press(AXUIElementRef window, int kind, const char *text) {
    @autoreleasepool {
        AXUIElementRef root = card_root(window, kind);
        if (!root) return -1;
        NSString *want = [NSString stringWithUTF8String:text];
        __block AXUIElementRef hit = NULL;
        int budget = 2000;
        card_walk(root, 0, &budget, ^(AXUIElementRef e, NSString *role) {
            if (hit || !([role isEqualToString:@"AXButton"] || [role isEqualToString:@"AXCheckBox"] || [role isEqualToString:@"AXPopUpButton"])) return;
            if ([card_attr(e, kAXTitleAttribute) isEqualToString:want] || [card_attr(e, kAXDescriptionAttribute) isEqualToString:want])
                hit = (AXUIElementRef)CFRetain(e);
        });
        CFRelease(root);
        if (!hit) return -2;
        AXError err = AXUIElementPerformAction(hit, kAXPressAction);
        CFRelease(hit);
        return err;
    }
}

static _Atomic pid_t front_pid;

int ax_front_watch(void) {
    @autoreleasepool {
        NSWorkspace *ws = NSWorkspace.sharedWorkspace;
        NSRunningApplication *app = ws.frontmostApplication;
        if (!app) return -1;
        front_pid = app.processIdentifier;
        id token = [ws.notificationCenter addObserverForName:NSWorkspaceDidActivateApplicationNotification
                                                      object:nil
                                                       queue:NSOperationQueue.mainQueue
                                                  usingBlock:^(NSNotification *n) {
            NSRunningApplication *a = n.userInfo[NSWorkspaceApplicationKey];
            if (a) front_pid = a.processIdentifier;
        }];
        return token ? 0 : -1;
    }
}

pid_t ax_front_pid(void) {
    return front_pid;
}
