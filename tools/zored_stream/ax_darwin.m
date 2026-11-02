#import <AppKit/AppKit.h>
#include "ax_darwin.h"

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

pid_t ax_frontmost_pid(void) {
    @autoreleasepool {
        return NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier;
    }
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

void ax_release(AXUIElementRef element) {
    CFRelease(element);
}
