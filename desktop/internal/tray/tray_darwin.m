//go:build darwin && cgo

// Значок SafeBridge в строке меню macOS. Меню приходит из Go как JSON и собирается
// в главном потоке; выбор пункта возвращается в Go через sbTrayClick.
#import <Cocoa/Cocoa.h>

extern void sbTrayClick(char *ident);

@interface SBTrayTarget : NSObject
- (void)click:(NSMenuItem *)item;
@end

@implementation SBTrayTarget
- (void)click:(NSMenuItem *)item {
    NSString *ident = item.representedObject;
    if ([ident isKindOfClass:[NSString class]]) {
        sbTrayClick((char *)[ident UTF8String]);
    }
}
@end

static NSStatusItem *sbItem = nil;
static SBTrayTarget *sbTarget = nil;

static NSMenu *sbBuildMenu(NSArray *items) {
    NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];
    menu.autoenablesItems = NO;
    for (id raw in items) {
        if (![raw isKindOfClass:[NSDictionary class]]) continue;
        NSDictionary *d = raw;
        if ([d[@"sep"] boolValue]) {
            [menu addItem:[NSMenuItem separatorItem]];
            continue;
        }
        NSString *title = [d[@"title"] isKindOfClass:[NSString class]] ? d[@"title"] : @"";
        NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:title action:nil keyEquivalent:@""];
        NSArray *sub = d[@"items"];
        if ([sub isKindOfClass:[NSArray class]] && sub.count > 0) {
            mi.submenu = sbBuildMenu(sub);
        } else if ([d[@"id"] isKindOfClass:[NSString class]]) {
            mi.target = sbTarget;
            mi.action = @selector(click:);
            mi.representedObject = d[@"id"];
        }
        mi.enabled = ![d[@"disabled"] boolValue];
        mi.state = [d[@"checked"] boolValue] ? NSOnState : NSOffState;
        [menu addItem:mi];
    }
    return menu;
}

void sbTraySet(const void *png, int pngLen, const char *menuJSON, const char *tooltip) {
    NSData *img = [NSData dataWithBytes:png length:(NSUInteger)pngLen];
    NSData *js = [NSData dataWithBytes:menuJSON length:strlen(menuJSON)];
    NSString *tip = [NSString stringWithUTF8String:tooltip];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sbTarget == nil) sbTarget = [[SBTrayTarget alloc] init];
        if (sbItem == nil) {
            sbItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
        }
        NSImage *image = [[NSImage alloc] initWithData:img];
        [image setSize:NSMakeSize(18, 18)];
        [image setTemplate:YES]; // цвет как у остальных значков: светлая и тёмная строка меню
        sbItem.button.image = image;
        sbItem.button.toolTip = tip;
        NSArray *items = [NSJSONSerialization JSONObjectWithData:js options:0 error:nil];
        if ([items isKindOfClass:[NSArray class]]) {
            sbItem.menu = sbBuildMenu(items);
        }
    });
}

void sbTrayRemove(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sbItem != nil) {
            [[NSStatusBar systemStatusBar] removeStatusItem:sbItem];
            sbItem = nil;
        }
    });
}

// Кнопки окна («закрыть», «свернуть», «развернуть»): на macOS 10.13 они пропадали.
// Показываем их явно и возвращаем состояние для журнала.
const char *sbFixWindowButtons(void) {
    __block NSString *info = @"";
    void (^fix)(void) = ^{
        NSMutableString *s = [NSMutableString string];
        for (NSWindow *w in [NSApp windows]) {
            if (![w isVisible] && ![w isMiniaturized]) continue;
            w.styleMask |= NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable;
            NSWindowButton kinds[] = { NSWindowCloseButton, NSWindowMiniaturizeButton, NSWindowZoomButton };
            [s appendFormat:@"окно mask=%lu", (unsigned long)w.styleMask];
            for (int i = 0; i < 3; i++) {
                NSButton *b = [w standardWindowButton:kinds[i]];
                [s appendFormat:@" кнопка%d=%@%@", i, b == nil ? @"нет" : (b.isHidden ? @"скрыта" : @"видна"), b.superview == nil ? @"(вне окна)" : @""];
                if (b != nil) [b setHidden:NO];
            }
            [s appendString:@"; "];
        }
        info = s;
    };
    if ([NSThread isMainThread]) fix(); else dispatch_sync(dispatch_get_main_queue(), fix);
    return strdup([info UTF8String]);
}
