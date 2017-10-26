package storage

import (
	"github.com/jinzhu/gorm"
	"gitlab.booking.com/infra/dora/model"
)

// NewNicStorage initializes the storage
func NewNicStorage(db *gorm.DB) *NicStorage {
	return &NicStorage{db}
}

// NicStorage stores all nics used by blades
type NicStorage struct {
	db *gorm.DB
}

// GetAll nics
func (n NicStorage) GetAll(offset string, limit string) (count int, nics []model.Nic, err error) {
	if offset != "" && limit != "" {
		if err = n.db.Limit(limit).Offset(offset).Order("mac_address").Find(&nics).Error; err != nil {
			return count, nics, err
		}
		n.db.Model(&model.Nic{}).Order("mac_address").Count(&count)
	} else {
		if err = n.db.Order("mac_address").Find(&nics).Error; err != nil {
			return count, nics, err
		}
	}
	return count, nics, err
}

// GetAllByBladeID of the nics by BladeID
func (n NicStorage) GetAllByBladeID(offset string, limit string, serials []string) (count int, nics []model.Nic, err error) {
	if offset != "" && limit != "" {
		if err = n.db.Limit(limit).Offset(offset).Where("blade_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
		n.db.Model(&model.Nic{}).Where("blade_serial in (?)", serials).Count(&count)
	} else {
		if err = n.db.Where("blade_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
	}
	return count, nics, err
}

// GetAllByChassisID of the nics by ChassisID
func (n NicStorage) GetAllByChassisID(offset string, limit string, serials []string) (count int, nics []model.Nic, err error) {
	if offset != "" && limit != "" {
		if err = n.db.Limit(limit).Offset(offset).Where("chassis_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
		n.db.Model(&model.Nic{}).Where("chassis_serial in (?)", serials).Count(&count)
	} else {
		if err = n.db.Where("chassis_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
	}
	return count, nics, err
}

// GetAllByDiscreteID of the nics by DiscreteID
func (n NicStorage) GetAllByDiscreteID(offset string, limit string, serials []string) (count int, nics []model.Nic, err error) {
	if offset != "" && limit != "" {
		if err = n.db.Limit(limit).Offset(offset).Where("discrete_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
		n.db.Model(&model.Nic{}).Where("discrete_serial in (?)", serials).Count(&count)
	} else {
		if err = n.db.Where("discrete_serial in (?)", serials).Find(&nics).Error; err != nil {
			return count, nics, err
		}
	}
	return count, nics, err
}

// GetOne nic
func (n NicStorage) GetOne(macAddress string) (nic model.Nic, err error) {
	if err := n.db.Where("mac_address = ?", macAddress).First(&nic).Error; err != nil {
		return nic, err
	}
	return nic, err
}
