#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <string.h>
char *orchestraMonospaceFonts(void) {
 @autoreleasepool {
  NSMutableSet *families = [NSMutableSet set];
  for (NSString *name in [NSFontManager.sharedFontManager availableFontNamesWithTraits:NSFixedPitchFontMask]) {
   NSString *family = [NSFont fontWithName:name size:12].familyName;
   if (family.length) [families addObject:family];
  }
  NSData *data = [NSJSONSerialization dataWithJSONObject:families.allObjects options:0 error:nil];
  if (!data) return NULL;
  return strdup([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding].UTF8String);
 }
}
