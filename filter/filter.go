package filter

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/manyminds/api2go"
	log "github.com/sirupsen/logrus"
)

var (
	simpleFiltering   = regexp.MustCompile(`filter\[(.+)\]`)
	extendedFiltering = regexp.MustCompile(`filter\[(.+)\]\[(.+)\]`)
)

// Filter is meant to store the filters of requested via api
type Filter struct {
	Filter    map[string][]string
	Exclusion bool
}

// Filters is is the collection of filters received on the api call
type Filters struct {
	filters []*Filter
}

// NewFilterSet returns an empty new filter structure
func NewFilterSet(r *api2go.Request) (f *Filters, hasFilters bool) {
	f = &Filters{}
	for key, values := range r.QueryParams {
		filter := extendedFiltering.FindStringSubmatch(key)
		if len(filter) == 0 {
			filter = simpleFiltering.FindStringSubmatch(key)
			if len(filter) != 0 {
				hasFilters = true
				exclusion := false
				if strings.HasSuffix(key, "!") {
					exclusion = true
				}
				f.Add(filter[1], values, exclusion)
				log.WithFields(log.Fields{"step": "request filter", "filter": filter, "values": values}).Debug("Dora web request with filters")
			}
		}
	}

	return f, hasFilters
}

// Add adds a new filter to the filter map
func (f *Filters) Add(name string, values []string, exclusion bool) {
	ft := &Filter{
		Filter:    map[string][]string{name: values},
		Exclusion: exclusion,
	}

	f.filters = append(f.filters, ft)
}

// Get retrieve all filters
func (f *Filters) Get() []*Filter {
	return f.filters
}

// BuildQuery receive a model as an interface and builds a query out of it
func (f *Filters) BuildQuery(m interface{}) (query string, err error) {
	for _, filter := range f.Get() {
		queryType := "in"
		if filter.Exclusion {
			queryType = "not in"
		}

		for key, values := range filter.Filter {
			if len(values) == 1 && values[0] == "" {
				continue
			}
			rfct := reflect.ValueOf(m)
			rfctType := rfct.Type()

			var structMemberName string
			var structJSONMemberName string
			for i := 0; i < rfctType.NumField(); i++ {
				jsondName := rfctType.Field(i).Tag.Get("json")
				if key == jsondName {
					structMemberName = rfctType.Field(i).Name
					structJSONMemberName = jsondName
					break
				}
			}

			if structJSONMemberName == "" || structJSONMemberName == "-" {
				return query, err
			}

			ftype := reflect.Indirect(rfct).FieldByName(structMemberName)
			switch ftype.Kind() {
			case reflect.String:
				if query == "" {
					query = fmt.Sprintf("%s %s ('%s')", structJSONMemberName, queryType, strings.Join(values, "', '"))
				} else {
					query = fmt.Sprintf("%s and %s %s ('%s')", query, structJSONMemberName, queryType, strings.Join(values, "', '"))
				}
			case reflect.Bool, reflect.Int, reflect.Float64, reflect.Float32:
				if query == "" {
					query = fmt.Sprintf("%s %s (%s)", structJSONMemberName, queryType, strings.Join(values, ", "))
				} else {
					query = fmt.Sprintf("%s and %s %s (%s)", query, structJSONMemberName, queryType, strings.Join(values, ", "))
				}
			}
		}
	}
	return query, err
}

// Clean cleanup the current filter list
func (f *Filters) Clean() {
	f.filters = make([]*Filter, 0)
}

// OffSetAndLimitParse parsers the limit and offset of the requests
func OffSetAndLimitParse(r *api2go.Request) (offset string, limit string) {
	offsetQuery, hasOffset := r.QueryParams["page[offset]"]
	limitQuery, hasLimit := r.QueryParams["page[limit]"]

	if hasOffset {
		offset = offsetQuery[0]
	}

	if hasLimit {
		limit = limitQuery[0]
	}

	if hasOffset && offset == "" {
		offset = "0"
	}

	if (hasLimit && limit == "") || (hasOffset && !hasLimit) {
		limit = "100"
	}

	return offset, limit
}
