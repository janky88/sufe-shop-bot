# Theme System Documentation

## Overview
The shop-bot admin interface now includes a comprehensive dark/light theme system with the following features:

- **Theme Toggle**: Easy switching between light and dark modes
- **Persistent Preference**: Theme choice is saved in localStorage
- **Keyboard Shortcut**: Ctrl+Shift+T to toggle theme
- **Smooth Transitions**: All theme changes animate smoothly
- **System Preference Support**: Defaults to system theme preference
- **Mobile Support**: Updates meta theme-color for mobile browsers

## Theme Colors

### Light Theme
- Primary Background: `#ffffff`
- Secondary Background: `#f5f7fa`
- Text Primary: `#1a1a2e`
- Text Secondary: `#4a4a6a`
- Border Color: `#dee2e6`
- Glass Background: `rgba(255, 255, 255, 0.8)`

### Dark Theme (Softer, Not Pure Black)
- Primary Background: `#1a1a2e`
- Secondary Background: `#16213e`
- Text Primary: `#e9ecef`
- Text Secondary: `#adb5bd`
- Border Color: `#2d3561`
- Glass Background: `rgba(255, 255, 255, 0.1)`

## Files Created

1. **`/templates/theme.css`**: Main theme CSS file with all CSS variables
2. **`/templates/theme.js`**: JavaScript theme manager
3. **`/templates/base.html`**: Updated base template with theme support
4. **`/templates/dashboard.html`**: Updated dashboard with theme support
5. **`/templates/login.html`**: Updated login page with theme support

## Implementation Guide

### 1. Include Theme Files
Add these to your HTML head:
```html
<link rel="stylesheet" href="/static/theme.css">
```

And before closing body tag:
```html
<script src="/static/theme.js"></script>
```

### 2. Add Theme Toggle Button
```html
<div class="theme-toggle" title="切换主题 (Ctrl+Shift+T)">
    <span class="theme-toggle-icon sun">☀️</span>
    <span class="theme-toggle-icon moon">🌙</span>
</div>
```

### 3. Use CSS Variables
Replace hardcoded colors with CSS variables:
```css
/* Instead of: */
color: #1a1a2e;
background: rgba(255, 255, 255, 0.1);

/* Use: */
color: var(--text-primary);
background: var(--glass-bg);
```

### 4. Add Transitions
Add theme transition to elements:
```css
.element {
    transition: var(--theme-transition);
}
```

## CSS Variables Reference

### Layout Colors
- `--bg-primary`: Main background color
- `--bg-secondary`: Secondary background color
- `--bg-tertiary`: Tertiary background color

### Text Colors
- `--text-primary`: Main text color
- `--text-secondary`: Secondary text color
- `--text-tertiary`: Tertiary/muted text color

### UI Elements
- `--glass-bg`: Glassmorphism background
- `--glass-border`: Glassmorphism border
- `--border-color`: Regular borders
- `--shadow-color`: Box shadows
- `--hover-bg`: Hover state background

### Gradients (Same for Both Themes)
- `--primary-gradient`: Primary brand gradient
- `--success-gradient`: Success state gradient
- `--danger-gradient`: Danger/error gradient
- `--warning-gradient`: Warning gradient
- `--info-gradient`: Info gradient

## JavaScript API

The theme manager provides these methods:

```javascript
// Get current theme
const currentTheme = window.themeManager.getTheme(); // 'light' or 'dark'

// Set theme programmatically
window.themeManager.setTheme('dark');

// Toggle theme
window.themeManager.toggleTheme();

// Reset to system preference
window.themeManager.resetToSystem();

// Listen for theme changes
window.addEventListener('themechange', (e) => {
    console.log('Theme changed to:', e.detail.theme);
});
```

## Updating Remaining Templates

To update templates to use the theme system:

1. Add the HTML structure with theme support
2. Replace hardcoded colors with CSS variables
3. Add the theme toggle button in the header
4. Include theme.css and theme.js files
5. Test both light and dark modes

## Best Practices

1. **Always use CSS variables** for colors that should change with theme
2. **Test both themes** when adding new features
3. **Add transitions** to new elements for smooth theme switching
4. **Consider contrast** in both themes for accessibility
5. **Use semantic variable names** (e.g., `var(--text-primary)` not `var(--black)`)

## Browser Support

The theme system supports all modern browsers:
- Chrome/Edge 88+
- Firefox 78+
- Safari 14+
- Mobile browsers with CSS variables support

## Future Enhancements

Consider adding:
- More theme options (e.g., high contrast, custom colors)
- Theme preview before applying
- Automatic theme switching based on time of day
- Per-page theme preferences
- Theme export/import functionality