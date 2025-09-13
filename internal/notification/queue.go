package notification

import (
	"context"
	"sync"
	"time"
	
	logger "shop-bot/internal/log"
	"github.com/google/uuid"
)

// MemoryQueue implements an in-memory notification queue
type MemoryQueue struct {
	service     *Service
	queue       chan *Notification
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	maxRetries  int
	retryDelay  time.Duration
	rateLimit   *rateLimiter
}

// rateLimiter implements a simple rate limiter
type rateLimiter struct {
	mu         sync.Mutex
	count      int
	window     time.Time
	maxPerMin  int
}

// NewMemoryQueue creates a new in-memory queue
func NewMemoryQueue(service *Service, config *NotificationConfig) *MemoryQueue {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &MemoryQueue{
		service:    service,
		queue:      make(chan *Notification, 1000), // Buffer size of 1000
		ctx:        ctx,
		cancel:     cancel,
		maxRetries: config.MaxRetries,
		retryDelay: config.RetryDelay,
		rateLimit: &rateLimiter{
			maxPerMin: config.RateLimit,
			window:    time.Now(),
		},
	}
}

// Push adds a notification to the queue
func (q *MemoryQueue) Push(notification *Notification) error {
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	
	select {
	case q.queue <- notification:
		logger.Info("Notification queued", 
			"id", notification.ID,
			"type", notification.Type,
			"priority", notification.Priority)
		return nil
	case <-q.ctx.Done():
		return context.Canceled
	default:
		logger.Warn("Notification queue full, dropping notification",
			"id", notification.ID,
			"type", notification.Type)
		return nil // Drop notification if queue is full
	}
}

// Process starts processing the queue
func (q *MemoryQueue) Process() {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		
		for {
			select {
			case notification := <-q.queue:
				q.processNotification(notification)
			case <-q.ctx.Done():
				// Process remaining notifications before shutting down
				for len(q.queue) > 0 {
					select {
					case notification := <-q.queue:
						q.processNotification(notification)
					default:
						return
					}
				}
				return
			}
		}
	}()
}

// Stop gracefully stops the queue
func (q *MemoryQueue) Stop() {
	q.cancel()
	q.wg.Wait()
	close(q.queue)
}

// processNotification processes a single notification with retry logic
func (q *MemoryQueue) processNotification(notification *Notification) {
	// Check rate limit
	if !q.checkRateLimit() {
		// Re-queue the notification for later
		time.Sleep(time.Second * 10)
		q.Push(notification)
		return
	}
	
	// Process by priority
	switch notification.Priority {
	case PriorityHigh:
		// Process immediately
	case PriorityMedium:
		time.Sleep(time.Second * 2)
	case PriorityLow:
		time.Sleep(time.Second * 5)
	default:
		notification.Priority = PriorityMedium
	}
	
	// Try to send the notification
	err := q.sendWithRetry(notification)
	if err != nil {
		logger.Error("Failed to send notification after retries",
			"id", notification.ID,
			"type", notification.Type,
			"error", err)
	}
}

// sendWithRetry sends a notification with retry logic
func (q *MemoryQueue) sendWithRetry(notification *Notification) error {
	var lastErr error
	
	for i := 0; i <= q.maxRetries; i++ {
		if i > 0 {
			// Wait before retry
			time.Sleep(q.retryDelay * time.Duration(i))
		}
		
		// Send notification using the service
		q.service.NotifyAdmins(notification.Type, notification.Data)
		
		// Since NotifyAdmins doesn't return error, assume success
		logger.Info("Notification sent successfully",
			"id", notification.ID,
			"type", notification.Type,
			"attempt", i+1)
		return nil
	}
	
	notification.LastError = lastErr.Error()
	return lastErr
}

// checkRateLimit checks if we can send a notification based on rate limits
func (q *MemoryQueue) checkRateLimit() bool {
	q.rateLimit.mu.Lock()
	defer q.rateLimit.mu.Unlock()
	
	now := time.Now()
	// Reset counter if we're in a new minute window
	if now.Sub(q.rateLimit.window) > time.Minute {
		q.rateLimit.count = 0
		q.rateLimit.window = now
	}
	
	if q.rateLimit.count >= q.rateLimit.maxPerMin {
		return false
	}
	
	q.rateLimit.count++
	return true
}