package repository

import (
	"booking-service/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WaitlistRepository interface {
	// Add inserts a new waiting entry. The DB unique index on (event_id, uid)
	// prevents duplicate waitlist registrations.
	Add(entry *model.WaitlistEntry) error

	// ExistsByEventAndUser reports whether the user is already on the waitlist.
	ExistsByEventAndUser(eventID int64, uid uuid.UUID) (bool, error)

	// CountWaiting returns how many users are ahead of position 0 (i.e. total
	// waiting entries) for position assignment at insertion time.
	CountWaiting(eventID int64) (int32, error)

	// GetPositionByUser returns the 1-based position of the user in the waitlist
	// (number of older 'waiting' entries + 1).
	GetPositionByUser(eventID int64, uid uuid.UUID) (int32, error)
}

type waitlistRepository struct {
	db *gorm.DB
}

func NewWaitlistRepository(db *gorm.DB) WaitlistRepository {
	return &waitlistRepository{db: db}
}

func (r *waitlistRepository) Add(entry *model.WaitlistEntry) error {
	return r.db.Create(entry).Error
}

func (r *waitlistRepository) ExistsByEventAndUser(eventID int64, uid uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.WaitlistEntry{}).
		Where("event_id = ? AND uid = ? AND status = ?", eventID, uid, model.WaitlistStatusWaiting).
		Count(&count).Error
	return count > 0, err
}

func (r *waitlistRepository) CountWaiting(eventID int64) (int32, error) {
	var count int64
	err := r.db.Model(&model.WaitlistEntry{}).
		Where("event_id = ? AND status = ?", eventID, model.WaitlistStatusWaiting).
		Count(&count).Error
	return int32(count), err
}

func (r *waitlistRepository) GetPositionByUser(eventID int64, uid uuid.UUID) (int32, error) {
	// Position = number of waiting entries older than this user's entry + 1.
	var entry model.WaitlistEntry
	if err := r.db.Where("event_id = ? AND uid = ?", eventID, uid).
		First(&entry).Error; err != nil {
		return 0, err
	}

	var ahead int64
	err := r.db.Model(&model.WaitlistEntry{}).
		Where("event_id = ? AND status = ? AND created_at < ?", eventID, model.WaitlistStatusWaiting, entry.CreatedAt).
		Count(&ahead).Error
	return int32(ahead) + 1, err
}
