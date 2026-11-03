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
                             @"title": e.title ?: @"", @"start": @(e.startDate.timeIntervalSince1970),
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
                [out addObject:@{@"id": r.calendarItemIdentifier, @"ext": @"", @"title": r.title ?: @"",
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

static const double canvas_scale = 4;

void *cal_canvas_new(double w, double h) {
    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate(NULL, w * canvas_scale, h * canvas_scale, 8, 0, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
    CGColorSpaceRelease(cs);
    CGContextScaleCTM(ctx, canvas_scale, canvas_scale);
    return ctx;
}

void cal_canvas_text(void *c, const char *text, double x, double y, double w, double h, double size, unsigned rgb, int center) {
    @autoreleasepool {
        CGContextRef ctx = c;
        double H = CGBitmapContextGetHeight(ctx) / canvas_scale;
        CTFontRef font = CTFontCreateWithName(CFSTR("JetBrains Mono"), size, NULL);
        CGColorRef color = CGColorCreateSRGB(((rgb >> 16) & 255) / 255.0, ((rgb >> 8) & 255) / 255.0, (rgb & 255) / 255.0, 1);
        CTTextAlignment align = center ? kCTTextAlignmentCenter : kCTTextAlignmentLeft;
        CTLineBreakMode wrap = kCTLineBreakByCharWrapping;
        CTParagraphStyleSetting ps[] = {{kCTParagraphStyleSpecifierAlignment, sizeof align, &align},
                                        {kCTParagraphStyleSpecifierLineBreakMode, sizeof wrap, &wrap}};
        CTParagraphStyleRef style = CTParagraphStyleCreate(ps, 2);
        NSDictionary *attrs = @{(id)kCTFontAttributeName: (__bridge id)font, (id)kCTForegroundColorAttributeName: (__bridge id)color,
                                (id)kCTParagraphStyleAttributeName: (__bridge id)style};
        NSAttributedString *as = [[NSAttributedString alloc] initWithString:[NSString stringWithUTF8String:text] attributes:attrs];
        CTFramesetterRef fs = CTFramesetterCreateWithAttributedString((__bridge CFAttributedStringRef)as);
        CGRect box = CGRectMake(x, H - y - h, w, h);
        CGPathRef path = CGPathCreateWithRect(CGRectMake(x, box.origin.y - H, w, h + H), NULL);
        CTFrameRef frame = CTFramesetterCreateFrame(fs, CFRangeMake(0, 0), path, NULL);
        CGContextSaveGState(ctx);
        CGContextClipToRect(ctx, box);
        CTFrameDraw(frame, ctx);
        CGContextRestoreGState(ctx);
        CFRelease(frame);
        CGPathRelease(path);
        CFRelease(fs);
        CFRelease(style);
        CGColorRelease(color);
        CFRelease(font);
    }
}

void *cal_canvas_png(void *c, int *len) {
    @autoreleasepool {
        CGContextRef ctx = c;
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
