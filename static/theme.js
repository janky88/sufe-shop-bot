// Theme Management System
window.themeManager = {
    // Get current theme
    getTheme: function() {
        return localStorage.getItem('theme') || 'light';
    },
    
    // Set theme
    setTheme: function(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('theme', theme);
        this.updateThemeToggle(theme);
    },
    
    // Toggle theme
    toggleTheme: function() {
        const currentTheme = this.getTheme();
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';
        this.setTheme(newTheme);
    },
    
    // Update toggle button state
    updateThemeToggle: function(theme) {
        const toggleBtn = document.querySelector('.theme-toggle');
        if (toggleBtn) {
            toggleBtn.classList.toggle('dark', theme === 'dark');
        }
    },
    
    // Initialize theme
    init: function() {
        const savedTheme = this.getTheme();
        this.setTheme(savedTheme);
        
        // Add keyboard shortcut
        document.addEventListener('keydown', (e) => {
            if (e.ctrlKey && e.shiftKey && e.key === 'T') {
                e.preventDefault();
                this.toggleTheme();
            }
        });
    }
};

// Initialize on DOM load
document.addEventListener('DOMContentLoaded', function() {
    window.themeManager.init();
});