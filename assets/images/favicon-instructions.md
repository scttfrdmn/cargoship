# Favicon Creation Instructions

To create a favicon from the CargoShip logo, follow these steps:

## Manual Creation (Recommended)

1. **Use the main logo.png** as the source image
2. **Resize to 32x32 pixels** while maintaining clarity of the ship silhouette
3. **Save as favicon.ico** using an online converter or image editor
4. **Test the favicon** in a browser to ensure visibility

## Online Tools

Use these tools to convert the PNG logo to favicon:

- [RealFaviconGenerator](https://realfavicongenerator.net/) - Comprehensive favicon generator
- [Favicon.io](https://favicon.io/) - Simple PNG to ICO converter
- [ICO Convert](https://icoconvert.com/) - Batch icon converter

## Multiple Sizes

For modern web applications, create multiple favicon sizes:

- `favicon-16x16.png` - 16x16 pixels
- `favicon-32x32.png` - 32x32 pixels  
- `favicon.ico` - Traditional ICO format
- `apple-touch-icon.png` - 180x180 for iOS
- `android-chrome-192x192.png` - 192x192 for Android
- `android-chrome-512x512.png` - 512x512 for Android

## HTML Implementation

Add these tags to HTML pages:

```html
<link rel="icon" type="image/x-icon" href="/assets/images/favicon.ico">
<link rel="icon" type="image/png" sizes="32x32" href="/assets/images/favicon-32x32.png">
<link rel="icon" type="image/png" sizes="16x16" href="/assets/images/favicon-16x16.png">
<link rel="apple-touch-icon" sizes="180x180" href="/assets/images/apple-touch-icon.png">
<link rel="manifest" href="/site.webmanifest">
```

## Design Guidelines

- **Keep it simple** - The ship silhouette should be recognizable at 16x16 pixels
- **Use high contrast** - Navy blue ship on light background works well
- **Consider monochrome version** - For very small sizes, a simplified single-color version may work better
- **Test on different backgrounds** - Ensure visibility on both light and dark browser themes