#import <AppKit/AppKit.h>

// 0 follows the system, 1 is Aqua and 2 is Dark Aqua. The app-wide appearance also covers menus
// and the About panel.
void orchestraSetAppearance(int mode) {
 dispatch_async(dispatch_get_main_queue(), ^{
  NSApp.appearance = mode == 0 ? nil : [NSAppearance appearanceNamed:mode == 1 ? NSAppearanceNameAqua : NSAppearanceNameDarkAqua];
 });
}
