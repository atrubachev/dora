package resource

import (
	"net/http"

	"github.com/jinzhu/gorm"
	"github.com/manyminds/api2go"
	"gitlab.booking.com/infra/dora/filter"
	"gitlab.booking.com/infra/dora/model"
	"gitlab.booking.com/infra/dora/storage"
)

// BladeResource for api2go routes
type BladeResource struct {
	BladeStorage   *storage.BladeStorage
	ChassisStorage *storage.ChassisStorage
	NicStorage     *storage.NicStorage
}

// FindAll Blades
func (b BladeResource) FindAll(r api2go.Request) (api2go.Responder, error) {
	_, blades, err := b.queryAndCountAllWrapper(r)
	return &Response{Res: blades}, err
}

// FindOne Blade
func (b BladeResource) FindOne(ID string, r api2go.Request) (api2go.Responder, error) {
	res, err := b.BladeStorage.GetOne(ID)
	if err == gorm.ErrRecordNotFound {
		return &Response{}, api2go.NewHTTPError(err, err.Error(), http.StatusNotFound)
	}
	return &Response{Res: res}, err
}

// PaginatedFindAll can be used to load blades in chunks
func (b BladeResource) PaginatedFindAll(r api2go.Request) (uint, api2go.Responder, error) {
	count, blades, err := b.queryAndCountAllWrapper(r)
	return uint(count), &Response{Res: blades}, err
}

// queryAndCountAllWrapper retrieve the data to be used for FindAll and PaginatedFindAll in a stardard way
func (b BladeResource) queryAndCountAllWrapper(r api2go.Request) (count int, blades []model.Blade, err error) {
	for _, invalidQuery := range []string{"page[number]", "page[size]"} {
		_, invalid := r.QueryParams[invalidQuery]
		if invalid {
			return count, blades, ErrPageSizeAndNumber
		}
	}

	filters, hasFilters := filter.NewFilterSet(&r.QueryParams)
	offset, limit := filter.OffSetAndLimitParse(&r)

	if hasFilters {
		count, blades, err = b.BladeStorage.GetAllByFilters(offset, limit, filters)
		filters.Clean()
		if err != nil {
			return count, blades, err
		}
	}

	include, hasInclude := r.QueryParams["include"]
	if hasInclude && include[0] == "nics" {
		if len(blades) == 0 {
			count, blades, err = b.BladeStorage.GetAllWithAssociations(offset, limit)
		} else {
			var bladesWithInclude []model.Blade
			for _, bl := range blades {
				blWithInclude, err := b.BladeStorage.GetOne(bl.Serial)
				if err != nil {
					return count, blades, err
				}
				bladesWithInclude = append(bladesWithInclude, blWithInclude)
			}
			blades = bladesWithInclude
		}
	}

	chassisID, hasChassis := r.QueryParams["chassisID"]
	if hasChassis {
		count, blades, err = b.BladeStorage.GetAllByChassisID(offset, limit, chassisID)
		if err != nil {
			return count, blades, err
		}
	}

	nicsID, hasNIC := r.QueryParams["nicsID"]
	if hasNIC {
		count, blades, err = b.BladeStorage.GetAllByNicsID(offset, limit, nicsID)
		if err != nil {
			return count, blades, err
		}
	}

	if !hasFilters && !hasChassis && !hasInclude && !hasNIC {
		count, blades, err = b.BladeStorage.GetAll(offset, limit)
		if err != nil {
			return count, blades, err
		}
	}

	return count, blades, err
}
