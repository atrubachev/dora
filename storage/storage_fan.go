package storage

import (
	"fmt"
	"strings"

	"github.com/bmc-toolbox/dora/filter"
	"github.com/bmc-toolbox/dora/model"
	"github.com/jinzhu/gorm"
	"github.com/manyminds/api2go"
)

// NewFanStorage initializes the storage
func NewFanStorage(db *gorm.DB) *FanStorage {
	return &FanStorage{db}
}

// FanStorage stores all fans used by blades
type FanStorage struct {
	db *gorm.DB
}

// Count get fans count based on the filter
func (f FanStorage) Count(filters *filter.Filters) (count int, err error) {
	q, err := filters.BuildQuery(model.Fan{}, f.db)
	if err != nil {
		return count, err
	}

	err = q.Model(&model.Fan{}).Count(&count).Error
	return count, err
}

// GetAll fans
func (f FanStorage) GetAll(offset string, limit string) (count int, fans []model.Fan, err error) {
	if offset != "" && limit != "" {
		if err = f.db.Limit(limit).Offset(offset).Order("serial").Find(&fans).Error; err != nil {
			return count, fans, err
		}
		f.db.Model(&model.Fan{}).Order("serial").Count(&count)
	} else {
		if err = f.db.Order("serial").Find(&fans).Error; err != nil {
			return count, fans, err
		}
	}
	return count, fans, err
}

// GetAllWithAssociations returns all chassis with their relationships
func (f FanStorage) GetAllWithAssociations(offset string, limit string, include []string) (count int, fans []model.Fan, err error) {
	q := f.db.Order("serial asc")
	for _, preload := range include {
		q = q.Preload(strings.Title(preload))
	}

	if offset != "" && limit != "" {
		q = f.db.Limit(limit).Offset(offset)
		f.db.Order("serial asc").Find(&model.Fan{}).Count(&count)
	}

	if err = q.Find(&fans).Error; err != nil {
		if strings.Contains(err.Error(), "can't preload field") {
			return count, fans, api2go.NewHTTPError(nil,
				fmt.Sprintf("invalid include: %s", strings.Split(err.Error(), " ")[3]), 422)
		}
		return count, fans, err
	}

	return count, fans, err
}

// GetAllByChassisID of the fans by ChassisID
func (f FanStorage) GetAllByChassisID(offset string, limit string, serials []string) (count int, fans []model.Fan, err error) {
	if offset != "" && limit != "" {
		if err = f.db.Limit(limit).Offset(offset).Where("chassis_serial in (?)", serials).Find(&fans).Error; err != nil {
			return count, fans, err
		}
		f.db.Model(&model.Fan{}).Where("chassis_serial in (?)", serials).Count(&count)
	} else {
		if err = f.db.Where("chassis_serial in (?)", serials).Find(&fans).Error; err != nil {
			return count, fans, err
		}
	}
	return count, fans, err
}

// GetAllByDiscreteID of the fans by DiscreteID
func (f FanStorage) GetAllByDiscreteID(offset string, limit string, serials []string) (count int, fans []model.Fan, err error) {
	if offset != "" && limit != "" {
		if err = f.db.Limit(limit).Offset(offset).Where("discrete_serial in (?)", serials).Find(&fans).Error; err != nil {
			return count, fans, err
		}
		f.db.Model(&model.Fan{}).Where("discrete_serial in (?)", serials).Count(&count)
	} else {
		if err = f.db.Where("discrete_serial in (?)", serials).Find(&fans).Error; err != nil {
			return count, fans, err
		}
	}
	return count, fans, err
}

// GetOne fan
func (f FanStorage) GetOne(serial string) (fan model.Fan, err error) {
	if err := f.db.Where("serial = ?", serial).First(&fan).Error; err != nil {
		return fan, err
	}
	return fan, err
}

// GetAllByFilters get all blades based on the filter
func (f FanStorage) GetAllByFilters(offset string, limit string, filters *filter.Filters) (count int, fans []model.Fan, err error) {
	q, err := filters.BuildQuery(model.Fan{}, f.db)
	if err != nil {
		return count, fans, err
	}

	if offset != "" && limit != "" {
		if err = q.Limit(limit).Offset(offset).Find(&fans).Error; err != nil {
			return count, fans, err
		}
		q.Model(&model.Fan{}).Count(&count)
	} else {
		if err = q.Find(&fans).Error; err != nil {
			return count, fans, err
		}
	}

	return count, fans, nil
}
