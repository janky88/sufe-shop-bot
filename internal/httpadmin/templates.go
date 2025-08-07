package httpadmin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"shop-bot/internal/store"
	logger "shop-bot/internal/log"
)

func (s *Server) handleTemplateList(c *gin.Context) {
	// Get all templates
	templates, err := store.GetAllTemplates(s.db)
	if err != nil {
		logger.Error("Failed to fetch templates", "error", err)
		c.String(http.StatusInternalServerError, "Database error")
		return
	}
	
	// Group by code
	templateMap := make(map[string][]store.MessageTemplate)
	for _, tmpl := range templates {
		templateMap[tmpl.Code] = append(templateMap[tmpl.Code], tmpl)
	}
	
	c.HTML(http.StatusOK, "templates.html", gin.H{
		"templateMap": templateMap,
		"variables": map[string][]string{
			"order_paid":        {"OrderID", "ProductName", "Code"},
			"no_stock":         {"OrderID", "ProductName"},
			"balance_recharged": {"Amount", "NewBalance", "CardCode"},
			"order_created":    {"ProductName", "Price", "OrderID"},
			"profile_info":     {"UserID", "Username", "Language", "JoinedDate", "Balance"},
		},
	})
}

func (s *Server) handleTemplateUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	
	var req struct {
		Content  string `json:"content" form:"content"`
		IsActive bool   `json:"is_active" form:"is_active"`
	}
	
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Get template to check variables
	var tmpl store.MessageTemplate
	if err := s.db.First(&tmpl, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}
	
	// Validate template content
	vars := store.GetTemplateVariables(tmpl.Code)
	if err := store.ValidateTemplateVariables(req.Content, vars); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template validation failed: " + err.Error()})
		return
	}
	
	// Update template
	if err := store.UpdateMessageTemplate(s.db, uint(id), req.Content, req.IsActive); err != nil {
		logger.Error("Failed to update template", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Template updated successfully"})
}