package model

import (
	"time"

	"github.com/manyminds/api2go/jsonapi"
)

/* READ THIS BEFORE CHANGING THE SCHEMA

To make the magic of dynamic filtering work, we need to define each json field matching the database collumn name

*/

// Nic contains the network information of the cards attached to blades or chassis
type Nic struct {
	MacAddress  string    `json:"mac_address" gorm:"primary_key"`
	Name        string    `json:"name"`
	UpdatedAt   time.Time `json:"updated_at"`
	BladeSerial string    `json:"-"`
}

// GetID to satisfy jsonapi.MarshalIdentifier interface
func (n Nic) GetID() string {
	return n.MacAddress
}

// GetReferences to satisfy the jsonapi.MarshalReferences interface
func (n Nic) GetReferences() []jsonapi.Reference {
	return []jsonapi.Reference{
		{
			Type:         "blades",
			Name:         "blades",
			Relationship: jsonapi.ToOneRelationship,
		},
	}
}

// GetReferencedIDs to satisfy the jsonapi.MarshalLinkedRelations interface
func (n Nic) GetReferencedIDs() []jsonapi.ReferenceID {
	return []jsonapi.ReferenceID{
		{
			ID:           n.BladeSerial,
			Type:         "blades",
			Name:         "blades",
			Relationship: jsonapi.ToOneRelationship,
		},
	}
}
