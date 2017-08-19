package resource

import (
	"net/http"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/manyminds/api2go"
	"gitlab.booking.com/infra/dora/model"
	"gitlab.booking.com/infra/dora/storage"
)

// ChassisResource for api2go routes
type ChassisResource struct {
	ChassisStorage *storage.ChassisStorage
	BladeStorage   *storage.BladeStorage
}

// FindAll Chassis
func (c ChassisResource) FindAll(r api2go.Request) (api2go.Responder, error) {
	_, chassis, err := c.queryAndCountAllWrapper(r)
	return &Response{Res: chassis}, err
}

// FindOne Chassis
func (c ChassisResource) FindOne(ID string, r api2go.Request) (api2go.Responder, error) {
	res, err := c.ChassisStorage.GetOne(ID)
	if err == gorm.ErrRecordNotFound {
		return &Response{}, api2go.NewHTTPError(err, err.Error(), http.StatusNotFound)
	}
	return &Response{Res: res}, err
}

// PaginatedFindAll can be used to load chassis in chunks
func (c ChassisResource) PaginatedFindAll(r api2go.Request) (uint, api2go.Responder, error) {
	count, chassis, err := c.queryAndCountAllWrapper(r)
	return uint(count), &Response{Res: chassis}, err
}

func (c ChassisResource) queryAndCountAllWrapper(r api2go.Request) (count int, chassis []model.Chassis, err error) {
	for _, invalidQuery := range []string{"page[number]", "page[size]"} {
		_, invalid := r.QueryParams[invalidQuery]
		if invalid {
			return count, chassis, ErrPageSizeAndNumber
		}
	}

	filters := NewFilter()
	hasFilters := false
	var offset string
	var limit string

	include, hasInclude := r.QueryParams["include"]
	bladesID, hasBlade := r.QueryParams["bladesID"]
	offsetQuery, hasOffset := r.QueryParams["page[offset]"]
	if hasOffset {
		offset = offsetQuery[0]
	}

	limitQuery, hasLimit := r.QueryParams["page[limit]"]
	if hasLimit {
		limit = limitQuery[0]
	}

	for key, values := range r.QueryParams {
		if strings.HasPrefix(key, "filter") {
			hasFilters = true
			filter := strings.TrimRight(strings.TrimLeft(key, "filter["), "]")
			filters.Add(filter, values)
		}
	}

	if hasFilters {
		count, chassis, err = c.ChassisStorage.GetAllByFilters(offset, limit, filters.Get())
		filters.Clean()
		if err != nil {
			return count, chassis, err
		}
	}

	if hasInclude && include[0] == "blades" {
		if len(chassis) == 0 {
			count, chassis, err = c.ChassisStorage.GetAllWithAssociations(offset, limit)
		} else {
			var chassisWithInclude []model.Chassis
			for _, ch := range chassis {
				chWithInclude, err := c.ChassisStorage.GetOne(ch.Serial)
				if err != nil {
					return count, chassis, err
				}
				chassisWithInclude = append(chassisWithInclude, chWithInclude)
			}
			chassis = chassisWithInclude
		}
	}

	if hasBlade {
		count, chassis, err = c.ChassisStorage.GetAllByBladesID(offset, limit, bladesID)
	}

	if !hasFilters && !hasInclude && !hasBlade {
		count, chassis, err = c.ChassisStorage.GetAll(offset, limit)
		if err != nil {
			return count, chassis, err
		}
	}

	return count, chassis, err
}
