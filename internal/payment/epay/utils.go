package epay

import (
	"strings"
)

// DetectDeviceType detects device type from User-Agent string
func DetectDeviceType(userAgent string) DeviceType {
	ua := strings.ToLower(userAgent)
	
	// Check for specific apps first
	if strings.Contains(ua, "micromessenger") {
		return DeviceWechat
	}
	if strings.Contains(ua, "qq/") && !strings.Contains(ua, "qqbrowser") {
		return DeviceQQ
	}
	if strings.Contains(ua, "alipayclient") {
		return DeviceAlipay
	}
	
	// Check for mobile devices
	mobileKeywords := []string{
		"mobile", "android", "iphone", "ipad", "ipod", 
		"blackberry", "windows phone", "opera mini",
	}
	
	for _, keyword := range mobileKeywords {
		if strings.Contains(ua, keyword) {
			return DeviceMobile
		}
	}
	
	// Default to PC
	return DevicePC
}

// GetRecommendedPaymentType suggests payment type based on device
func GetRecommendedPaymentType(device DeviceType) PaymentType {
	switch device {
	case DeviceWechat:
		return PaymentWechat
	case DeviceQQ:
		return PaymentQQ
	case DeviceAlipay:
		return PaymentAlipay
	default:
		// Default to Alipay for PC and mobile browsers
		return PaymentAlipay
	}
}