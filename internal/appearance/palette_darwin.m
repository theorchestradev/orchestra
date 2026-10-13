#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <string.h>
char *orchestraSystemPalette(int dark) {
 @autoreleasepool {
  NSMutableArray *values = [NSMutableArray array];
  NSAppearance *appearance = [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
  [appearance performAsCurrentDrawingAppearance:^{
   NSArray *colors = @[NSColor.windowBackgroundColor, NSColor.controlBackgroundColor, NSColor.controlBackgroundColor, NSColor.labelColor, NSColor.secondaryLabelColor];
   for (NSColor *color in colors) {
    NSColor *rgb = [color colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
    [values addObject:[NSString stringWithFormat:@"#%02X%02X%02X", (int)round(rgb.redComponent * 255), (int)round(rgb.greenComponent * 255), (int)round(rgb.blueComponent * 255)]];
   }
   [values addObject:dark ? @"#393B3F" : @"#DDE0E4"];
   NSColor *accent = [NSColor.controlAccentColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
   [values addObject:[NSString stringWithFormat:@"#%02X%02X%02X", (int)round(accent.redComponent * 255), (int)round(accent.greenComponent * 255), (int)round(accent.blueComponent * 255)]];
   [values addObject:dark ? @"#34363A" : @"#E6E8EB"];
  }];
  NSData *data = [NSJSONSerialization dataWithJSONObject:values options:0 error:nil];
  return strdup([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding].UTF8String);
 }
}
