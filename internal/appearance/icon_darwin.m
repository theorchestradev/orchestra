#import <AppKit/AppKit.h>

int orchestraSetAppIcon(const void *bytes, int length) {
 NSData *data = [[NSData alloc] initWithBytes:bytes length:length];
 NSImage *image = [[NSImage alloc] initWithData:data];
 [data release];
 if (!image) return 0;
 dispatch_async(dispatch_get_main_queue(), ^{
  NSApp.applicationIconImage = image;
  [image release];
 });
 return 1;
}
