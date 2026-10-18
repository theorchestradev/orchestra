//go:build darwin

#import <AppKit/AppKit.h>

// Wails' AppMenu role cannot contain custom items. Move the registered items so
// their existing Wails targets, callbacks and keyboard shortcuts remain intact.
void orchestraMoveAppMenuItems(void) {
 dispatch_async(dispatch_get_main_queue(), ^{
  NSMenu *main = NSApp.mainMenu;
  NSMenu *app = main.itemArray.firstObject.submenu;
  NSMenu *file = [main itemWithTitle:@"File"].submenu;
  NSMenuItem *settings = [file itemWithTitle:@"Settings…"];
  NSMenuItem *update = [file itemWithTitle:@"Check for Updates…"];
  if (app == nil || settings == nil || update == nil) return;
  [settings retain];
  [update retain];
  [file removeItem:settings];
  [file removeItem:update];
  // The native About item and its separator already occupy the first two slots.
  NSInteger index = MIN(2, app.numberOfItems);
  [app insertItem:settings atIndex:index++];
  [app insertItem:update atIndex:index++];
  [app insertItem:[NSMenuItem separatorItem] atIndex:index];
  [settings release];
  [update release];
 });
}
