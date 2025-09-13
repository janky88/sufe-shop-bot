package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestLogger creates a test logger
func TestLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}

// AssertJSONEqual asserts that two JSON strings are equivalent
func AssertJSONEqual(t *testing.T, expected, actual string) {
	var expectedData, actualData interface{}
	
	err := json.Unmarshal([]byte(expected), &expectedData)
	require.NoError(t, err, "Failed to unmarshal expected JSON")
	
	err = json.Unmarshal([]byte(actual), &actualData)
	require.NoError(t, err, "Failed to unmarshal actual JSON")
	
	require.Equal(t, expectedData, actualData, "JSON mismatch")
}

// CleanupDatabase removes all test data
func CleanupDatabase(db *gorm.DB) {
	tables := []string{
		"audit_logs",
		"sessions", 
		"orders",
		"products",
		"users",
		"settings",
		"templates",
		"broadcasts",
		"faqs",
	}
	
	for _, table := range tables {
		db.Exec("DELETE FROM " + table)
	}
}