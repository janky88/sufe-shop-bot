package httpadmin

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
)

// Product endpoints

func (s *Server) handleProductList(c *gin.Context) {
	var products []store.Product

	// Check if we should show all products including inactive ones
	showAll := c.Query("show_all") == "true"

	query := s.db
	if !showAll {
		// By default, only show active products
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&products).Error; err != nil {
		logger.Error("Failed to fetch products", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	logger.Info("Fetched products", "count", len(products))
	
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
		logger.Info("Product", "id", p.ID, "name", p.Name, "price", p.PriceCents, "stock", stock)
	}
	
	if c.GetHeader("Accept") == "application/json" {
		// 确保返回完整的产品信息
		type ProductResponse struct {
			ID          uint   `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			PriceCents  int    `json:"price_cents"`
			IsActive    bool   `json:"is_active"`
			Stock       int64  `json:"stock"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
		}
		
		var response []ProductResponse
		for _, p := range productsWithStock {
			response = append(response, ProductResponse{
				ID:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				PriceCents:  p.PriceCents,
				IsActive:    p.IsActive,
				Stock:       p.Stock,
				CreatedAt:   p.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
			})
		}
		
		c.JSON(http.StatusOK, response)
		return
	}
	
	// Get currency settings
	_, symbol := store.GetCurrencySettings(s.db, s.config)
	
	// Add debug endpoint
	if c.Query("debug") == "true" {
		var rawProducts []store.Product
		s.db.Raw("SELECT * FROM products").Scan(&rawProducts)
		
		c.JSON(http.StatusOK, gin.H{
			"raw_products": rawProducts,
			"products_with_stock": productsWithStock,
			"currency": symbol,
		})
		return
	}
	
	// HTML response

	c.HTML(http.StatusOK, "product_list.html", gin.H{
		"products": productsWithStock,
		"currency": symbol,
		"show_all": showAll,
	})
}

func (s *Server) handleProductCreate(c *gin.Context) {
	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description string  `json:"description"`
		PriceCents  int     `json:"price_cents"`
		Price       float64 `json:"price"` // Alternative: price in dollars
		IsActive    bool    `json:"is_active"`
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
		Name:        req.Name,
		Description: req.Description,
		PriceCents:  req.PriceCents,
		IsActive:    true, // Default to active
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
		Name        string  `json:"name"`
		Description string  `json:"description"`
		PriceCents  int     `json:"price_cents"`
		Price       float64 `json:"price"`
		IsActive    *bool   `json:"is_active"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
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

// Product restore handler
func (s *Server) handleProductRestore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Restore - reactivate the product
	if err := s.db.Model(&store.Product{}).Where("id = ?", id).Update("is_active", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "restored"})
}

// Product permanent delete handler
func (s *Server) handleProductPermanentDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// First check if product exists and is inactive
	var product store.Product
	if err := s.db.First(&product, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Only allow permanent deletion of inactive products
	if product.IsActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot permanently delete active product"})
		return
	}

	// Check if there are related orders
	var orderCount int64
	s.db.Model(&store.Order{}).Where("product_id = ?", id).Count(&orderCount)
	if orderCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot delete product: %d related orders exist", orderCount)})
		return
	}

	// Check if there are related codes
	var codeCount int64
	s.db.Model(&store.Code{}).Where("product_id = ?", id).Count(&codeCount)
	if codeCount > 0 {
		// Delete related codes first
		if err := s.db.Where("product_id = ?", id).Delete(&store.Code{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete related codes: " + err.Error()})
			return
		}
		logger.Info("Deleted related codes", "product_id", id, "count", codeCount)
	}

	// Hard delete - permanently remove from database
	if err := s.db.Delete(&store.Product{}, id).Error; err != nil {
		// Check if it's a foreign key constraint error
		if strings.Contains(err.Error(), "foreign key constraint") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete product: it has related records in other tables"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Info("Product permanently deleted", "product_id", id, "product_name", product.Name)
	c.JSON(http.StatusOK, gin.H{"message": "permanently deleted"})
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
	
	c.HTML(http.StatusOK, "product_codes.html", gin.H{
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
		
		// Get product for notification
		var product store.Product
		if err := s.db.First(&product, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
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

func (s *Server) handleCodeDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	
	// Check if code exists and is not sold
	var code store.Code
	if err := s.db.First(&code, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "code not found"})
		return
	}
	
	if code.IsSold {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete sold code"})
		return
	}
	
	// Delete the code
	if err := s.db.Delete(&code).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "code deleted"})
}

func processCodesText(text string, productID uint) []store.Code {
	var codes []store.Code
	lines := strings.Split(text, "\n")
	
	// Support both single-line and multi-line codes
	// Multi-line codes are separated by empty lines
	var currentCode []string
	
	for _, line := range lines {
		// Check if this is a separator line (empty or only contains dashes/equals)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Trim(trimmed, "-=") == "" {
			// If we have accumulated lines, save them as a code
			if len(currentCode) > 0 {
				codeText := strings.Join(currentCode, "\n")
				codes = append(codes, store.Code{
					ProductID: productID,
					Code:      strings.TrimSpace(codeText),
					IsSold:    false,
				})
				currentCode = nil
			}
			continue
		}
		
		// Add line to current code
		currentCode = append(currentCode, line)
	}
	
	// Don't forget the last code if there's no trailing empty line
	if len(currentCode) > 0 {
		codeText := strings.Join(currentCode, "\n")
		codes = append(codes, store.Code{
			ProductID: productID,
			Code:      strings.TrimSpace(codeText),
			IsSold:    false,
		})
	}
	
	return codes
}

func (s *Server) handleCodeTemplate(c *gin.Context) {
	// Return template content
	templateContent := `【单行卡密示例】
=================
ABC123456789
DEF987654321
GHI456789123

【多行卡密示例 - 用空行分隔】
==============================
租户名: elmejorporteroesfran
用户名: elmejorporteroesfran@hotmail.com
密码: mudvoc-nitsa0-Nifjas
Region: Japan East (Tokyo)
绕后码: 897884934218 804031171594 747973230039

租户名: bestusertenant2023
用户名: user2023@outlook.com
密码: secure-pass-2023
Region: US West
绕后码: 123456789012 987654321098 456789123456

【混合格式示例】
===============
SINGLE-CODE-001
SINGLE-CODE-002

账号类型: 高级会员
用户名: vip001@email.com
密码: vip-password-001
有效期: 2025-12-31
激活码: VIP-ACTIVATE-001

SINGLE-CODE-003

【游戏账号示例】
===============
游戏: 王者荣耀
服务器: 微信区
账号: wxgame001
密码: game123456
角色: 满级账号

游戏: 原神
服务器: 亚服
UID: 800123456
密码: genshin2025
充值: 6480原石

【注意事项】
===========
1. 文件格式：必须是纯文本文件（.txt）
2. 编码格式：UTF-8（支持中文）
3. 单行卡密：每行一个卡密
4. 多行卡密：
   - 多行内容之间不要有空行
   - 不同卡密之间用空行分隔
   - 可以用分隔线（如 === 或 ---）来更清晰地分隔
5. 每次上传会自动识别格式并正确保存`
	
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"卡密上传模板.txt\"")
	c.String(http.StatusOK, templateContent)
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
	
	// Get orders with codes
	var orders []store.Order
	if err := query.Preload("User").Preload("Product").Order("created_at DESC").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Load codes for each order
	for i := range orders {
		if orders[i].Status == "delivered" && orders[i].ProductID != nil {
			var code store.Code
			if err := s.db.Where("order_id = ?", orders[i].ID).First(&code).Error; err == nil {
				orders[i].Code = &code
			}
		}
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
		TotalRevenue    int64
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

	// Total revenue
	var totalRevenue struct {
		Total int64
	}
	s.db.Model(&store.Order{}).
		Select("COALESCE(SUM(amount_cents), 0) as total").
		Where("status IN (?, ?)", "paid", "delivered").
		Scan(&totalRevenue)
	stats.TotalRevenue = totalRevenue.Total

	// Code stats
	s.db.Model(&store.Code{}).Count(&stats.TotalCodes)
	s.db.Model(&store.Code{}).Where("is_sold = ?", false).Count(&stats.AvailableCodes)

	// Get sales data for last 7 days
	salesData := make([]struct {
		Date   string
		Amount int64
		Count  int64
	}, 7)

	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i).Truncate(24 * time.Hour)
		nextDate := date.AddDate(0, 0, 1)

		var dailyStats struct {
			Amount int64
			Count  int64
		}

		s.db.Model(&store.Order{}).
			Select("COALESCE(SUM(amount_cents), 0) as amount, COUNT(*) as count").
			Where("status IN (?, ?) AND paid_at >= ? AND paid_at < ?", "paid", "delivered", date, nextDate).
			Scan(&dailyStats)

		salesData[6-i] = struct {
			Date   string
			Amount int64
			Count  int64
		}{
			Date:   date.Format("01-02"),
			Amount: dailyStats.Amount,
			Count:  dailyStats.Count,
		}
	}

	// Get order status distribution
	var orderStatus []struct {
		Status string
		Count  int64
	}
	s.db.Model(&store.Order{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&orderStatus)

	// Get top selling products
	var topProducts []struct {
		ProductID   uint
		ProductName string
		Count       int64
	}
	s.db.Table("orders").
		Select("product_id, products.name as product_name, COUNT(*) as count").
		Joins("LEFT JOIN products ON products.id = orders.product_id").
		Where("orders.status IN (?, ?) AND orders.product_id IS NOT NULL", "paid", "delivered").
		Group("product_id, products.name").
		Order("count DESC").
		Limit(5).
		Scan(&topProducts)

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
			"sales_data":    salesData,
			"order_status":  orderStatus,
			"top_products":  topProducts,
		})
		return
	}

	// Get currency settings
	_, symbol := store.GetCurrencySettings(s.db, s.config)

	// HTML response
	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"stats":         stats,
		"recent_orders": recentOrders,
		"currency":      symbol,
		"sales_data":    salesData,
		"order_status":  orderStatus,
		"top_products":  topProducts,
	})
}

// Settings handlers
func (s *Server) handleSettingsList(c *gin.Context) {
	// Get current currency settings
	currency, symbol := store.GetCurrencySettings(s.db, s.config)
	
	// Get order settings
	orderSettings, err := store.GetSettingsMap(s.db)
	if err != nil {
		logger.Error("Failed to get order settings", "error", err)
		orderSettings = make(map[string]string)
	}
	
	// Get order statistics
	orderStats, err := store.GetOrderStats(s.db)
	if err != nil {
		logger.Error("Failed to get order stats", "error", err)
		orderStats = make(map[string]int64)
	}
	
	// Map of available currencies
	currencies := []struct {
		Code   string
		Symbol string
		Name   string
	}{
		{"CNY", "¥", "人民币"},
		{"USD", "$", "美元"},
		{"EUR", "€", "欧元"},
		{"GBP", "£", "英镑"},
		{"JPY", "¥", "日元"},
		{"KRW", "₩", "韩元"},
		{"HKD", "HK$", "港币"},
		{"TWD", "NT$", "新台币"},
		{"SGD", "S$", "新加坡元"},
		{"CAD", "C$", "加元"},
		{"AUD", "A$", "澳元"},
	}
	
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{
			"currency":   currency,
			"symbol":     symbol,
			"currencies": currencies,
			"orderSettings": orderSettings,
			"orderStats": orderStats,
		})
		return
	}
	
	// HTML response
	c.HTML(http.StatusOK, "settings.html", gin.H{
		"currency":   currency,
		"symbol":     symbol,
		"currencies": currencies,
		"orderSettings": orderSettings,
		"orderStats": orderStats,
	})
}

func (s *Server) handleSettingsUpdate(c *gin.Context) {
	// Check content type to determine what's being updated
	var req map[string]interface{}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Handle currency settings
	if currency, ok := req["currency"].(string); ok {
		if symbol, ok := req["symbol"].(string); ok {
			// Update currency settings in database
			err := s.db.Transaction(func(tx *gorm.DB) error {
				if err := store.SetSystemSetting(tx, "currency", currency); err != nil {
					return err
				}
				if err := store.SetSystemSetting(tx, "currency_symbol", symbol); err != nil {
					return err
				}
				return nil
			})
			
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
				return
			}
		}
	}
	
	// Handle order settings
	for key, value := range req {
		switch key {
		case "order_expire_hours", "order_cleanup_days", "enable_auto_expire", "enable_auto_cleanup":
			valueStr := fmt.Sprintf("%v", value)
			var description, settingType string
			
			switch key {
			case "order_expire_hours":
				description = "订单过期时间（小时）"
				settingType = "int"
			case "order_cleanup_days":
				description = "清理过期订单的天数"
				settingType = "int"
			case "enable_auto_expire":
				description = "启用订单自动过期"
				settingType = "bool"
			case "enable_auto_cleanup":
				description = "启用过期订单自动清理"
				settingType = "bool"
			}
			
			if err := store.SetSetting(s.db, key, valueStr, description, settingType); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
				return
			}
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "设置已更新"})
}

// User management handlers

func (s *Server) handleUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 50
	offset := (page - 1) * limit

	// Build query
	query := s.db.Model(&store.User{})
	
	// Add search filter
	if search := c.Query("search"); search != "" {
		query = query.Where("username LIKE ? OR CAST(tg_user_id AS TEXT) LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	
	// Get total count
	var total int64
	query.Count(&total)
	
	// Get users with order count and total spent
	type UserWithStats struct {
		store.User
		OrderCount  int64
		TotalSpent  int64
		LastOrderAt *time.Time
	}
	
	var users []UserWithStats
	if err := s.db.Table("users").
		Select("users.*, COUNT(DISTINCT orders.id) as order_count, COALESCE(SUM(orders.amount_cents), 0) as total_spent, MAX(orders.created_at) as last_order_at").
		Joins("LEFT JOIN orders ON orders.user_id = users.id AND orders.status IN ('paid', 'delivered')").
		Group("users.id").
		Order("users.created_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// HTML response
	c.HTML(http.StatusOK, "user_list.html", gin.H{
		"users":  users,
		"total":  total,
		"page":   page,
		"limit":  limit,
		"search": c.Query("search"),
	})
}

func (s *Server) handleUserDetail(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	
	// Get user
	var user store.User
	if err := s.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Get user balance
	balance, _ := store.GetUserBalance(s.db, uint(userID))
	
	// Get user statistics
	var stats struct {
		TotalOrders     int64
		TotalSpent      int64
		PendingOrders   int64
		DeliveredOrders int64
	}
	
	s.db.Model(&store.Order{}).Where("user_id = ?", userID).Count(&stats.TotalOrders)
	s.db.Model(&store.Order{}).Where("user_id = ? AND status = ?", userID, "pending").Count(&stats.PendingOrders)
	s.db.Model(&store.Order{}).Where("user_id = ? AND status = ?", userID, "delivered").Count(&stats.DeliveredOrders)
	s.db.Model(&store.Order{}).Where("user_id = ? AND status IN (?)", userID, []string{"paid", "delivered"}).
		Select("COALESCE(SUM(amount_cents), 0)").Scan(&stats.TotalSpent)
	
	// Get recent orders
	var orders []store.Order
	s.db.Where("user_id = ?", userID).
		Preload("Product").
		Order("created_at DESC").
		Limit(20).
		Find(&orders)
		
	// Load codes for delivered orders
	for i := range orders {
		if orders[i].Status == "delivered" && orders[i].ProductID != nil {
			var code store.Code
			if err := s.db.Where("order_id = ?", orders[i].ID).First(&code).Error; err == nil {
				orders[i].Code = &code
			}
		}
	}
	
	// Get balance transactions
	var transactions []store.BalanceTransaction
	s.db.Where("user_id = ?", userID).
		Preload("RechargeCard").
		Preload("Order").
		Order("created_at DESC").
		Limit(20).
		Find(&transactions)
	
	// HTML response
	c.HTML(http.StatusOK, "user_detail.html", gin.H{
		"user":         user,
		"balance":      balance,
		"stats":        stats,
		"orders":       orders,
		"transactions": transactions,
	})
}

// FAQ management handlers

func (s *Server) handleFAQList(c *gin.Context) {
	lang := c.DefaultQuery("lang", "zh")
	
	// Get all FAQs for the language
	var faqs []store.FAQ
	if err := s.db.Where("language = ?", lang).Order("sort_order ASC, id ASC").Find(&faqs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.HTML(http.StatusOK, "faq_list.html", gin.H{
		"faqs":     faqs,
		"language": lang,
	})
}

func (s *Server) handleFAQCreate(c *gin.Context) {
	var req struct {
		Question  string `form:"question" binding:"required"`
		Answer    string `form:"answer" binding:"required"`
		Language  string `form:"language" binding:"required"`
		SortOrder int    `form:"sort_order"`
		IsActive  string `form:"is_active"`
	}

	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle checkbox value conversion
	isActive := req.IsActive == "on" || req.IsActive == "true"

	faq := store.FAQ{
		Question:  req.Question,
		Answer:    req.Answer,
		Language:  req.Language,
		SortOrder: req.SortOrder,
		IsActive:  isActive,
	}

	if err := s.db.Create(&faq).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "id": faq.ID})
}

func (s *Server) handleFAQUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	var faq store.FAQ
	if err := s.db.First(&faq, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "FAQ not found"})
		return
	}
	
	var req struct {
		Question  string `form:"question" binding:"required"`
		Answer    string `form:"answer" binding:"required"`
		Language  string `form:"language" binding:"required"`
		SortOrder int    `form:"sort_order"`
		IsActive  string `form:"is_active"`
	}

	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle checkbox value conversion
	isActive := req.IsActive == "on" || req.IsActive == "true"

	faq.Question = req.Question
	faq.Answer = req.Answer
	faq.Language = req.Language
	faq.SortOrder = req.SortOrder
	faq.IsActive = isActive
	
	if err := s.db.Save(&faq).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleFAQDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	if err := s.db.Delete(&store.FAQ{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleFAQSort(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	var req struct {
		SortOrder int `json:"sort_order" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := s.db.Model(&store.FAQ{}).Where("id = ?", id).Update("sort_order", req.SortOrder).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"success": true})
}