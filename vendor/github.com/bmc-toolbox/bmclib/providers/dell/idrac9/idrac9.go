package idrac9

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/bmc-toolbox/bmclib/cfgresources"
	"github.com/bmc-toolbox/bmclib/devices"
	"github.com/bmc-toolbox/bmclib/errors"
	"github.com/bmc-toolbox/bmclib/internal/httpclient"
	"github.com/bmc-toolbox/bmclib/internal/sshclient"
	"github.com/bmc-toolbox/bmclib/providers/dell"

	// this make possible to setup logging and properties at any stage
	_ "github.com/bmc-toolbox/bmclib/logging"
	log "github.com/sirupsen/logrus"
)

const (
	// BMCType defines the bmc model that is supported by this package
	BMCType = "idrac9"
)

// IDrac9 holds the status and properties of a connection to an iDrac device
type IDrac9 struct {
	ip             string
	username       string
	password       string
	xsrfToken      string
	httpClient     *http.Client
	sshClient      *sshclient.SSHClient
	iDracInventory *dell.IDracInventory
}

// New returns a new IDrac9 ready to be used
func New(ip string, username string, password string) (iDrac *IDrac9, err error) {
	return &IDrac9{ip: ip, username: username, password: password}, err
}

// CheckCredentials verify whether the credentials are valid or not
func (i *IDrac9) CheckCredentials() (err error) {
	err = i.httpLogin()
	if err != nil {
		return err
	}
	return err
}

// get calls a given json endpoint of the ilo and returns the data
func (i *IDrac9) get(endpoint string, extraHeaders *map[string]string) (payload []byte, err error) {
	log.WithFields(log.Fields{"step": "bmc connection", "vendor": dell.VendorID, "ip": i.ip, "endpoint": endpoint}).Debug("retrieving data from bmc")

	bmcURL := fmt.Sprintf("https://%s", i.ip)
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", bmcURL, endpoint), nil)
	if err != nil {
		return payload, err
	}

	req.Header.Add("XSRF-TOKEN", i.xsrfToken)

	if extraHeaders != nil {
		for key, value := range *extraHeaders {
			req.Header.Add(key, value)
		}
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	payload, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return payload, err
	}

	if resp.StatusCode == 404 {
		return payload, errors.ErrPageNotFound
	}

	return payload, err
}

// Nics returns all found Nics in the device
func (i *IDrac9) Nics() (nics []*devices.Nic, err error) {
	err = i.httpLogin()
	if err != nil {
		return nics, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_NICView" {
			for _, property := range component.Properties {
				if property.Name == "ProductName" && property.Type == "string" {
					data := strings.Split(property.Value, " - ")
					if len(data) == 2 {
						if nics == nil {
							nics = make([]*devices.Nic, 0)
						}

						n := &devices.Nic{
							Name:       data[0],
							MacAddress: strings.ToLower(data[1]),
						}
						nics = append(nics, n)
					} else {
						err = multierror.Append(err, fmt.Errorf("invalid network card %s, please review", data))
					}
				}
			}
		} else if component.Classname == "DCIM_iDRACCardView" {
			for _, property := range component.Properties {
				if property.Name == "PermanentMACAddress" && property.Type == "string" {
					if nics == nil {
						nics = make([]*devices.Nic, 0)
					}

					n := &devices.Nic{
						Name:       "bmc",
						MacAddress: strings.ToLower(property.Value),
					}
					nics = append(nics, n)
				}
			}
		}
	}
	return nics, err
}

// Serial returns the device serial
func (i *IDrac9) Serial() (serial string, err error) {
	err = i.httpLogin()
	if err != nil {
		return serial, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "NodeID" && property.Type == "string" {
					return strings.ToLower(property.Value), err
				}
			}
		}
	}
	return serial, err
}

// Status returns health string status from the bmc
func (i *IDrac9) Status() (status string, err error) {
	err = i.httpLogin()
	if err != nil {
		return status, err
	}

	extraHeaders := &map[string]string{
		"X-SYSMGMT-OPTIMIZE": "true",
	}

	url := "sysmgmt/2016/server/extended_health"
	payload, err := i.get(url, extraHeaders)
	if err != nil {
		return status, err
	}

	iDracHealthStatus := &dell.IDracHealthStatus{}
	err = json.Unmarshal(payload, iDracHealthStatus)
	if err != nil {
		httpclient.DumpInvalidPayload(url, i.ip, payload)
		return status, err
	}

	for _, entry := range iDracHealthStatus.HealthStatus {
		if entry != 0 && entry != 2 {
			return "Degraded", err
		}
	}

	return "OK", err
}

// PowerKw returns the current power usage in Kw
func (i *IDrac9) PowerKw() (power float64, err error) {
	err = i.httpLogin()
	if err != nil {
		return power, err
	}

	url := "sysmgmt/2015/server/sensor/power"
	payload, err := i.get(url, nil)
	if err != nil {
		return power, err
	}

	iDracPowerData := &dell.IDracPowerData{}
	err = json.Unmarshal(payload, iDracPowerData)
	if err != nil {
		httpclient.DumpInvalidPayload(url, i.ip, payload)
		return power, err
	}

	return iDracPowerData.Root.Powermonitordata.PresentReading.Reading.Reading / 1000.00, err
}

// PowerState returns the current power state of the machine
func (i *IDrac9) PowerState() (state string, err error) {
	err = i.httpLogin()
	if err != nil {
		return state, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "PowerState" && property.Type == "uint16" {
					return strings.ToLower(property.DisplayValue), err
				}
			}
		}
	}
	return state, err
}

// BiosVersion returns the current version of the bios
func (i *IDrac9) BiosVersion() (version string, err error) {
	err = i.httpLogin()
	if err != nil {
		return version, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "BIOSVersionString" && property.Type == "string" {
					return property.Value, err
				}
			}
		}
	}

	return version, err
}

// Name returns the name of this server from the bmc point of view
func (i *IDrac9) Name() (name string, err error) {
	err = i.httpLogin()
	if err != nil {
		return name, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "HostName" && property.Type == "string" {
					return property.Value, err
				}
			}
		}
	}

	return name, err
}

// BmcVersion returns the version of the bmc we are running
func (i *IDrac9) BmcVersion() (bmcVersion string, err error) {
	err = i.httpLogin()
	if err != nil {
		return bmcVersion, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_iDRACCardView" {
			for _, property := range component.Properties {
				if property.Name == "FirmwareVersion" && property.Type == "string" {
					return property.Value, err
				}
			}
		}
	}
	return bmcVersion, err
}

// Model returns the device model
func (i *IDrac9) Model() (model string, err error) {
	err = i.httpLogin()
	if err != nil {
		return model, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "Model" && property.Type == "string" {
					return property.Value, err
				}
			}
		}
	}
	return model, err
}

// BmcType returns the type of bmc we are talking to
func (i *IDrac9) BmcType() (bmcType string) {
	return BMCType
}

// License returns the bmc license information
func (i *IDrac9) License() (name string, licType string, err error) {
	err = i.httpLogin()
	if err != nil {
		return name, licType, err
	}

	extraHeaders := &map[string]string{
		"X_SYSMGMT_OPTIMIZE": "true",
	}

	url := "sysmgmt/2012/server/license"
	payload, err := i.get(url, extraHeaders)
	if err != nil {
		return name, licType, err
	}

	iDracLicense := &dell.IDracLicense{}
	err = json.Unmarshal(payload, iDracLicense)
	if err != nil {
		httpclient.DumpInvalidPayload(url, i.ip, payload)
		return name, licType, err
	}

	if iDracLicense.License.VConsole == 1 {
		return "Enterprise", "Licensed", err
	}
	return "-", "Unlicensed", err
}

// Memory return the total amount of memory of the server
func (i *IDrac9) Memory() (mem int, err error) {
	err = i.httpLogin()
	if err != nil {
		return mem, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_SystemView" {
			for _, property := range component.Properties {
				if property.Name == "SysMemTotalSize" && property.Type == "uint32" {
					size, err := strconv.Atoi(property.Value)
					if err != nil {
						return mem, err
					}
					return size / 1024, err
				}
			}
		}
	}
	return mem, err
}

// TempC returns the current temperature of the machine
func (i *IDrac9) TempC() (temp int, err error) {
	err = i.httpLogin()
	if err != nil {
		return temp, err
	}

	extraHeaders := &map[string]string{
		"X-SYSMGMT-OPTIMIZE": "true",
	}

	url := "sysmgmt/2012/server/temperature"
	payload, err := i.get(url, extraHeaders)
	if err != nil {
		return temp, err
	}

	iDracTemp := &dell.IDracTemp{}
	err = json.Unmarshal(payload, iDracTemp)
	if err != nil {
		httpclient.DumpInvalidPayload(url, i.ip, payload)
		return temp, err
	}

	return iDracTemp.Temperatures.IDRACEmbedded1SystemBoardInletTemp.Reading, err
}

// CPU return the cpu, cores and hyperthreads the server
func (i *IDrac9) CPU() (cpu string, cpuCount int, coreCount int, hyperthreadCount int, err error) {
	err = i.httpLogin()
	if err != nil {
		return cpu, cpuCount, coreCount, hyperthreadCount, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_CPUView" {
			cpuCount++
			if component.Key == "CPU.Socket.1" {
				var e error
				for _, property := range component.Properties {
					if property.Name == "Model" && property.Type == "string" {
						cpu = httpclient.StandardizeProcessorName(property.DisplayValue)
					} else if property.Name == "NumberOfProcessorCores" && property.Type == "uint32" {
						if coreCount, e = strconv.Atoi(property.Value); e != nil {
							err = multierror.Append(err, fmt.Errorf("invalid core count %s", e))
						}
					} else if property.Name == "NumberOfEnabledThreads" && property.Type == "uint32" {
						if hyperthreadCount, e = strconv.Atoi(property.Value); e != nil {
							err = multierror.Append(err, fmt.Errorf("invalid thread count %s", e))
						}
					}
				}
			}
		}
	}

	return cpu, cpuCount, coreCount, hyperthreadCount, err
}

// IsBlade returns if the current hardware is a blade or not
func (i *IDrac9) IsBlade() (isBlade bool, err error) {
	err = i.httpLogin()
	if err != nil {
		return isBlade, err
	}

	model, err := i.Model()
	if err != nil {
		return isBlade, err
	}

	if strings.HasPrefix(model, "PowerEdge M") {
		isBlade = true
	}

	return isBlade, err
}

// Psus returns a list of psus installed on the device
func (i *IDrac9) Psus() (psus []*devices.Psu, err error) {
	err = i.httpLogin()
	if err != nil {
		return psus, err
	}

	url := "data?get=powerSupplies"
	payload, err := i.get(url, nil)
	if err != nil {
		return psus, err
	}

	iDracRoot := &dell.IDracRoot{}
	err = xml.Unmarshal(payload, iDracRoot)
	if err != nil {
		httpclient.DumpInvalidPayload(url, i.ip, payload)
		return psus, err
	}

	serial, _ := i.Serial()

	for _, psu := range iDracRoot.PsSensorList {
		if psus == nil {
			psus = make([]*devices.Psu, 0)
		}
		var status string
		if psu.SensorHealth == 2 {
			status = "OK"
		} else {
			status = "BROKEN"
		}

		p := &devices.Psu{
			Serial:     fmt.Sprintf("%s_%s", serial, strings.Split(psu.Name, " ")[0]),
			Status:     status,
			CapacityKw: float64(psu.MaxWattage) / 1000.00,
		}

		psus = append(psus, p)
	}

	return psus, err
}

// Vendor returns bmc's vendor
func (i *IDrac9) Vendor() (vendor string) {
	return dell.VendorID
}

// ServerSnapshot do best effort to populate the server data and returns a blade or discrete
func (i *IDrac9) ServerSnapshot() (server interface{}, err error) {
	err = i.httpLogin()
	if err != nil {
		return server, err
	}

	if isBlade, _ := i.IsBlade(); isBlade {
		blade := &devices.Blade{}
		blade.Vendor = i.Vendor()
		blade.BmcAddress = i.ip
		blade.BmcType = i.BmcType()

		blade.Serial, _ = i.Serial()
		if err != nil {
			return nil, err
		}
		blade.BmcVersion, err = i.BmcVersion()
		if err != nil {
			return nil, err
		}
		blade.Model, err = i.Model()
		if err != nil {
			return nil, err
		}
		blade.Nics, err = i.Nics()
		if err != nil {
			return nil, err
		}
		blade.Disks, err = i.Disks()
		if err != nil {
			return nil, err
		}
		blade.BiosVersion, err = i.BiosVersion()
		if err != nil {
			return nil, err
		}
		blade.Processor, blade.ProcessorCount, blade.ProcessorCoreCount, blade.ProcessorThreadCount, err = i.CPU()
		if err != nil {
			return nil, err
		}
		blade.Memory, err = i.Memory()
		if err != nil {
			return nil, err
		}
		blade.Status, err = i.Status()
		if err != nil {
			return nil, err
		}
		blade.Name, err = i.Name()
		if err != nil {
			return nil, err
		}
		blade.TempC, err = i.TempC()
		if err != nil {
			return nil, err
		}
		blade.PowerKw, err = i.PowerKw()
		if err != nil {
			return nil, err
		}
		blade.BmcLicenceType, blade.BmcLicenceStatus, err = i.License()
		if err != nil {
			return nil, err
		}
		server = blade
	} else {
		discrete := &devices.Discrete{}
		discrete.Vendor = i.Vendor()
		discrete.BmcAddress = i.ip
		discrete.BmcType = i.BmcType()

		discrete.Serial, err = i.Serial()
		if err != nil {
			return nil, err
		}
		discrete.BmcVersion, err = i.BmcVersion()
		if err != nil {
			return nil, err
		}
		discrete.Model, err = i.Model()
		if err != nil {
			return nil, err
		}
		discrete.Nics, err = i.Nics()
		if err != nil {
			return nil, err
		}
		discrete.Disks, err = i.Disks()
		if err != nil {
			return nil, err
		}
		discrete.BiosVersion, err = i.BiosVersion()
		if err != nil {
			return nil, err
		}
		discrete.Processor, discrete.ProcessorCount, discrete.ProcessorCoreCount, discrete.ProcessorThreadCount, err = i.CPU()
		if err != nil {
			return nil, err
		}
		discrete.Memory, err = i.Memory()
		if err != nil {
			return nil, err
		}
		discrete.Status, err = i.Status()
		if err != nil {
			return nil, err
		}
		discrete.Name, err = i.Name()
		if err != nil {
			return nil, err
		}
		discrete.TempC, err = i.TempC()
		if err != nil {
			return nil, err
		}
		discrete.PowerKw, err = i.PowerKw()
		if err != nil {
			return nil, err
		}
		discrete.BmcLicenceType, discrete.BmcLicenceStatus, err = i.License()
		if err != nil {
			return nil, err
		}
		discrete.Psus, err = i.Psus()
		if err != nil {
			return nil, err
		}
		server = discrete
	}

	return server, err
}

// Disks returns a list of disks installed on the device
func (i *IDrac9) Disks() (disks []*devices.Disk, err error) {
	err = i.httpLogin()
	if err != nil {
		return disks, err
	}

	for _, component := range i.iDracInventory.Component {
		if component.Classname == "DCIM_PhysicalDiskView" {
			if disks == nil {
				disks = make([]*devices.Disk, 0)
			}
			disk := &devices.Disk{}

			for _, property := range component.Properties {
				if property.Name == "Model" {
					disk.Model = strings.ToLower(property.Value)
				} else if property.Name == "SerialNumber" {
					disk.Serial = strings.ToLower(property.Value)
				} else if property.Name == "MediaType" {
					if property.DisplayValue == "Solid State Drive" {
						disk.Type = "SSD"
					} else if property.DisplayValue == "Hard Disk Drive" {
						disk.Type = "HDD"
					} else {
						disk.Type = property.DisplayValue
					}
				} else if property.Name == "PrimaryStatus" {
					disk.Status = property.DisplayValue
				} else if property.Name == "DeviceDescription" {
					disk.Location = property.DisplayValue
				} else if property.Name == "SizeInBytes" {
					size, err := strconv.Atoi(property.Value)
					if err != nil {
						return disks, err
					}
					disk.Size = fmt.Sprintf("%d GB", size/1024/1024/1024)
				} else if property.Name == "Revision" {
					disk.FwVersion = strings.ToLower(property.Value)
				}
			}

			if disk.Serial != "" {
				disks = append(disks, disk)
			}
		}
	}
	return disks, err
}

// ApplyCfg applies the configuration on the bmc
func (i *IDrac9) ApplyCfg(config *cfgresources.ResourcesConfig) (err error) {
	return errors.ErrNotImplemented
}

// UpdateCredentials updates login credentials
func (i *IDrac9) UpdateCredentials(username string, password string) {
	i.username = username
	i.password = password
}
