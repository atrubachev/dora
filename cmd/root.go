// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "dora",
	Short: "Tool to discover, collect data and manage all types of BMCs and Chassis",
	Long: `Tool to discover, collect data and manage all types of BMCs and Chassis:

Dora scan the networks found in the kea.conf from there it discovers 
all types of BMCs and Chassis. Dora can also configure chassis and/or 
make ad-hoc queries to specific devices and/or ips. 

We currently support HP, Dell and Supermicros.
`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/bmc-toolbox/dora.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		viper.SetConfigName("dora")
		viper.AddConfigPath("/etc/bmc-toolbox")
		viper.AddConfigPath(fmt.Sprintf("%s/.bmc-toolbox", home))
	}

	viper.SetDefault("site", "all")
	viper.SetDefault("concurrency", 20)
	viper.SetDefault("debug", false)
	viper.SetDefault("noop", false)
	viper.SetDefault("disable_chassis", false)
	viper.SetDefault("disable_discretes", false)
	viper.SetDefault("dump_invalid_payloads", false)
	viper.SetDefault("dump_invalid_payload_path", "/tmp/dora/dumps")
	viper.SetDefault("kea_config", "/etc/kea/kea.conf")
	viper.SetDefault("nmap", "/bin/nmap")
	viper.SetDefault("http_server_port", 8000)
	viper.SetDefault("nmap_tcp_ports", "22,443")
	viper.SetDefault("nmap_udp_ports", "161,623")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Failed to read config: %s", err)
		os.Exit(1)
	}

	if viper.GetBool("debug") {
		log.SetLevel(log.DebugLevel)
	}
}
