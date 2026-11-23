#import <AppKit/AppKit.h>

void *orca_svg_png(const void *svg, int len, int px, int *out_len) {
    @autoreleasepool {
        NSImage *image = [[NSImage alloc] initWithData:[NSData dataWithBytes:svg length:len]];
        if (!image) return NULL;
        NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:px pixelsHigh:px bitsPerSample:8
            samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSCalibratedRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
        [NSGraphicsContext saveGraphicsState];
        [NSGraphicsContext setCurrentContext:[NSGraphicsContext graphicsContextWithBitmapImageRep:rep]];
        [image drawInRect:NSMakeRect(0, 0, px, px)];
        [NSGraphicsContext restoreGraphicsState];
        NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (!png) return NULL;
        void *out = malloc(png.length);
        memcpy(out, png.bytes, png.length);
        *out_len = (int)png.length;
        return out;
    }
}
