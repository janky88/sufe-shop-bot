package httpadmin

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	
	"shop-bot/internal/store"
)

// handleFAQInit initializes FAQ data if empty
func (s *Server) handleFAQInit(c *gin.Context) {
	// Check if FAQs already exist
	var count int64
	s.db.Model(&store.FAQ{}).Count(&count)
	
	if count > 0 {
		c.JSON(http.StatusOK, gin.H{"message": "FAQs already exist"})
		return
	}
	
	// Insert sample FAQs
	sampleFAQs := []store.FAQ{
		// Chinese FAQs
		{
			Question:  "如何购买商品？",
			Answer:    "点击\"购买\"按钮，选择您需要的商品，然后按照提示完成支付即可。支付成功后，系统会自动发送卡密给您。",
			Language:  "zh",
			SortOrder: 10,
			IsActive:  true,
		},
		{
			Question:  "如何充值余额？",
			Answer:    "点击\"充值\"按钮，选择充值金额或输入自定义金额，完成支付后余额会自动到账。您也可以使用充值卡进行充值。",
			Language:  "zh",
			SortOrder: 20,
			IsActive:  true,
		},
		{
			Question:  "支付失败怎么办？",
			Answer:    "如果支付失败，请检查您的支付账户余额是否充足。如果问题持续存在，请联系客服人员协助处理。",
			Language:  "zh",
			SortOrder: 30,
			IsActive:  true,
		},
		{
			Question:  "如何查看我的订单？",
			Answer:    "点击\"我的订单\"按钮，即可查看您的所有订单记录，包括订单状态和已发货的卡密信息。",
			Language:  "zh",
			SortOrder: 40,
			IsActive:  true,
		},
		{
			Question:  "余额可以提现吗？",
			Answer:    "目前系统暂不支持余额提现功能，余额仅可用于购买商品。请合理安排您的充值金额。",
			Language:  "zh",
			SortOrder: 50,
			IsActive:  true,
		},
		// English FAQs
		{
			Question:  "How to buy products?",
			Answer:    "Click the \"Buy\" button, select the product you need, and follow the prompts to complete payment. After successful payment, the system will automatically send you the card code.",
			Language:  "en",
			SortOrder: 10,
			IsActive:  true,
		},
		{
			Question:  "How to recharge balance?",
			Answer:    "Click the \"Deposit\" button, select the recharge amount or enter a custom amount. Your balance will be credited automatically after payment. You can also use recharge cards.",
			Language:  "en",
			SortOrder: 20,
			IsActive:  true,
		},
		{
			Question:  "What if payment fails?",
			Answer:    "If payment fails, please check if your payment account has sufficient balance. If the problem persists, please contact customer service for assistance.",
			Language:  "en",
			SortOrder: 30,
			IsActive:  true,
		},
		{
			Question:  "How to view my orders?",
			Answer:    "Click the \"My Orders\" button to view all your order records, including order status and delivered card codes.",
			Language:  "en",
			SortOrder: 40,
			IsActive:  true,
		},
		{
			Question:  "Can I withdraw my balance?",
			Answer:    "Currently, the system does not support balance withdrawal. Balance can only be used for purchasing products. Please plan your recharge amount accordingly.",
			Language:  "en",
			SortOrder: 50,
			IsActive:  true,
		},
	}
	
	// Insert all FAQs
	if err := s.db.Create(&sampleFAQs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Sample FAQs created successfully", "count": len(sampleFAQs)})
}