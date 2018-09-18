package idrac9

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/bmc-toolbox/bmclib/cfgresources"
	"github.com/bmc-toolbox/bmclib/internal/helper"

	log "github.com/sirupsen/logrus"
	"gopkg.in/go-playground/validator.v9"
)

func (i *IDrac9) ApplyCfg(config *cfgresources.ResourcesConfig) (err error) {

	if config == nil {
		msg := "No config to apply."
		log.WithFields(log.Fields{
			"step": "ApplyCfg",
		}).Warn(msg)
		return errors.New(msg)
	}

	cfg := reflect.ValueOf(config).Elem()

	//Each Field in ResourcesConfig struct is a ptr to a resource,
	//Here we figure the resources to be configured, i.e the ptr is not nil
	for r := 0; r < cfg.NumField(); r++ {
		resourceName := cfg.Type().Field(r).Name
		if cfg.Field(r).Pointer() != 0 {
			switch resourceName {
			case "User":
				//retrieve users resource values as an interface
				userAccounts := cfg.Field(r).Interface()

				//assert userAccounts interface to its actual type - A slice of ptrs to User
				err := i.applyUserParams(userAccounts.([]*cfgresources.User))
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "ApplyCfg",
						"resource": cfg.Field(r).Kind(),
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
						"Error":    err,
					}).Warn("Unable to apply User config.")
				}
			case "Syslog":
				syslogCfg := cfg.Field(r).Interface().(*cfgresources.Syslog)
				err := i.applySyslogParams(syslogCfg)
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "ApplyCfg",
						"resource": cfg.Field(r).Kind(),
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
						"Error":    err,
					}).Warn("Unable to set Syslog config.")
				}
			case "Network":
				networkCfg := cfg.Field(r).Interface().(*cfgresources.Network)
				err := i.applyNetworkParams(networkCfg)
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "ApplyCfg",
						"resource": cfg.Field(r).Kind(),
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
						"Error":    err,
					}).Warn("Unable to set Network config params.")
				}
			case "Ntp":
				ntpCfg := cfg.Field(r).Interface().(*cfgresources.Ntp)
				err := i.applyNtpParams(ntpCfg)
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "ApplyCfg",
						"resource": cfg.Field(r).Kind(),
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
					}).Warn("Unable to set NTP config.")
				}
			case "Ldap":
				ldapCfg := cfg.Field(r).Interface().(*cfgresources.Ldap)
				err := i.applyLdapServerParams(ldapCfg)
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "applyLdapParams",
						"resource": "Ldap",
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
						"Error":    err,
					}).Warn("applyLdapServerParams returned error.")
				}
			case "LdapGroup":
				ldapGroupCfg := cfg.Field(r).Interface().([]*cfgresources.LdapGroup)
				err := i.applyLdapGroupParams(ldapGroupCfg)
				if err != nil {
					log.WithFields(log.Fields{
						"step":     "applyLdapParams",
						"resource": "Ldap",
						"IP":       i.ip,
						"Model":    i.BmcType(),
						"Serial":   i.Serial,
						"Error":    err,
					}).Warn("applyLdapGroupParams returned error.")
				}
			case "Ssl":
			case "Dell":
				biosCfg := cfg.Field(r).Interface().(*cfgresources.Dell)
				if biosCfg.Idrac9BiosSettings != nil {
					err := i.applyBiosParams(biosCfg.Idrac9BiosSettings)
					if err != nil {
						log.WithFields(log.Fields{
							"step":     "applyBiosCfg",
							"resource": "Bios",
							"IP":       i.ip,
							"Model":    i.BmcType(),
							"Serial":   i.Serial,
							"Error":    err,
						}).Warn("applyBiosParams returned error.")
					}
				}
			default:
				log.WithFields(log.Fields{
					"step": "ApplyCfg",
				}).Warn("Unknown resource.")
			}
		}
	}

	return err
}

// Applies Bios params
func (i *IDrac9) applyBiosParams(newBiosSettings *cfgresources.Idrac9BiosSettings) (err error) {

	//The bios settings that will be applied
	toApplyBiosSettings := &BiosSettings{}

	//validate config
	validate := validator.New()
	err = validate.Struct(newBiosSettings)
	if err != nil {
		log.WithFields(log.Fields{
			"step":  "applyBiosParams",
			"Error": err,
		}).Fatal("Config validation failed.")
		return err
	}

	//GET current settings
	currentBiosSettings, err := i.getBiosSettings()
	if err != nil {
		msg := "Unable to get current bios settings through redfish."
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn(msg)
		return errors.New(msg)
	}

	//Compare current bios settings with our declared config.
	if *newBiosSettings != *currentBiosSettings {

		//retrieve fields that is the config to be applied
		toApplyBiosSettings, err = diffBiosSettings(newBiosSettings, currentBiosSettings)
		if err != nil {
			log.WithFields(log.Fields{
				"IP":     i.ip,
				"Model":  i.BmcType(),
				"Serial": i.Serial,
				"step":   helper.WhosCalling(),
				"Error":  err,
			}).Fatal("diffBiosSettings returned error.")
		}

		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Changes (Ignore empty fields)": fmt.Sprintf("%+v", toApplyBiosSettings),
		}).Info("Bios configuration to be applied.")

		//purge any existing pending bios setting jobs
		//or we will not be able to set any params
		err = i.purgeJobsForBiosSettings()
		if err != nil {
			log.WithFields(log.Fields{
				"step":                  "applyBiosParams",
				"resource":              "Bios",
				"IP":                    i.ip,
				"Model":                 i.BmcType(),
				"Serial":                i.Serial,
				"Bios settings pending": err,
			}).Warn("Unable to purge pending bios setting jobs.")
		}

		err = i.setBiosSettings(toApplyBiosSettings)
		if err != nil {
			msg := "setBiosAttributes returned error."
			log.WithFields(log.Fields{
				"IP":     i.ip,
				"Model":  i.BmcType(),
				"Serial": i.Serial,
				"step":   helper.WhosCalling(),
				"Error":  err,
			}).Warn(msg)
			return errors.New(msg)
		}

		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
		}).Info("Bios configuration update job queued in iDrac.")

	} else {

		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
		}).Info("Bios configuration is up to date.")
	}

	return err
}

// Iterates over iDrac users and adds/removes/modifies the user account
func (i *IDrac9) applyUserParams(cfgUsers []*cfgresources.User) (err error) {

	err = i.validateCfg(cfgUsers)
	if err != nil {
		msg := "Config validation failed."
		log.WithFields(log.Fields{
			"step":   "applyUserParams",
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"Error":  err,
		}).Warn(msg)
		return errors.New(msg)
	}

	idracUsers, err := i.queryUsers()
	if err != nil {
		msg := "Unable to query existing users"
		log.WithFields(log.Fields{
			"step":   "applyUserParams",
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"Error":  err,
		}).Warn(msg)
		return errors.New(msg)
	}

	//for each configuration user
	for _, cfgUser := range cfgUsers {

		userId, userInfo, uExists := userInIdrac(cfgUser.Name, idracUsers)

		//user to be added/updated
		if cfgUser.Enable {

			//new user to be added
			if uExists == false {
				userId, userInfo, err = getEmptyUserSlot(idracUsers)
				if err != nil {
					log.WithFields(log.Fields{
						"IP":     i.ip,
						"Model":  i.BmcType(),
						"Serial": i.Serial,
						"step":   helper.WhosCalling(),
						"User":   cfgUser.Name,
						"Error":  err,
					}).Warn("Unable to add new User.")
					continue
				}
			}

			userInfo.Enable = "Enabled"
			userInfo.SolEnable = "Enabled"
			userInfo.UserName = cfgUser.Name
			userInfo.Password = cfgUser.Password

			//set appropriate privileges
			if cfgUser.Role == "admin" {
				userInfo.Privilege = "511"
				userInfo.IpmiLanPrivilege = "Administrator"
			} else {
				userInfo.Privilege = "499"
				userInfo.IpmiLanPrivilege = "Operator"
			}

			err = i.putUser(userId, userInfo)
			if err != nil {
				log.WithFields(log.Fields{
					"IP":     i.ip,
					"Model":  i.BmcType(),
					"Serial": i.Serial,
					"step":   helper.WhosCalling(),
					"User":   cfgUser.Name,
					"Error":  err,
				}).Warn("Add/Update user request failed.")
				continue
			}

		} // end if cfgUser.Enable

		//if the user exists but is disabled in our config, remove the user
		if cfgUser.Enable == false && uExists == true {
			endpoint := fmt.Sprintf("sysmgmt/2017/server/user?userid=%d", userId)
			statusCode, response, err := i.delete_(endpoint)
			if err != nil {
				log.WithFields(log.Fields{
					"IP":         i.ip,
					"Model":      i.BmcType(),
					"Serial":     i.Serial,
					"step":       helper.WhosCalling(),
					"User":       cfgUser.Name,
					"Error":      err,
					"StatusCode": statusCode,
					"Response":   response,
				}).Warn("Delete user request failed.")
				continue
			}
		}

		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"User":   cfgUser.Name,
		}).Debug("User parameters applied.")

	}

	return err
}

// Applies LDAP server params
func (i *IDrac9) applyLdapServerParams(cfg *cfgresources.Ldap) (err error) {
	params := map[string]string{
		"Enable":               "Disabled",
		"Port":                 "636",
		"UserAttribute":        "uid",
		"GroupAttribute":       "memberUid",
		"GroupAttributeIsDN":   "Enabled",
		"CertValidationEnable": "Disabled",
		"SearchFilter":         "objectClass=posixAccount",
	}

	if cfg.Server == "" {
		msg := "Ldap resource parameter Server required but not declared."
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling,
		}).Warn(msg)
		return errors.New(msg)
	}

	if cfg.BaseDn == "" {
		msg := "Ldap resource parameter BaseDn required but not declared."
		log.WithFields(log.Fields{
			"Model": i.BmcType(),
			"step":  helper.WhosCalling,
		}).Warn(msg)
		return errors.New(msg)
	}

	if cfg.Enable {
		params["Enable"] = "Enabled"
	}

	if cfg.Port == 0 {
		params["Port"] = string(cfg.Port)
	}

	if cfg.UserAttribute != "" {
		params["UserAttribute"] = cfg.UserAttribute
	}

	if cfg.GroupAttribute != "" {
		params["GroupAttribute"] = cfg.GroupAttribute
	}

	if cfg.SearchFilter != "" {
		params["SearchFilter"] = cfg.SearchFilter
	}

	payload := Ldap{
		BaseDN:               cfg.BaseDn,
		BindDN:               cfg.BindDn,
		CertValidationEnable: params["CertValidationEnable"],
		Enable:               params["Enable"],
		GroupAttribute:       params["GroupAttribute"],
		GroupAttributeIsDN:   params["GroupAttributeIsDN"],
		Port:                 params["Port"],
		SearchFilter:         params["SearchFilter"],
		Server:               cfg.Server,
		UserAttribute:        params["UserAttribute"],
	}

	err = i.putLdap(payload)
	if err != nil {
		msg := "Ldap params PUT request failed."
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn(msg)
		return errors.New("Ldap params put request failed.")
	}

	return err
}

// Iterates over iDrac Ldap role groups and adds/removes/modifies ldap role groups
func (i *IDrac9) applyLdapGroupParams(cfg []*cfgresources.LdapGroup) (err error) {

	idracLdapRoleGroups, err := i.queryLdapRoleGroups()
	if err != nil {
		msg := "Unable to query existing users"
		log.WithFields(log.Fields{
			"step":   "applyUserParams",
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"Error":  err,
		}).Warn(msg)
		return errors.New(msg)
	}

	//for each configuration ldap role group
	for _, cfgRole := range cfg {
		roleId, role, rExists := ldapRoleGroupInIdrac(cfgRole.Group, idracLdapRoleGroups)

		//role to be added/updated
		if cfgRole.Enable {

			//new role to be added
			if rExists == false {
				roleId, role, err = getEmptyLdapRoleGroupSlot(idracLdapRoleGroups)
				if err != nil {
					log.WithFields(log.Fields{
						"IP":              i.ip,
						"Model":           i.BmcType(),
						"Serial":          i.Serial,
						"step":            helper.WhosCalling(),
						"Ldap Role Group": cfgRole.Group,
						"Role Group DN":   cfgRole.Role,
						"Error":           err,
					}).Warn("Unable to add new Ldap Role Group.")
					continue
				}
			}

			role.DN = cfgRole.Group

			//set appropriate privileges
			if cfgRole.Role == "admin" {
				role.Privilege = "511"
			} else {
				role.Privilege = "499"
			}

			err = i.putLdapRoleGroup(roleId, role)
			if err != nil {
				log.WithFields(log.Fields{
					"IP":              i.ip,
					"Model":           i.BmcType(),
					"Serial":          i.Serial,
					"step":            helper.WhosCalling(),
					"Ldap Role Group": cfgRole.Group,
					"Role Group DN":   cfgRole.Role,
					"Error":           err,
				}).Warn("Add/Update LDAP Role Group request failed.")
				continue
			}

		} // end if cfgUser.Enable

		//if the role exists but is disabled in our config, remove the role
		if cfgRole.Enable == false && rExists == true {

			role.DN = ""
			role.Privilege = "0"
			err = i.putLdapRoleGroup(roleId, role)
			if err != nil {
				log.WithFields(log.Fields{
					"IP":              i.ip,
					"Model":           i.BmcType(),
					"Serial":          i.Serial,
					"step":            helper.WhosCalling(),
					"Ldap Role Group": cfgRole.Group,
					"Role Group DN":   cfgRole.Role,
					"Error":           err,
				}).Warn("Remove LDAP Role Group request failed.")
				continue
			}
		}

		log.WithFields(log.Fields{
			"IP":              i.ip,
			"Model":           i.BmcType(),
			"Serial":          i.Serial,
			"Step":            helper.WhosCalling(),
			"Ldap Role Group": cfgRole.Role,
			"Role Group DN":   cfgRole.Role,
		}).Debug("Ldap Role Group parameters applied.")

	}

	return err
}

func (i *IDrac9) applyNtpParams(cfg *cfgresources.Ntp) (err error) {

	var enable string

	if cfg.Enable {
		enable = "Enabled"
	} else {
		enable = "Disabled"
	}

	if cfg.Server1 == "" {
		msg := "NTP resource expects parameter: server1."
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"Step":   helper.WhosCalling(),
		}).Warn(msg)
		return errors.New(msg)
	}

	if cfg.Timezone == "" {
		msg := "NTP resource expects parameter: timezone."
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"Step":   helper.WhosCalling(),
		}).Warn(msg)
		return errors.New(msg)
	}

	_, validTimezone := Timezones[cfg.Timezone]
	if !validTimezone {
		msg := "NTP resource a valid timezone parameter, for valid timezones see dell/idrac9/model.go"
		log.WithFields(log.Fields{
			"IP":               i.ip,
			"Model":            i.BmcType(),
			"Serial":           i.Serial,
			"step":             helper.WhosCalling(),
			"Unknown Timezone": cfg.Timezone,
		}).Warn(msg)
		return errors.New(msg)
	}

	err = i.putTimezone(Timezone{Timezone: cfg.Timezone})
	if err != nil {
		log.WithFields(log.Fields{
			"IP":       i.ip,
			"Model":    i.BmcType(),
			"Serial":   i.Serial,
			"step":     helper.WhosCalling(),
			"Timezone": cfg.Timezone,
			"Error":    err,
		}).Warn("PUT timezone request failed.")
		return err
	}

	payload := NtpConfig{
		Enable: enable,
		NTP1:   cfg.Server1,
		NTP2:   cfg.Server2,
		NTP3:   cfg.Server3,
	}

	err = i.putNtpConfig(payload)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT Ntp  request failed.")
		return err
	}

	log.WithFields(log.Fields{
		"IP":     i.ip,
		"Model":  i.BmcType(),
		"Serial": i.Serial,
	}).Debug("NTP servers param applied.")

	return err
}

func (i *IDrac9) applySyslogParams(cfg *cfgresources.Syslog) (err error) {

	var port int
	enable := "Enabled"

	if cfg.Server == "" {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
		}).Warn("Syslog resource expects parameter: Server.")
		return
	}

	if cfg.Port == 0 {
		log.WithFields(log.Fields{
			"step": helper.WhosCalling(),
		}).Debug("Syslog resource port set to default: 514.")
		port = 514
	} else {
		port = cfg.Port
	}

	if cfg.Enable != true {
		enable = "Disabled"
		log.WithFields(log.Fields{
			"step": helper.WhosCalling(),
		}).Debug("Syslog resource declared with enable: false.")
	}

	payload := Syslog{
		Port:    strconv.Itoa(port),
		Server1: cfg.Server,
		Server2: "",
		Server3: "",
		Enable:  enable,
	}
	err = i.putSyslog(payload)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT Syslog request failed.")
		return err
	}

	log.WithFields(log.Fields{
		"IP":     i.ip,
		"Model":  i.BmcType(),
		"Serial": i.Serial,
	}).Debug("Syslog parameters applied.")
	return err
}

func (i *IDrac9) applyNetworkParams(cfg *cfgresources.Network) (err error) {

	params := map[string]string{
		"EnableIPv4":              "Enabled",
		"DHCPEnable":              "Enabled",
		"DNSFromDHCP":             "Enabled",
		"EnableSerialOverLan":     "Enabled",
		"EnableSerialRedirection": "Enabled",
		"EnableIpmiOverLan":       "Enabled",
	}

	if cfg.DNSFromDHCP == false {
		params["DNSFromDHCP"] = "Disabled"
	}

	if cfg.SolEnable == false {
		params["EnableSerialOverLan"] = "Disabled"
		params["EnableSerialRedirection"] = "Disabled"
	}

	if cfg.IpmiEnable == false {
		params["EnableIpmiOverLan"] = "Disabled"
	}

	ipv4 := Ipv4{
		Enable:      params["EnableIPv4"],
		DHCPEnable:  params["DHCPEnable"],
		DNSFromDHCP: params["DNSFromDHCP"],
	}

	serialOverLan := SerialOverLan{
		Enable:       params["EnableSerialOverLan"],
		BaudRate:     "115200",
		MinPrivilege: "Administrator",
	}

	serialRedirection := SerialRedirection{
		Enable:  params["EnableSerialRedirection"],
		QuitKey: "^\\",
	}

	ipmiOverLan := IpmiOverLan{
		Enable:        params["EnableIpmiOverLan"],
		PrivLimit:     "Administrator",
		EncryptionKey: "0000000000000000000000000000000000000000",
	}

	err = i.putIPv4(ipv4)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT IPv4 request failed.")
	}

	err = i.putSerialOverLan(serialOverLan)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT SerialOverLan request failed.")
	}

	err = i.putSerialRedirection(serialRedirection)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT SerialRedirection request failed.")
	}

	err = i.putIpmiOverLan(ipmiOverLan)
	if err != nil {
		log.WithFields(log.Fields{
			"IP":     i.ip,
			"Model":  i.BmcType(),
			"Serial": i.Serial,
			"step":   helper.WhosCalling(),
			"Error":  err,
		}).Warn("PUT IpmiOverLan request failed.")
	}

	log.WithFields(log.Fields{
		"IP":     i.ip,
		"Model":  i.BmcType(),
		"Serial": i.Serial,
	}).Debug("Network config parameters applied.")
	return err
}
