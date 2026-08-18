// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

#import <Cocoa/Cocoa.h>

// removeAboutMenuItem removes the standard "About" item from the application
// menu so the only About is the in-app one. The Cocoa about panel only shows
// static bundle data and cannot reflect the loaded core.
//
// This may be called at startup before the menu exists, so the work is deferred
// onto the main queue and runs once after the menu has been created.
void removeAboutMenuItem(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSMenu *mainMenu = [NSApp mainMenu];
        if (mainMenu == nil) {
            return;
        }
        NSMenu *appMenu = [[mainMenu itemAtIndex:0] submenu];
        if (appMenu == nil) {
            return;
        }
        NSInteger idx = [appMenu indexOfItemWithTarget:nil
                                             andAction:@selector(orderFrontStandardAboutPanel:)];
        if (idx < 0) {
            return;
        }
        [appMenu removeItemAtIndex:idx];
        // Drop the separator the About item left behind, if present.
        if (idx < [appMenu numberOfItems] && [[appMenu itemAtIndex:idx] isSeparatorItem]) {
            [appMenu removeItemAtIndex:idx];
        }
    });
}
