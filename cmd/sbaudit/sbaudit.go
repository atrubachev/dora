package main

import (
	"os"
	"strings"
	"sync"

	"gitlab.booking.com/infra/thermalnator/audit"
	"gitlab.booking.com/infra/thermalnator/simpleapi"

	"github.com/google/gops/agent"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	simpleAPI   *simpleapi.SimpleAPI
	auditor     *audit.Audit
	site        string
	concurrency int
)

func init() {
	// log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func chassisStep() {
	chassis, err := simpleAPI.Chassis()
	if err != nil {
		log.WithFields(log.Fields{"site": site}).Error("Unable to retrieve chassis data. It's the minimum requirement, so I can't continue...")
		return
	}

	cc := make(chan simpleapi.Chassis, concurrency)
	wg := sync.WaitGroup{}
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(input <-chan simpleapi.Chassis, audit *audit.Audit, wg *sync.WaitGroup) {
			auditor.AuditChassis(input)
			wg.Done()
		}(cc, auditor, &wg)
	}

	log.WithFields(log.Fields{"site": site, "type": "chassis"}).Info("Starting data collection")

	for _, c := range chassis.Chassis {
		if strings.Compare(c.Location, site) == 0 || strings.Compare(site, "all") == 0 {
			cc <- *c
		}
	}

	close(cc)
	wg.Wait()
}

func main() {
	if err := agent.Listen(nil); err != nil {
		log.Fatal("Couldn't start gops agent", err)
	}
	viper.SetConfigName("thermalnator")
	viper.AddConfigPath("/etc/bmc-toolbox")
	viper.AddConfigPath("$HOME/.bmc-toolbox")
	viper.SetDefault("site", "all")
	viper.SetDefault("concurrency", 20)
	viper.SetDefault("debug", false)
	viper.SetDefault("noop", false)
	viper.SetDefault("disable_chassis", false)
	viper.SetDefault("disable_discretes", false)

	configItems := []string{
		"bmc_pass",
		"bmc_user",
		"simpleapi_base_url",
		"simpleapi_pass",
		"simpleapi_user",
		"site",
		"telegraf_url",
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalln("Exiting because I couldn't find the configuration file...")
	}

	for _, item := range configItems {
		if !viper.IsSet(item) {
			log.Fatalf("Parameter %s is missing in the config file\n", item)
		}
	}

	if viper.GetBool("debug") {
		log.SetLevel(log.DebugLevel)
	}

	simpleAPI = simpleapi.New(
		viper.GetString("simpleapi_user"),
		viper.GetString("simpleapi_pass"),
		viper.GetString("simpleapi_base_url"),
	)

	auditor = audit.New(
		viper.GetString("bmc_user"),
		viper.GetString("bmc_pass"),
		viper.GetString("telegraf_url"),
		simpleAPI,
	)

	site = viper.GetString("site")
	concurrency = viper.GetInt("concurrency")

	chassisStep()
}
