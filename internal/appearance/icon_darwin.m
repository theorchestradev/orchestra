#import <AppKit/AppKit.h>

void orchestraSetAppIcon(const void *bytes, size_t length) {
    // Copy before returning to Go; the main-queue block must not retain its memory.
    NSData *data = [[NSData alloc] initWithBytes:bytes length:length];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSImage *image = [[NSImage alloc] initWithData:data];
        if (image) [NSApp setApplicationIconImage:image];
        [image release];
    });
    [data release];
}
