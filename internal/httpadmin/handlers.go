package httpadmin

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"github.com/gin-gonic/gin"
	"shop-bot/internal/store"
)

// Product endpoints

func (s *Server) handleProductList(c *gin.Context) {
	var products []store.Product
	if err := s.db.Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Add stock count to response
	type ProductWithStock struct {
		store.Product
		Stock int64 `json:"stock"`
	}
	
	var productsWithStock []ProductWithStock
	for _, p := range products {
		stock, _ := store.CountAvailableCodes(s.db, p.ID)
		productsWithStock = append(productsWithStock, ProductWithStock{
			Product: p,
			Stock:   stock,
		})
	}
	
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, productsWithStock)
		return
	}
	
	// HTML response
	c.HTML(http.StatusOK, "product_list.html", gin.H{
		"products": productsWithStock,
	})
}

func (s *Server) handleProductCreate(c *gin.Context) {
	var req struct {
		Name       string  `json:"name" binding:"required"`
		PriceCents int     `json:"price_cents"`
		Price      float64 `json:"price"` // Alternative: price in dollars
		IsActive   bool    `json:"is_active"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Convert price to cents if provided in dollars
	if req.Price > 0 && req.PriceCents == 0 {
		req.PriceCents = int(req.Price * 100)
	}
	
	product := store.Product{
		Name:       req.Name,
		PriceCents: req.PriceCents,
		IsActive:   req.IsActive,
	}
	
	if err := s.db.Create(&product).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, product)
}

func (s *Server) handleProductUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	
	var req struct {
		Name       string  `json:"name"`
		PriceCents int     `json:"price_cents"`
		Price      float64 `json:"price"`
		IsActive   *bool   `json:"is_active"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Price > 0 {
		updates["price_cents"] = int(req.Price * 100)
	} else if req.PriceCents > 0 {
		updates["price_cents"] = req.PriceCents
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	
	if err := s.db.Model(&store.Product{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (s *Server) handleProductDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	
	// Soft delete - just deactivate
	if err := s.db.Model(&store.Product{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "deactivated"})
}

// Inventory endpoints

func (s *Server) handleProductCodes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	
	// Get product
	var product store.Product
	if err := s.db.First(&product, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	
	// Get codes with pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit
	
	var codes []store.Code
	query := s.db.Where("product_id = ?", id)
	
	// Filter by sold status if requested
	if soldStr := c.Query("sold"); soldStr != "" {
		sold := soldStr == "true"
		query = query.Where("is_sold = ?", sold)
	}
	
	if err := query.Offset(offset).Limit(limit).Find(&codes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Get total count
	var total int64
	s.db.Model(&store.Code{}).Where("product_id = ?", id).Count(&total)
	
	c.JSON(http.StatusOK, gin.H{
		"product": product,
		"codes":   codes,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

func (s *Server) handleCodesUpload(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	
	// Parse multipart form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// Try to get codes from text field
		codesText := c.PostForm("codes")
		if codesText == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no file or codes provided"})
			return
		}
		
		// Process text codes
		codes := processCodesText(codesText, uint(id))
		if len(codes) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no valid codes found"})
			return
		}
		
		if err := s.db.Create(&codes).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%d codes uploaded", len(codes))})
		
		// Send stock update notification
		go s.sendStockUpdateNotification(product.Name, len(codes))
		
		return
	}
	defer file.Close()
	
	// Check file size (10MB limit)
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 10MB)"})
		return
	}
	
	// Process file
	scanner := bufio.NewScanner(file)
	var codes []store.Code
	lineNum := 0
	
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		codes = append(codes, store.Code{
			ProductID: uint(id),
			Code:      line,
			IsSold:    false,
		})
		
		// Batch insert every 100 codes
		if len(codes) >= 100 {
			if err := s.db.Create(&codes).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": fmt.Sprintf("error at line %d: %v", lineNum, err),
				})
				return
			}
			codes = codes[:0]
		}
	}
	
	// Insert remaining codes
	if len(codes) > 0 {
		if err := s.db.Create(&codes).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%d codes uploaded", lineNum)})
	
	// Get product for notification
	var product store.Product
	if err := s.db.First(&product, id).Error; err == nil {
		// Send stock update notification
		go s.sendStockUpdateNotification(product.Name, lineNum)
	}
}

func processCodesText(text string, productID uint) []store.Code {
	var codes []store.Code
	lines := strings.Split(text, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		codes = append(codes, store.Code{
			ProductID: productID,
			Code:      line,
			IsSold:    false,
		})
	}
	
	return codes
}

// Order endpoints

func (s *Server) handleOrderList(c *gin.Context) {
	// Parse filters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit
	
	query := s.db.Model(&store.Order{}).Preload("User").Preload("Product")
	
	// Filter by status
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	// Filter by date range
	if startDate := c.Query("start_date"); startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if endDate := c.Query("end_date"); endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			query = query.Where("created_at <= ?", t.Add(24*time.Hour))
		}
	}
	
	// Get total count
	var total int64
	query.Count(&total)
	
	// Get orders
	var orders []store.Order
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"orders": orders,
			"total":  total,
			"page":   page,
			"limit":  limit,
		})
		return
	}
	
	// HTML response
	c.HTML(http.StatusOK, "order_list.html", gin.H{
		"orders": orders,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// Dashboard

func (s *Server) handleAdminDashboard(c *gin.Context) {
	// Get statistics
	var stats struct {
		TotalProducts   int64
		TotalOrders     int64
		TotalUsers      int64
		PendingOrders   int64
		TodayOrders     int64
		TodayRevenue    int64
		TotalCodes      int64
		AvailableCodes  int64
	}
	
	s.db.Model(&store.Product{}).Count(&stats.TotalProducts)
	s.db.Model(&store.Order{}).Count(&stats.TotalOrders)
	s.db.Model(&store.User{}).Count(&stats.TotalUsers)
	s.db.Model(&store.Order{}).Where("status = ?", "pending").Count(&stats.PendingOrders)
	
	// Today's stats
	today := time.Now().Truncate(24 * time.Hour)
	s.db.Model(&store.Order{}).Where("created_at >= ?", today).Count(&stats.TodayOrders)
	
	var todayRevenue struct {
		Total int64
	}
	s.db.Model(&store.Order{}).
		Select("COALESCE(SUM(amount_cents), 0) as total").
		Where("status IN (?, ?) AND paid_at >= ?", "paid", "delivered", today).
		Scan(&todayRevenue)
	stats.TodayRevenue = todayRevenue.Total
	
	// Code stats
	s.db.Model(&store.Code{}).Count(&stats.TotalCodes)
	s.db.Model(&store.Code{}).Where("is_sold = ?", false).Count(&stats.AvailableCodes)
	
	// Recent orders
	var recentOrders []store.Order
	s.db.Preload("User").Preload("Product").
		Order("created_at DESC").
		Limit(10).
		Find(&recentOrders)
	
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"stats":         stats,
			"recent_orders": recentOrders,
		})
		return
	}
	
	// HTML response
	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"stats":         stats,
		"recent_orders": recentOrders,
	})
}