#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

char *orchestraApplicationPath(const char *bundleID) {
 @autoreleasepool {
  NSURL *url = [NSWorkspace.sharedWorkspace URLForApplicationWithBundleIdentifier:[NSString stringWithUTF8String:bundleID]];
  if (!url.path.length) return NULL;
  return strdup(url.fileSystemRepresentation);
 }
}

// Returns a size×size PNG of the app's icon, or NULL. The caller frees it.
void *orchestraApplicationIcon(const char *path, int size, int *length) {
 @autoreleasepool {
  NSImage *image = [NSWorkspace.sharedWorkspace iconForFile:[NSString stringWithUTF8String:path]];
  NSBitmapImageRep *bitmap = [[[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:size pixelsHigh:size bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0] autorelease];
  NSGraphicsContext *context = [NSGraphicsContext graphicsContextWithBitmapImageRep:bitmap];
  if (!image || !context) return NULL;
  [NSGraphicsContext saveGraphicsState];
  NSGraphicsContext.currentContext = context;
  [image drawInRect:NSMakeRect(0, 0, size, size) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
  [NSGraphicsContext restoreGraphicsState];
  NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
  if (!png.length) return NULL;
  void *bytes = malloc(png.length);
  memcpy(bytes, png.bytes, png.length);
  *length = (int)png.length;
  return bytes;
 }
}
