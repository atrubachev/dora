package connectors

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
	"gitlab.booking.com/infra/dora/model"
)

// IloReader holds the status and properties of a connection to an iLO device
type IloReader struct {
	ip          *string
	username    *string
	password    *string
	client      *http.Client
	loginURL    *url.URL
	hpRimpBlade *HpRimpBlade
}

// NewIloReader returns a new IloReader ready to be used
func NewIloReader(ip *string, username *string, password *string) (ilo *IloReader, err error) {
	loginURL, err := url.Parse(fmt.Sprintf("https://%s/json/login_session", *ip))
	if err != nil {
		return nil, err
	}

	client, err := buildClient()
	if err != nil {
		return ilo, err
	}

	resp, err := client.Get(fmt.Sprintf("https://%s/xmldata?item=all", *ip))
	if err != nil {
		return ilo, err
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ilo, err
	}
	defer resp.Body.Close()

	hpRimpBlade := &HpRimpBlade{}
	err = xml.Unmarshal(payload, hpRimpBlade)
	if err != nil {
		DumpInvalidPayload(*ip, payload)
		return ilo, err
	}

	return &IloReader{ip: ip, username: username, password: password, loginURL: loginURL, hpRimpBlade: hpRimpBlade, client: client}, err
}

// Login initiates the connection to an iLO device
func (i *IloReader) Login() (err error) {
	log.WithFields(log.Fields{"step": "Ilo Connection HP", "ip": *i.ip}).Debug("Connecting to iLO")

	data := fmt.Sprintf("{\"method\":\"login\", \"user_login\":\"%s\", \"password\":\"%s\" }", *i.username, *i.password)

	req, err := http.NewRequest("POST", i.loginURL.String(), bytes.NewBufferString(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == 404 {
		return ErrPageNotFound
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if strings.Contains(string(payload), "Invalid login attempt") {
		return ErrLoginFailed
	}

	return err
}

// get calls a given json endpoint of the ilo and returns the data
func (i *IloReader) get(endpoint string) (payload []byte, err error) {
	log.WithFields(log.Fields{"step": "Ilo Connection HP", "ip": *i.ip, "endpoint": endpoint}).Debug("Retrieving data from iLO")

	resp, err := i.client.Get(fmt.Sprintf("https://%s/%s", *i.ip, endpoint))
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	payload, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return payload, err
	}

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	return payload, err
}

// Serial returns the device serial
func (i *IloReader) Serial() (serial string, err error) {
	return strings.ToLower(strings.TrimSpace(i.hpRimpBlade.HpHSI.Sbsn)), err
}

// Model returns the device model
func (i *IloReader) Model() (model string, err error) {
	return i.hpRimpBlade.HpHSI.Spn, err
}

// BmcType returns the type of bmc we are talking to
func (i *IloReader) BmcType() (bmcType string, err error) {
	switch i.hpRimpBlade.HpMP.Pn {
	case "Integrated Lights-Out 4 (iLO 4)":
		return "iLO4", err
	case "Integrated Lights-Out 3 (iLO 3)":
		return "iLO3", err
	default:
		return i.hpRimpBlade.HpMP.Pn, err
	}
}

// BmcVersion returns the version of the bmc we are running
func (i *IloReader) BmcVersion() (bmcVersion string, err error) {
	return i.hpRimpBlade.HpMP.Fwri, err
}

// Name returns the name of this server from the iLO point of view
func (i *IloReader) Name() (name string, err error) {
	payload, err := i.get("json/overview")
	if err != nil {
		return name, err
	}

	hpOverview := &HpOverview{}
	err = json.Unmarshal(payload, hpOverview)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return name, err
	}

	return hpOverview.ServerName, err
}

// Status returns health string status from the bmc
func (i *IloReader) Status() (health string, err error) {
	payload, err := i.get("json/overview")
	if err != nil {
		return health, err
	}

	hpOverview := &HpOverview{}
	err = json.Unmarshal(payload, hpOverview)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return health, err
	}

	if hpOverview.SystemHealth == "OP_STATUS_OK" {
		return "OK", err
	}

	return hpOverview.SystemHealth, err
}

// Memory returns the total amount of memory of the server
func (i *IloReader) Memory() (mem int, err error) {
	payload, err := i.get("json/mem_info")
	if err != nil {
		return mem, err
	}

	hpMemData := &HpMem{}
	err = json.Unmarshal(payload, hpMemData)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return mem, err
	}

	if hpMemData.MemTotalMemSize != 0 {
		return hpMemData.MemTotalMemSize / 1024, err
	}

	for _, slot := range hpMemData.Memory {
		mem = mem + slot.MemSize
	}

	return mem / 1024, err
}

// CPU returns the cpu, cores and hyperthreads of the server
func (i *IloReader) CPU() (cpu string, cpuCount int, coreCount int, hyperthreadCount int, err error) {
	payload, err := i.get("json/proc_info")
	if err != nil {
		return cpu, cpuCount, coreCount, hyperthreadCount, err
	}

	hpProcData := &HpProcs{}
	err = json.Unmarshal(payload, hpProcData)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return cpu, cpuCount, coreCount, hyperthreadCount, err
	}

	for _, proc := range hpProcData.Processors {
		return strings.TrimSpace(proc.ProcName), len(hpProcData.Processors), proc.ProcNumCores, proc.ProcNumThreads, err
	}

	return cpu, cpuCount, coreCount, hyperthreadCount, err
}

// BiosVersion returns the current version of the bios
func (i *IloReader) BiosVersion() (version string, err error) {
	payload, err := i.get("json/overview")
	if err != nil {
		return version, err
	}

	hpOverview := &HpOverview{}
	err = json.Unmarshal(payload, hpOverview)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return version, err
	}

	if hpOverview.SystemRom != "" {
		return hpOverview.SystemRom, err
	}

	return version, ErrBiosNotFound
}

// PowerKw returns the current power usage in Kw
func (i *IloReader) PowerKw() (power float64, err error) {
	payload, err := i.get("json/power_summary")
	if err != nil {
		return power, err
	}

	hpPowerSummary := &HpPowerSummary{}
	err = json.Unmarshal(payload, hpPowerSummary)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return power, err
	}

	return float64(hpPowerSummary.PowerSupplyInputPower) / 1024, err
}

// TempC returns the current temperature of the machine
func (i *IloReader) TempC() (temp int, err error) {
	payload, err := i.get("json/health_temperature")
	if err != nil {
		return temp, err
	}

	hpHelthTemperature := &HpHelthTemperature{}
	err = json.Unmarshal(payload, hpHelthTemperature)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return temp, err
	}

	for _, item := range hpHelthTemperature.Temperature {
		if item.Location == "Ambient" {
			return item.Currentreading, err
		}
	}

	return temp, err
}

// Nics returns all found Nics in the device
func (i *IloReader) Nics() (nics []*model.Nic, err error) {
	if i.hpRimpBlade.HpHSI != nil &&
		i.hpRimpBlade.HpHSI.HpNICS != nil &&
		i.hpRimpBlade.HpHSI.HpNICS.HpNIC != nil {
		for _, nic := range i.hpRimpBlade.HpHSI.HpNICS.HpNIC {
			var name string
			if strings.HasPrefix(nic.Description, "iLO") {
				name = "bmc"
			} else {
				name = nic.Description
			}

			if nics == nil {
				nics = make([]*model.Nic, 0)
			}

			n := &model.Nic{
				Name:       name,
				MacAddress: strings.ToLower(nic.MacAddr),
			}
			nics = append(nics, n)
		}
	}
	return nics, err
}

// License returns the iLO's license information
func (i *IloReader) License() (name string, licType string, err error) {
	payload, err := i.get("json/license")
	if err != nil {
		return name, licType, err
	}

	hpIloLicense := &HpIloLicense{}
	err = json.Unmarshal(payload, hpIloLicense)
	if err != nil {
		DumpInvalidPayload(*i.ip, payload)
		return name, licType, err
	}

	return hpIloLicense.Name, hpIloLicense.Type, err
}

// Logout logs out and close the iLo connection
func (i *IloReader) Logout() (err error) {
	log.WithFields(log.Fields{"step": "Ilo Connection HP", "ip": *i.ip}).Debug("Logout from iLO")

	data := []byte(`{"method":"logout"}`)

	req, err := http.NewRequest("POST", i.loginURL.String(), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	return err
}
