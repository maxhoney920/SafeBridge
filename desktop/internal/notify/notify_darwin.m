//go:build darwin && cgo

// Уведомление macOS (NSUserNotification: работает и на 10.13, и без подписи разработчика).
// По нажатию открывается ссылка из userInfo (продление подписки).
#import <Cocoa/Cocoa.h>

#pragma clang diagnostic ignored "-Wdeprecated-declarations"

@interface SBNotifyDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation SBNotifyDelegate
- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center shouldPresentNotification:(NSUserNotification *)n {
    return YES; // показывать, даже когда окно SafeBridge на переднем плане
}
- (void)userNotificationCenter:(NSUserNotificationCenter *)center didActivateNotification:(NSUserNotification *)n {
    NSString *link = n.userInfo[@"link"];
    if ([link isKindOfClass:[NSString class]] && link.length > 0) {
        NSURL *u = [NSURL URLWithString:link];
        if (u != nil && [u.scheme isEqualToString:@"https"]) [[NSWorkspace sharedWorkspace] openURL:u];
    } else {
        [NSApp activateIgnoringOtherApps:YES];
    }
    [center removeDeliveredNotification:n];
}
@end

static SBNotifyDelegate *sbNotifyDelegate = nil;

void sbNotify(const char *title, const char *text, const char *link) {
    NSString *t = [NSString stringWithUTF8String:title];
    NSString *x = [NSString stringWithUTF8String:text];
    NSString *l = [NSString stringWithUTF8String:link];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSUserNotificationCenter *c = [NSUserNotificationCenter defaultUserNotificationCenter];
        if (sbNotifyDelegate == nil) {
            sbNotifyDelegate = [[SBNotifyDelegate alloc] init];
            c.delegate = sbNotifyDelegate;
        }
        NSUserNotification *n = [[NSUserNotification alloc] init];
        n.title = t;
        n.informativeText = x;
        n.soundName = NSUserNotificationDefaultSoundName;
        if (l.length > 0) n.userInfo = @{ @"link": l };
        [c deliverNotification:n];
    });
}
