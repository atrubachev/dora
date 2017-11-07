package connectors

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// DellCMC is the entry of the json exposed by dell
// We don't need to use an maps[string] with DellChassis, because we don't have clusters
type DellCMC struct {
	DellChassis *DellChassis `json:"0"`
}

// DellCMCTemp is the entry of the json exposed by dell when reading the temp metrics
type DellCMCTemp struct {
	DellChassisTemp *DellChassisTemp `json:"1"`
}

// DellChassisTemp is where the chassis thermal data is kept
type DellChassisTemp struct {
	TempHealth                 int    `json:"TempHealth"`
	TempUpperCriticalThreshold int    `json:"TempUpperCriticalThreshold"`
	TempSensorID               int    `json:"TempSensorID"`
	TempCurrentValue           int    `json:"TempCurrentValue"`
	TempLowerCriticalThreshold int    `json:"TempLowerCriticalThreshold"`
	TempPresence               int    `json:"TempPresence"`
	TempSensorName             string `json:"TempSensorName"`
}

// DellChassis groups all the interresting stuff we will ready from the chassis
type DellChassis struct {
	DellChassisGroupMemberHealthBlob *DellChassisGroupMemberHealthBlob `json:"ChassisGroupMemberHealthBlob"`
}

// DellChassisGroupMemberHealthBlob has a collection of metrics from the chassis, psu and blades
type DellChassisGroupMemberHealthBlob struct {
	DellBlades        map[string]*DellBlade `json:"blades_status"`
	DellPsuStatus     *DellPsuStatus        `json:"psu_status"`
	DellChassisStatus *DellChassisStatus    `json:"chassis_status"`
	DellCMCStatus     *DellCMCStatus        `json:"cmc_status"`
	// TODO: active_alerts
}

// DellChassisStatus expose the basic information that identify the chassis
type DellChassisStatus struct {
	ROCmcFwVersionString string `json:"RO_cmc_fw_version_string"`
	ROChassisServiceTag  string `json:"RO_chassis_service_tag"`
	ROChassisProductname string `json:"RO_chassis_productname"`
	CHASSISName          string `json:"CHASSIS_name"`
}

// DellCMCStatus brings the information about the cmc status itself we will use it to know if the chassis has errors
type DellCMCStatus struct {
	CMCActiveError string `json:"cmcActiveError"`
}

// DellNic is the nic we have on a servers
type DellNic struct {
	BladeNicName string `json:"bladeNicName"`
	BladeNicVer  string `json:"bladeNicVer"`
}

// DellBlade contains all the blade information
type DellBlade struct {
	BladeTemperature    string              `json:"bladeTemperature"`
	BladePresent        int                 `json:"bladePresent"`
	IdracURL            string              `json:"idracURL"`
	BladeLogDescription string              `json:"bladeLogDescription"`
	StorageNumDrives    int                 `json:"storageNumDrives"`
	BladeCPUInfo        string              `json:"bladeCpuInfo"`
	Nics                map[string]*DellNic `json:"nic"`
	BladeMasterSlot     int                 `json:"bladeMasterSlot"`
	BladeUSCVer         string              `json:"bladeUSCVer"`
	BladeSvcTag         string              `json:"bladeSvcTag"`
	BladeBIOSver        string              `json:"bladeBIOSver"`
	ActualPwrConsump    int                 `json:"actualPwrConsump"`
	IsStorageBlade      int                 `json:"isStorageBlade"`
	BladeModel          string              `json:"bladeModel"`
	BladeName           string              `json:"bladeName"`
	BladeSerialNum      string              `json:"bladeSerialNum"`
}

// DellPsuStatus contains the information and power usage of the pdus
type DellPsuStatus struct {
	AcPower  string        `json:"acPower"`
	PsuCount int64         `json:"psuCount"`
	Psus     []DellPsuData `json:"-"`
}

type DellPsuData struct {
	PsuPosition    string `json:"-"`
	PsuCapacity    int    `json:"psuCapacity"`
	PsuPresent     int    `json:"psuPresent"`
	PsuActiveError string `json:"psuActiveError"`
	PsuHealth      int    `json:"psuHealth"`
	PsuAcCurrent   string `json:"psuAcCurrent"`
	PsuAcVolts     string `json:"psuAcVolts"`
}

// UnmarshalJSON custom unmarshalling for this "special" data structure
func (d *DellPsuStatus) UnmarshalJSON(data []byte) error {
	var jsonMapping map[string]json.RawMessage
	if err := json.Unmarshal(data, &jsonMapping); err != nil {
		return err
	}

	rfct := reflect.ValueOf(d).Elem()
	rfctType := rfct.Type()

	// TODO(jumartinez): Juliano of the future, if you know by the time a better way of
	//                   doing this. Please refactor it!!.
	for key, value := range jsonMapping {
		for i := 0; i < rfctType.NumField(); i++ {
			if strings.HasPrefix(key, "psu_") {
				p := DellPsuData{}
				err := json.Unmarshal(value, &p)
				if err != nil {
					return err
				}
				p.PsuPosition = key
				d.Psus = append(d.Psus, p)
				break
			} else if key == rfctType.Field(i).Tag.Get("json") {
				var data interface{}
				err := json.Unmarshal(value, &data)
				if err != nil {
					return err
				}

				name := rfctType.Field(i).Name
				f := reflect.Indirect(rfct).FieldByName(name)

				switch f.Kind() {
				case reflect.String:
					f.SetString(data.(string))
				case reflect.Int64:
					d := int64(data.(float64))
					if !f.OverflowInt(d) {
						f.SetInt(d)
					}
				}
				break
			}
		}
	}

	sort.Slice(d.Psus, func(i, j int) bool {
		return d.Psus[i].PsuPosition < d.Psus[j].PsuPosition
	})

	return nil
}

// DellBladeMemoryEndpoint is the struct used to collect data from "https://$ip/sysmgmt/2012/server/memory" when passing the header X_SYSMGMT_OPTIMIZE:true
type DellBladeMemoryEndpoint struct {
	Memory *DellBladeMemory `json:"Memory"`
}

// DellBladeMemory is part of the payload returned by "https://$ip/sysmgmt/2012/server/memory"
type DellBladeMemory struct {
	Capacity       int `json:"capacity"`
	ErrCorrection  int `json:"err_correction"`
	MaxCapacity    int `json:"max_capacity"`
	SlotsAvailable int `json:"slots_available"`
	SlotsUsed      int `json:"slots_used"`
}

// DellBladeProcessorEndpoint is the struct used to collect data from "https://$ip/sysmgmt/2012/server/processor" when passing the header X_SYSMGMT_OPTIMIZE:true
type DellBladeProcessorEndpoint struct {
	Proccessors map[string]*DellBladeProcessor `json:"Processor"`
}

// DellBladeProcessor contains the processor data information
type DellBladeProcessor struct {
	Brand             string                     `json:"brand"`
	CoreCount         int                        `json:"core_count"`
	CurrentSpeed      int                        `json:"current_speed"`
	DeviceDescription string                     `json:"device_description"`
	HyperThreading    []*DellBladeHyperThreading `json:"hyperThreading"`
}

// DellBladeHyperThreading contains the hyperthread information
type DellBladeHyperThreading struct {
	Capable int `json:"capable"`
	Enabled int `json:"enabled"`
}

// IDracAuth is the struct used to verify the iDrac authentication
type IDracAuth struct {
	Status     string `xml:"status"`
	AuthResult int    `xml:"authResult"`
	ForwardURL string `xml:"forwardUrl"`
	ErrorMsg   string `xml:"errorMsg"`
}

// IDracLicense is the struct used to collect data from "https://$ip/sysmgmt/2012/server/license" and it contains the license information for the bmc
type IDracLicense struct {
	License struct {
		VConsole int `json:"VCONSOLE"`
	} `json:"License"`
}

// IDracRoot is the structure used to render the data when querying -> https://$ip/data?get
type IDracRoot struct {
	BiosVer          string                 `xml:"biosVer"`
	FwVersion        string                 `xml:"fwVersion"`
	SysDesc          string                 `xml:"sysDesc"`
	Powermonitordata *IDracPowermonitordata `xml:"powermonitordata,omitempty"`
}

// IDracPowermonitordata contains the power consumption data for the iDrac
type IDracPowermonitordata struct {
	PresentReading *IDracPresentReading `xml:"presentReading,omitempty"`
}

// IDracPresentReading contains the present reading data
type IDracPresentReading struct {
	Reading *IDracReading `xml:" reading,omitempty"`
}

// IDracReading is used to express the power data
type IDracReading struct {
	ProbeName string `xml:" probeName,omitempty"`
	Reading   string `xml:" reading"`
	//Text             string            `xml:",chardata" json:",omitempty"`
}

// DellSVMInventory is the struct used to collect data from "https://$ip/sysmgmt/2012/server/inventory/software"
type DellSVMInventory struct {
	Device []*DellIDracDevice `xml:"Device"`
}

// DellIDracDevice contains the list of devices and their information
type DellIDracDevice struct {
	Display     string                `xml:" display,attr"`
	Application *DellIDracApplication `xml:" Application"`
}

// DellIDracApplication contains the name of the device and it's version
type DellIDracApplication struct {
	Display string `xml:" display,attr"`
	Version string `xml:" version,attr"`
}

// DellSystemServerOS contains the hostname, os name and os version
type DellSystemServerOS struct {
	SystemServerOS struct {
		HostName  string `json:"HostName"`
		OSName    string `json:"OSName"`
		OSVersion string `json:"OSVersion"`
	} `json:"system.ServerOS"`
}

// IDracInventory contains the whole hardware inventory exposed thru https://$ip/sysmgmt/2012/server/inventory/hardware
type IDracInventory struct {
	Version   string            `xml:" version,attr"`
	Component []*IDracComponent `xml:" Component,omitempty"`
}

// IDracComponent holds the information from each component detected by the iDrac
type IDracComponent struct {
	Classname  string           `xml:" Classname,attr"`
	Key        string           `xml:" Key,attr"`
	Properties []*IDracProperty `xml:" PROPERTY,omitempty"`
}

// IDracProperty is the property of each component exposed to iDrac
type IDracProperty struct {
	Name         string `xml:" NAME,attr"`
	Type         string `xml:" TYPE,attr"`
	DisplayValue string `xml:" DisplayValue,omitempty"`
	Value        string `xml:" VALUE,omitempty"`
}

// IDracTemp contains the data structure to render the thermal data from iDrac http://$ip/sysmgmt/2012/server/temperature
type IDracTemp struct {
	Statistics   string `json:"Statistics"`
	Temperatures struct {
		IDRACEmbedded1SystemBoardInletTemp struct {
			MaxFailure         int    `json:"max_failure"`
			MaxWarning         int    `json:"max_warning"`
			MaxWarningSettable int    `json:"max_warning_settable"`
			MinFailure         int    `json:"min_failure"`
			MinWarning         int    `json:"min_warning"`
			MinWarningSettable int    `json:"min_warning_settable"`
			Name               string `json:"name"`
			Reading            int    `json:"reading"`
			SensorStatus       int    `json:"sensor_status"`
		} `json:"iDRAC.Embedded.1#SystemBoardInletTemp"`
	} `json:"Temperatures"`
	IsFreshAirCompliant int `json:"is_fresh_air_compliant"`
}

// DellCMCWWN is the structure used to render the data when querying /json?method=blades-wwn-info
type DellCMCWWN struct {
	SlotMacWwn DellCMCSlotMacWwn `json:"slot_mac_wwn"`
}

// DellCMCWWNBlade contains the blade structure used by DellCMCWWN
type DellCMCWWNBlade struct {
	BladeSlotName     string `json:"bladeSlotName"`
	IsNotDoubleHeight struct {
		IsInstalled string `json:"isInstalled"`
		PortFMAC    string `json:"portFMAC"`
	} `json:"is_not_double_height"`
}

// DellCMCSlotMacWwn contains index of blade by position inside of the chassis
type DellCMCSlotMacWwn struct {
	SlotMacWwnList map[int]DellCMCWWNBlade `json:"-"`
}

// UnmarshalJSON custom unmarshalling for this "special" data structure
func (d *DellCMCSlotMacWwn) UnmarshalJSON(data []byte) error {
	d.SlotMacWwnList = make(map[int]DellCMCWWNBlade, 0)
	var slotMacWwn map[string]json.RawMessage
	if err := json.Unmarshal(data, &slotMacWwn); err != nil {
		return err
	}

	if data, ok := slotMacWwn["slot_mac_wwn_list"]; ok {
		var slotMacWwnList map[string]json.RawMessage
		if err := json.Unmarshal(data, &slotMacWwnList); err != nil {
			return err
		}

		for slot, slotData := range slotMacWwnList {
			if pos, err := strconv.Atoi(slot); err == nil {
				var blade map[string]json.RawMessage
				if err := json.Unmarshal(slotData, &blade); err != nil {
					return err
				}

				b := DellCMCWWNBlade{}
				for key, value := range blade {
					switch key {
					case "bladeSlotName":
						if err := json.Unmarshal(value, &b.BladeSlotName); err != nil {
							return err
						}
					case "is_not_double_height":
						if err := json.Unmarshal(value, &b.IsNotDoubleHeight); err != nil {
							return err
						}
					}
				}
				d.SlotMacWwnList[pos] = b
			}
		}

	}

	return nil
}
