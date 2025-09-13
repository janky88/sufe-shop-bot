// Simple Charts for Dashboard
(function() {
    'use strict';
    
    // Simple chart drawing functions
    function drawLineChart(canvas, values, color) {
        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const padding = 5;
        
        // Clear canvas
        ctx.clearRect(0, 0, width, height);
        
        if (!values || values.length < 2) return;
        
        // Find min and max values
        const max = Math.max(...values);
        const min = Math.min(...values);
        const range = max - min || 1;
        
        // Calculate points
        const points = values.map((value, index) => ({
            x: padding + (index / (values.length - 1)) * (width - 2 * padding),
            y: padding + (1 - (value - min) / range) * (height - 2 * padding)
        }));
        
        // Draw line
        ctx.beginPath();
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        
        points.forEach((point, index) => {
            if (index === 0) {
                ctx.moveTo(point.x, point.y);
            } else {
                ctx.lineTo(point.x, point.y);
            }
        });
        
        ctx.stroke();
        
        // Draw gradient fill
        const gradient = ctx.createLinearGradient(0, 0, 0, height);
        gradient.addColorStop(0, color + '40');
        gradient.addColorStop(1, color + '00');
        
        ctx.fillStyle = gradient;
        ctx.lineTo(points[points.length - 1].x, height - padding);
        ctx.lineTo(points[0].x, height - padding);
        ctx.closePath();
        ctx.fill();
    }
    
    function drawBarChart(canvas, values, color) {
        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const padding = 5;
        
        // Clear canvas
        ctx.clearRect(0, 0, width, height);
        
        if (!values || values.length === 0) return;
        
        const max = Math.max(...values) || 1;
        const barWidth = (width - 2 * padding) / values.length - 2;
        
        ctx.fillStyle = color;
        
        values.forEach((value, index) => {
            const barHeight = (value / max) * (height - 2 * padding);
            const x = padding + index * (barWidth + 2);
            const y = height - padding - barHeight;
            
            ctx.fillRect(x, y, barWidth, barHeight);
        });
    }
    
    // Initialize mini charts
    function initMiniCharts() {
        document.querySelectorAll('.stat-chart').forEach(canvas => {
            const type = canvas.dataset.type;
            const values = JSON.parse(canvas.dataset.values || '[]');
            const color = canvas.dataset.color || '#667eea';
            
            if (type === 'line') {
                drawLineChart(canvas, values, color);
            } else if (type === 'bar') {
                drawBarChart(canvas, values, color);
            }
        });
    }
    
    // Initialize larger charts (placeholder for now)
    function initMainCharts() {
        // Sales trend chart
        const salesCanvas = document.getElementById('salesChart');
        if (salesCanvas) {
            // This would be replaced with actual data
            drawLineChart(salesCanvas, [10, 25, 30, 45, 40, 55, 60, 58, 70, 65, 80, 75], '#667eea');
        }
        
        // Orders chart
        const ordersCanvas = document.getElementById('ordersChart');
        if (ordersCanvas) {
            drawBarChart(ordersCanvas, [30, 45, 25, 50, 40, 60, 55], '#10b981');
        }
        
        // Products chart
        const productsCanvas = document.getElementById('productsChart');
        if (productsCanvas) {
            drawBarChart(productsCanvas, [40, 30, 20, 10], '#fb923c');
        }
    }
    
    // Initialize all charts
    function init() {
        initMiniCharts();
        initMainCharts();
    }
    
    // Run on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();