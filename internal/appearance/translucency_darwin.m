#import <AppKit/AppKit.h>

static NSString *const vibrancyIdentifier = @"orchestra.vibrancy";
static NSColor *opaqueBackground;

int orchestraReduceTransparency(void) {
 return NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceTransparency;
}

// Adds or removes a vibrancy view behind the transparent webview of the main window.
void orchestraSetTranslucent(int on) {
 dispatch_async(dispatch_get_main_queue(), ^{
  for (NSWindow *window in NSApp.windows) {
   if (![window isKindOfClass:NSClassFromString(@"WailsWindow")]) continue;
   NSView *content = window.contentView;
   NSView *effect = nil;
   for (NSView *view in content.subviews)
    if ([view.identifier isEqualToString:vibrancyIdentifier]) effect = view;
   if (on && !effect) {
    NSVisualEffectView *view = [[NSVisualEffectView alloc] initWithFrame:content.bounds];
    view.identifier = vibrancyIdentifier;
    view.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    view.blendingMode = NSVisualEffectBlendingModeBehindWindow;
    view.material = NSVisualEffectMaterialUnderWindowBackground;
    view.state = NSVisualEffectStateActive;
    [content addSubview:view positioned:NSWindowBelow relativeTo:nil];
    [view release];
    if (!opaqueBackground) opaqueBackground = [window.backgroundColor retain];
    window.opaque = NO;
    window.backgroundColor = NSColor.clearColor;
   } else if (!on && effect) {
    [effect removeFromSuperview];
    window.opaque = YES;
    if (opaqueBackground) window.backgroundColor = opaqueBackground;
   }
  }
 });
}
