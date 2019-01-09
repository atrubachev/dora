package idrac8

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bmc-toolbox/bmclib/internal/helper"
	log "github.com/sirupsen/logrus"
)

// Grab screen preview.
func (i *IDrac8) Screenshot() (response []byte, extension string, err error) {

	endpoint1 := fmt.Sprintf("data?get=consolepreview[auto%%20%d]",
		time.Now().UnixNano()/int64(time.Millisecond))

	extension = "png"

	// here we expect an empty response
	response, err = i.get(endpoint1, &map[string]string{"idracAutoRefresh": "1"})
	if err != nil {
		return []byte{}, extension, err
	}

	if !strings.Contains(string(response), "<status>ok</status>") {
		return []byte{}, extension, fmt.Errorf(string(response))
	}

	endpoint2 := fmt.Sprintf("capconsole/scapture0.png?%d",
		time.Now().UnixNano()/int64(time.Millisecond))

	response, err = i.get(endpoint2, &map[string]string{})
	if err != nil {
		return []byte{}, extension, err
	}

	return response, extension, err
}

//Queries Idrac8 for current user accounts
func (i *IDrac8) queryUsers() (userInfo UserInfo, err error) {

	userInfo = make(UserInfo)

	endpoint := "data?get=user"

	response, err := i.get(endpoint, &map[string]string{})
	if err != nil {
		log.WithFields(log.Fields{
			"IP":       i.ip,
			"Model":    i.BmcType(),
			"endpoint": endpoint,
			"step":     helper.WhosCalling(),
			"Error":    err,
		}).Warn("GET request failed.")
		return userInfo, err
	}

	xmlData := XmlRoot{}
	err = xml.Unmarshal(response, &xmlData)
	if err != nil {
		log.WithFields(log.Fields{
			"step":     "queryUserInfo",
			"resource": "User",
			"IP":       i.ip,
			"Model":    i.BmcType(),
			"Error":    err,
		}).Warn("Unable to unmarshal payload.")
		return userInfo, err
	}

	for _, userAccount := range xmlData.XmlUserAccount {

		user := User{
			UserName:  userAccount.Name,
			Privilege: strconv.Itoa(userAccount.Privileges),
		}

		switch userAccount.Privileges {
		case 511:
			user.IpmiLanPrivilege = "Administrator"
		case 499:
			user.IpmiLanPrivilege = "Operator"
		}

		if userAccount.SolEnabled == 1 {
			user.SolEnable = "Enabled"
		} else {
			user.SolEnable = "disabled"
		}

		if userAccount.Enabled == 1 {
			user.Enable = "Enabled"
		} else {
			user.Enable = "disabled"
		}

		userInfo[userAccount.Id] = user
	}

	return userInfo, err
}
