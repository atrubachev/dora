package connectors

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"golang.org/x/net/publicsuffix"
)

var (
	// ErrLoginFailed is returned when we fail to login to a bmc
	ErrLoginFailed = errors.New("Failed to login")
	// ErrBiosNotFound is returned when we are not able to find the server bios version
	ErrBiosNotFound = errors.New("Bios version not found")
)

// newUUID generates a random UUID according to RFC 4122
func newUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80
	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func httpGet(url string, username *string, password *string) (payload []byte, err error) {
	log.WithFields(log.Fields{"step": "ChassisConnections", "url": url}).Debug("Requesting data from BMC")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return payload, err
	}
	req.SetBasicAuth(*username, *password)
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Timeout:   time.Second * 20,
		Transport: tr,
	}
	resp, err := client.Do(req)
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	payload, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return payload, err
	}

	return payload, err
}

func httpGetDell(hostname *string, endpoint string, username *string, password *string) (payload []byte, err error) {
	log.WithFields(log.Fields{"step": "ChassisConnections Dell", "hostname": *hostname}).Debug("Requesting data from BMC")

	form := url.Values{}
	form.Add("user", *username)
	form.Add("password", *password)

	u, err := url.Parse(fmt.Sprintf("https://%s/cgi-bin/webcgi/login", *hostname))
	if err != nil {
		return payload, err
	}

	req, err := http.NewRequest("POST", u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return payload, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return payload, err
	}

	client := &http.Client{
		Timeout:   time.Second * 20,
		Transport: tr,
		Jar:       jar,
	}

	resp, err := client.Do(req)
	if err != nil {
		return payload, err
	}
	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	resp, err = client.Get(fmt.Sprintf("https://%s/cgi-bin/webcgi/%s", *hostname, endpoint))
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	payload, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return payload, err
	}

	resp, err = client.Get(fmt.Sprintf("https://%s/cgi-bin/webcgi/logout", *hostname))
	if err != nil {
		return payload, err
	}
	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	// Dell has a really shitty consistency of the data type returned, here we fix what's possible
	payload = bytes.Replace(payload, []byte("\"bladeTemperature\":-1"), []byte("\"bladeTemperature\":\"0\""), -1)
	payload = bytes.Replace(payload, []byte("\"nic\": [],"), []byte("\"nic\": {},"), -1)
	payload = bytes.Replace(payload, []byte("N\\/A"), []byte("0"), -1)

	return payload, err
}

func httpGetHP(hostname *string, endpoint string, username *string, password *string) (payload []byte, err error) {
	log.WithFields(log.Fields{"step": "ChassisConnections HP", "hostname": *hostname}).Debug("Requesting data from BMC")

	data := []byte(fmt.Sprintf("{\"method\":\"login\", \"user_login\":\"%s\", \"password\":\"%s\" }", *username, *password))
	u, err := url.Parse(fmt.Sprintf("https://%s/json/login_session", *hostname))
	if err != nil {
		return payload, err
	}

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(data))
	if err != nil {
		return payload, err
	}
	req.Header.Set("Content-Type", "application/json")

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: false,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return payload, err
	}

	client := &http.Client{
		Timeout:   time.Second * 20,
		Transport: tr,
		Jar:       jar,
	}

	resp, err := client.Do(req)
	if err != nil {
		return payload, err
	}

	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	resp, err = client.Get(fmt.Sprintf("https://%s/%s", *hostname, endpoint))
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	payload, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return payload, err
	}

	data = []byte(`{"method":"logout"}`)

	req, err = http.NewRequest("POST", u.String(), bytes.NewBuffer(data))
	if err != nil {
		return payload, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err = client.Do(req)
	if err != nil {
		return payload, err
	}
	io.Copy(ioutil.Discard, resp.Body)
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return payload, ErrPageNotFound
	}

	return payload, err
}
