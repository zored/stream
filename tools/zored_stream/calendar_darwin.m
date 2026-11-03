#import <EventKit/EventKit.h>
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
        NSPredicate *rp = [store predicateForIncompleteRemindersWithDueDateStarting:nil ending:end calendars:nil];
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        __block NSArray<EKReminder *> *reminders;
        [store fetchRemindersMatchingPredicate:rp completion:^(NSArray<EKReminder *> *r) {
            reminders = r;
            dispatch_semaphore_signal(done);
        }];
        dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
        NSCalendar *cal = [NSCalendar currentCalendar];
        for (EKReminder *r in reminders) {
            NSDateComponents *c = r.dueDateComponents;
            NSDate *due = c ? [cal dateFromComponents:c] : nil;
            if (!due) continue;
            BOOL allDay = c.hour == NSDateComponentUndefined;
            [out addObject:@{@"id": r.calendarItemIdentifier, @"ext": @"", @"title": r.title ?: @"",
                             @"start": @(due.timeIntervalSince1970), @"end": @(due.timeIntervalSince1970),
                             @"allDay": @(allDay), @"reminder": @YES}];
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

char *cal_complete(const char *id) {
    @autoreleasepool {
        char *err = ensure();
        if (err) return err;
        EKCalendarItem *item = [store calendarItemWithIdentifier:[NSString stringWithUTF8String:id]];
        if (![item isKindOfClass:[EKReminder class]]) return dup_error(@"reminder not found");
        EKReminder *r = (EKReminder *)item;
        r.completed = YES;
        NSError *e;
        if (![store saveReminder:r commit:YES error:&e]) return dup_error(e.localizedDescription);
        return NULL;
    }
}
