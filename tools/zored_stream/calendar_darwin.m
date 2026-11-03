#import <EventKit/EventKit.h>
#import <CoreText/CoreText.h>
#import <ImageIO/ImageIO.h>
#include <stdlib.h>
#include <string.h>
#include "calendar_darwin.h"

static EKEventStore *store;
static int granted_events, granted_reminders;

static char *dup_error(NSString *s) {
    return strdup(s.UTF8String);
}

static int request(EKEntityType type) {
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL ok = NO;
    void (^finish)(BOOL, NSError *) = ^(BOOL g, NSError *e) {
        ok = g;
        dispatch_semaphore_signal(done);
    };
    if (type == EKEntityTypeEvent) {
        [store requestFullAccessToEventsWithCompletion:finish];
    } else {
        [store requestFullAccessToRemindersWithCompletion:finish];
    }
    dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
    return ok;
}

static char *ensure(void) {
    if (!store) store = [[EKEventStore alloc] init];
    if (!granted_events) granted_events = request(EKEntityTypeEvent);
    if (!granted_reminders) granted_reminders = request(EKEntityTypeReminder);
    if (!granted_events) return dup_error(@"calendars: access denied");
    if (!granted_reminders) return dup_error(@"reminders: access denied");
    return NULL;
}

char *cal_items(double from, double to, char **err) {
    @autoreleasepool {
        if ((*err = ensure())) return NULL;
        [store refreshSourcesIfNecessary];
        NSDate *start = [NSDate dateWithTimeIntervalSince1970:from];
        NSDate *end = [NSDate dateWithTimeIntervalSince1970:to];
        NSMutableArray *out = [NSMutableArray array];
        NSPredicate *ep = [store predicateForEventsWithStartDate:start endDate:end calendars:nil];
        for (EKEvent *e in [store eventsMatchingPredicate:ep]) {
            if (e.status == EKEventStatusCanceled) continue;
            [out addObject:@{@"id": e.calendarItemIdentifier, @"ext": e.eventIdentifier ?: @"",
                             @"title": e.title ?: @"", @"notes": e.notes ?: @"", @"start": @(e.startDate.timeIntervalSince1970),
                             @"end": @(e.endDate.timeIntervalSince1970), @"allDay": @(e.allDay), @"reminder": @NO}];
        }
        NSCalendar *cal = [NSCalendar currentCalendar];
        NSArray *preds = @[
            [store predicateForIncompleteRemindersWithDueDateStarting:nil ending:end calendars:nil],
            [store predicateForCompletedRemindersWithCompletionDateStarting:[cal startOfDayForDate:[NSDate date]] ending:end calendars:nil],
        ];
        for (NSPredicate *rp in preds) {
            dispatch_semaphore_t done = dispatch_semaphore_create(0);
            __block NSArray<EKReminder *> *reminders;
            [store fetchRemindersMatchingPredicate:rp completion:^(NSArray<EKReminder *> *r) {
                reminders = r;
                dispatch_semaphore_signal(done);
            }];
            dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
            for (EKReminder *r in reminders) {
                NSDateComponents *c = r.dueDateComponents;
                NSDate *due = c ? [cal dateFromComponents:c] : r.completionDate;
                if (!due) continue;
                BOOL allDay = !c || c.hour == NSDateComponentUndefined;
                [out addObject:@{@"id": r.calendarItemIdentifier, @"ext": @"", @"title": r.title ?: @"", @"notes": r.notes ?: @"",
                                 @"start": @(due.timeIntervalSince1970), @"end": @(due.timeIntervalSince1970),
                                 @"allDay": @(allDay), @"reminder": @YES, @"done": @(r.completed)}];
            }
        }
        NSError *e;
        NSData *json = [NSJSONSerialization dataWithJSONObject:out options:0 error:&e];
        if (!json) {
            *err = dup_error(e.localizedDescription);
            return NULL;
        }
        char *s = malloc(json.length + 1);
        memcpy(s, json.bytes, json.length);
        s[json.length] = 0;
        return s;
    }
}

char *cal_set_done(const char *id, int done) {
    @autoreleasepool {
        char *err = ensure();
        if (err) return err;
        EKCalendarItem *item = [store calendarItemWithIdentifier:[NSString stringWithUTF8String:id]];
        if (![item isKindOfClass:[EKReminder class]]) return dup_error(@"reminder not found");
        EKReminder *r = (EKReminder *)item;
        r.completed = done;
        NSError *e;
        if (![store saveReminder:r commit:YES error:&e]) return dup_error(e.localizedDescription);
        return NULL;
    }
}

typedef struct {
    CGContextRef ctx;
    double h;
} canvas;

void *cal_canvas_new(double w, double h, double scale, int rotate) {
    canvas *c = malloc(sizeof *c);
    c->h = h;
    double bw = rotate ? h : w, bh = rotate ? w : h;
    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    c->ctx = CGBitmapContextCreate(NULL, bw * scale, bh * scale, 8, 0, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
    CGColorSpaceRelease(cs);
    CGContextScaleCTM(c->ctx, scale, scale);
    if (rotate) {
        CGContextTranslateCTM(c->ctx, bw, 0);
        CGContextRotateCTM(c->ctx, M_PI_2);
    }
    return c;
}

static CGColorRef rgb_color(unsigned rgb) {
    return CGColorCreateSRGB(((rgb >> 16) & 255) / 255.0, ((rgb >> 8) & 255) / 255.0, (rgb & 255) / 255.0, 1);
}

void cal_canvas_text(void *c, const char *text, const char *tail, double x, double y, double w, double h, double size, unsigned rgb, unsigned tail_rgb, int center) {
    @autoreleasepool {
        CGContextRef ctx = ((canvas *)c)->ctx;
        double H = ((canvas *)c)->h;
        CTFontRef font = CTFontCreateWithName(CFSTR("JetBrains Mono"), size, NULL);
        CGColorRef color = rgb_color(rgb), tail_color = rgb_color(tail_rgb);
        CTTextAlignment align = center ? kCTTextAlignmentCenter : kCTTextAlignmentLeft;
        CTLineBreakMode wrap = kCTLineBreakByCharWrapping;
        CGFloat lh = size * 1.25;
        CTParagraphStyleSetting ps[] = {{kCTParagraphStyleSpecifierAlignment, sizeof align, &align},
                                        {kCTParagraphStyleSpecifierLineBreakMode, sizeof wrap, &wrap},
                                        {kCTParagraphStyleSpecifierMinimumLineHeight, sizeof lh, &lh},
                                        {kCTParagraphStyleSpecifierMaximumLineHeight, sizeof lh, &lh}};
        CTParagraphStyleRef style = CTParagraphStyleCreate(ps, 4);
        NSDictionary *attrs = @{(id)kCTFontAttributeName: (__bridge id)font, (id)kCTForegroundColorAttributeName: (__bridge id)color,
                                (id)kCTParagraphStyleAttributeName: (__bridge id)style};
        NSMutableAttributedString *as = [[NSMutableAttributedString alloc] initWithString:[NSString stringWithUTF8String:text] attributes:attrs];
        if (*tail) {
            NSMutableDictionary *ta = [attrs mutableCopy];
            ta[(id)kCTForegroundColorAttributeName] = (__bridge id)tail_color;
            [as appendAttributedString:[[NSAttributedString alloc] initWithString:[NSString stringWithUTF8String:tail] attributes:ta]];
        }
        CTFramesetterRef fs = CTFramesetterCreateWithAttributedString((__bridge CFAttributedStringRef)as);
        CGRect box = CGRectMake(x, H - y - h, w, h);
        CGPathRef path = CGPathCreateWithRect(CGRectMake(x, box.origin.y - H, w, h + H), NULL);
        CTFrameRef frame = CTFramesetterCreateFrame(fs, CFRangeMake(0, 0), path, NULL);
        CFArrayRef lines = CTFrameGetLines(frame);
        CFIndex n = CFArrayGetCount(lines);
        CGPoint origins[n];
        CTFrameGetLineOrigins(frame, CFRangeMake(0, 0), origins);
        double top = box.origin.y + h, ascent = CTFontGetAscent(font);
        CGContextSaveGState(ctx);
        CGContextClipToRect(ctx, box);
        for (CFIndex i = 0; i < n; i++) {
            CGContextSetTextPosition(ctx, x + origins[i].x, top - ascent - i * lh);
            CTLineDraw(CFArrayGetValueAtIndex(lines, i), ctx);
        }
        CGContextRestoreGState(ctx);
        CFRelease(frame);
        CGPathRelease(path);
        CFRelease(fs);
        CFRelease(style);
        CGColorRelease(color);
        CGColorRelease(tail_color);
        CFRelease(font);
    }
}

void cal_canvas_glyph(void *c, const char *text, double cx, double cy, double size, double alpha) {
    @autoreleasepool {
        CGContextRef ctx = ((canvas *)c)->ctx;
        double H = ((canvas *)c)->h;
        CTFontRef font = CTFontCreateWithName(CFSTR("Apple Color Emoji"), size, NULL);
        NSDictionary *attrs = @{(id)kCTFontAttributeName: (__bridge id)font};
        NSAttributedString *as = [[NSAttributedString alloc] initWithString:[NSString stringWithUTF8String:text] attributes:attrs];
        CTLineRef line = CTLineCreateWithAttributedString((__bridge CFAttributedStringRef)as);
        CGRect b = CTLineGetBoundsWithOptions(line, kCTLineBoundsUseGlyphPathBounds);
        CGContextSaveGState(ctx);
        CGContextSetAlpha(ctx, alpha);
        CGContextSetTextPosition(ctx, cx - b.origin.x - b.size.width / 2, H - cy - b.origin.y - b.size.height / 2);
        CTLineDraw(line, ctx);
        CGContextRestoreGState(ctx);
        CFRelease(line);
        CFRelease(font);
    }
}

void cal_canvas_ring(void *c, double cx, double cy, double r, double width, unsigned rgb) {
    CGContextRef ctx = ((canvas *)c)->ctx;
    double H = ((canvas *)c)->h;
    CGContextSetRGBStrokeColor(ctx, ((rgb >> 16) & 255) / 255.0, ((rgb >> 8) & 255) / 255.0, (rgb & 255) / 255.0, 1);
    CGContextSetLineWidth(ctx, width);
    CGContextStrokeEllipseInRect(ctx, CGRectMake(cx - r, H - cy - r, 2 * r, 2 * r));
}

void *cal_canvas_png(void *c, int *len) {
    @autoreleasepool {
        CGContextRef ctx = ((canvas *)c)->ctx;
        free(c);
        CGImageRef img = CGBitmapContextCreateImage(ctx);
        CGContextRelease(ctx);
        NSMutableData *data = [NSMutableData data];
        CGImageDestinationRef dst = CGImageDestinationCreateWithData((__bridge CFMutableDataRef)data, CFSTR("public.png"), 1, NULL);
        CGImageDestinationAddImage(dst, img, NULL);
        BOOL ok = CGImageDestinationFinalize(dst);
        CFRelease(dst);
        CGImageRelease(img);
        if (!ok) return NULL;
        *len = (int)data.length;
        void *out = malloc(data.length);
        memcpy(out, data.bytes, data.length);
        return out;
    }
}
