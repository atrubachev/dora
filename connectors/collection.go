package connectors

import (
	"fmt"
	"net"
	"sync"

	"gitlab.booking.com/go/bmc/errors"

	"github.com/jinzhu/gorm"
	nats "github.com/nats-io/go-nats"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gitlab.booking.com/go/bmc/devices"
	"gitlab.booking.com/go/bmc/discover"
	"gitlab.booking.com/go/dora/model"
	"gitlab.booking.com/go/dora/storage"
)

var (
	notifyChange chan string
)

func collect(input <-chan string, source *string, db *gorm.DB) {
	bmcUser := viper.GetString("bmc_user")
	bmcPass := viper.GetString("bmc_pass")

	for host := range input {
		log.WithFields(log.Fields{"operation": "scan", "ip": host}).Debug("collection started")

		conn, err := discover.ScanAndConnect(host, bmcUser, bmcPass)
		if err != nil {
			log.WithFields(log.Fields{"operation": "scan", "ip": host}).Error(err)
			continue
		}

		if bmc, ok := conn.(devices.Bmc); ok {
			err = bmc.Login()
			if err == errors.ErrLoginFailed {
				bmc.UpdateCredentials(
					viper.GetString(fmt.Sprintf("collector.default.%s.username", bmc.Vendor())),
					viper.GetString(fmt.Sprintf("collector.default.%s.password", bmc.Vendor())),
				)
				err = bmc.Login()
				if err != nil {
					log.WithFields(log.Fields{"operation": "connection", "ip": host}).Error(err)
					continue
				}
			} else if err != nil {
				log.WithFields(log.Fields{"operation": "connection", "ip": host}).Error(err)
				continue
			}

			if isBlade, err := bmc.IsBlade(); isBlade && *source != "cli-with-force" {
				log.WithFields(log.Fields{"operation": "detection", "ip": host}).Debug("we don't want to scan blades directly since the chassis does it for us")
				continue
			} else if err != nil {
				log.WithFields(log.Fields{"operation": "collection", "ip": host}).Error(err)
				continue
			}

			err := collectBmc(bmc)
			if err != nil {
				log.WithFields(log.Fields{"operation": "collection", "ip": host}).Error(err)
			}
		} else if bmc, ok := conn.(devices.BmcChassis); ok {
			err = bmc.Login()
			if err == errors.ErrLoginFailed {
				bmc.UpdateCredentials(
					viper.GetString(fmt.Sprintf("collector.default.%s.username", bmc.Vendor())),
					viper.GetString(fmt.Sprintf("collector.default.%s.password", bmc.Vendor())),
				)
				err = bmc.Login()
				if err != nil {
					log.WithFields(log.Fields{"operation": "connection", "ip": host}).Error(err)
					continue
				}
			} else if err != nil {
				log.WithFields(log.Fields{"operation": "connection", "ip": host}).Error(err)
				continue
			}

			err := collectBmcChassis(bmc)
			if err != nil {
				log.WithFields(log.Fields{"operation": "collection", "ip": host}).Error(err)
			}
		} else {
			log.WithFields(log.Fields{"operation": "collection", "ip": host}).Debug("Unknown hardware skipping...")
		}
	}
}

// DataCollection collects the data of all given ips
func DataCollection(ips []string, source string) {
	concurrency := viper.GetInt("collector.concurrency")

	cc := make(chan string, concurrency)
	wg := sync.WaitGroup{}
	db := storage.InitDB()

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(input <-chan string, source *string, db *gorm.DB, wg *sync.WaitGroup) {
			defer wg.Done()
			collect(input, source, db)
		}(cc, &source, db, &wg)
	}

	notifyChange = make(chan string)
	go func(notification <-chan string) {
		for callback := range notification {
			err := assetNotify(callback)
			if err != nil {
				log.WithFields(log.Fields{"operation": "ServerDB callback", "url": callback}).Error(err)
			}
		}
	}(notifyChange)

	if ips[0] == "all" {
		hosts := []model.ScannedPort{}
		if err := db.Where("port = 443 and protocol = 'tcp' and state = 'open'").Find(&hosts).Error; err != nil {
			log.WithFields(log.Fields{"operation": "retrieving scanned hosts", "ip": "all"}).Error(err)
		} else {
			for _, host := range hosts {
				cc <- host.IP
			}
		}
	} else {
		for _, ip := range ips {
			host := model.ScannedPort{}
			parsedIP := net.ParseIP(ip)
			if parsedIP == nil {
				lookup, err := net.LookupHost(ip)
				if err != nil {
					log.WithFields(log.Fields{"operation": "retrieving scanned hosts", "ip": ip}).Error(err)
					continue
				}
				ip = lookup[0]
			}

			if err := db.Where("ip = ? and port = 443 and protocol = 'tcp' and state = 'open'", ip).Find(&host).Error; err != nil {
				log.WithFields(log.Fields{"operation": "retrieving scanned hosts", "ip": ip}).Error(err)
				continue
			}
			cc <- host.IP
		}
	}

	close(cc)
	wg.Wait()
}

// DataCollectionWorker collects the data of all given ips
func DataCollectionWorker() {
	nc, err := nats.Connect(viper.GetString("collector.worker.server"), nats.UserInfo(viper.GetString("collector.worker.username"), viper.GetString("collector.worker.password")))
	if err != nil {
		log.Fatalf("Subscriber unable to connect: %v\n", err)
	}

	concurrency := viper.GetInt("collector.concurrency")
	cc := make(chan string, concurrency)
	wg := sync.WaitGroup{}
	db := storage.InitDB()
	source := "worker"

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(input <-chan string, source *string, db *gorm.DB, wg *sync.WaitGroup) {
			defer wg.Done()
			collect(input, source, db)
		}(cc, &source, db, &wg)
	}

	nc.QueueSubscribe("dora::collect", viper.GetString("collector.worker.queue"), func(msg *nats.Msg) {
		ip := string(msg.Data)
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			lookup, err := net.LookupHost(ip)
			if err != nil {
				log.WithFields(log.Fields{"operation": "retrieving scanned hosts", "ip": ip}).Error(err)
				return
			}
			ip = lookup[0]
		}
		cc <- ip
	})
	nc.Flush()

	if err := nc.LastError(); err != nil {
		log.WithFields(log.Fields{"operation": "registering worker"}).Fatal(err)
	}

	log.WithFields(log.Fields{"queue": viper.GetString("collector.worker.queue"), "subject": "dora::collect"}).Info("Subscribed to queue")

	notifyChange = make(chan string)
	go func(notification <-chan string) {
		for callback := range notification {
			err := assetNotify(callback)
			if err != nil {
				log.WithFields(log.Fields{"operation": "ServerDB callback", "url": callback}).Error(err)
			}
		}
	}(notifyChange)

	//close(cc)
	//wg.Wait()
}

func collectBmc(bmc devices.Bmc) (err error) {
	defer bmc.Logout()

	serial, err := bmc.Serial()
	if err != nil {
		return err
	}

	if serial == "" || serial == "[unknown]" || serial == "0000000000" || serial == "_" {
		return ErrInvalidSerial
	}

	isBlade, err := bmc.IsBlade()
	if err != nil {
		return err
	}

	db := storage.InitDB()
	if isBlade {
		server, err := bmc.ServerSnapshot()
		if err != nil {
			return err
		}

		b, ok := server.(*devices.Blade)
		if !ok {
			return fmt.Errorf("Unable to read devices.Blade")
		}

		blade := model.NewBladeFromDevice(b)
		blade.BmcAuth = true
		blade.BmcWEBReachable = true
		scans := []model.ScannedPort{}
		db.Where("ip = ?", blade.BmcAddress).Find(&scans)
		for _, scan := range scans {
			if scan.Port == 22 && scan.Protocol == "tcp" && scan.State == "open" {
				blade.BmcSSHReachable = true
			} else if scan.Port == 443 && scan.Protocol == "tcp" && scan.State == "open" {
				blade.BmcWEBReachable = true
			} else if scan.Port == 623 && scan.Protocol == "ipmi" && scan.State == "open" {
				blade.BmcIpmiReachable = true
			}
		}

		bladeStorage := storage.NewBladeStorage(db)
		existingData, err := bladeStorage.GetOne(blade.Serial)
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		_, err = bladeStorage.UpdateOrCreate(blade)
		if err != nil {
			return err
		}

		if len(blade.Diff(&existingData)) != 0 {
			notifyChange <- fmt.Sprintf("%s/%s/%s", viper.GetString("url"), "blades", blade.Serial)
			for _, nic := range existingData.Nics {
				if !blade.HasNic(nic.MacAddress) {
					db.Delete(&nic)
				}
			}
		}
	} else {
		server, err := bmc.ServerSnapshot()
		if err != nil {
			return err
		}

		b, ok := server.(*devices.Discrete)
		if !ok {
			return fmt.Errorf("Unable to read devices.Discrete")
		}

		discrete := model.NewDiscreteFromDevice(b)
		discrete.BmcAuth = true
		discrete.BmcWEBReachable = true

		scans := []model.ScannedPort{}
		db.Where("ip = ?", discrete.BmcAddress).Find(&scans)
		for _, scan := range scans {
			if scan.Port == 22 && scan.Protocol == "tcp" && scan.State == "open" {
				discrete.BmcSSHReachable = true
			} else if scan.Port == 443 && scan.Protocol == "tcp" && scan.State == "open" {
				discrete.BmcWEBReachable = true
			} else if scan.Port == 623 && scan.Protocol == "ipmi" && scan.State == "open" {
				discrete.BmcIpmiReachable = true
			}
		}

		discreteStorage := storage.NewDiscreteStorage(db)
		existingData, err := discreteStorage.GetOne(discrete.Serial)
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		_, err = discreteStorage.UpdateOrCreate(discrete)
		if err != nil {
			return err
		}

		if len(discrete.Diff(&existingData)) != 0 {
			notifyChange <- fmt.Sprintf("%s/%s/%s", viper.GetString("url"), "discretes", discrete.Serial)
			for _, nic := range existingData.Nics {
				if !discrete.HasNic(nic.MacAddress) {
					db.Delete(&nic)
				}
			}

			for _, psu := range existingData.Psus {
				if !discrete.HasPsu(psu.Serial) {
					db.Delete(&psu)
				}
			}
		}
	}

	return nil
}

func collectBmcChassis(bmc devices.BmcChassis) (err error) {
	defer bmc.Logout()

	if !bmc.IsActive() {
		return err
	}

	db := storage.InitDB()

	ch, err := bmc.ChassisSnapshot()
	if err != nil {
		return err
	}

	chassis := model.NewChassisFromDevice(ch)
	scans := []model.ScannedPort{}
	db.Where("ip = ?", chassis.BmcAddress).Find(&scans)
	for _, scan := range scans {
		if scan.Port == 443 && scan.Protocol == "tcp" && scan.State == "open" {
			chassis.BmcWEBReachable = true
		} else if scan.Port == 22 && scan.Protocol == "tcp" && scan.State == "open" {
			chassis.BmcSSHReachable = true
		}
	}

	for _, blade := range chassis.Blades {
		if conn, err := discover.ScanAndConnect(blade.BmcAddress, viper.GetString("bmc_user"), viper.GetString("bmc_pass")); err == nil {
			if b, ok := conn.(devices.Bmc); ok {
				err = b.Login()
				if err == errors.ErrLoginFailed {
					b.UpdateCredentials(
						viper.GetString(fmt.Sprintf("collector.default.%s.username", b.Vendor())),
						viper.GetString(fmt.Sprintf("collector.default.%s.password", b.Vendor())),
					)
					err = b.Login()
					if err != nil {
						log.WithFields(log.Fields{"operation": "connection", "ip": blade.BmcAddress}).Error(err)
						continue
					}
				} else if err != nil {
					log.WithFields(log.Fields{"operation": "connection", "ip": blade.BmcAddress}).Error(err)
					continue
				}

				blade.BmcAuth = true
				blade.BmcWEBReachable = true

				scans := []model.ScannedPort{}
				db.Where("ip = ?", blade.BmcAddress).Find(&scans)
				for _, scan := range scans {
					if scan.Port == 22 && scan.Protocol == "tcp" && scan.State == "open" {
						blade.BmcSSHReachable = true
					} else if scan.Port == 443 && scan.Protocol == "tcp" && scan.State == "open" {
						blade.BmcWEBReachable = true
					} else if scan.Port == 623 && scan.Protocol == "ipmi" && scan.State == "open" {
						blade.BmcIpmiReachable = true
					}
				}

				blade.BmcType, err = b.BmcType()
				if err != nil {
					log.WithFields(log.Fields{"operation": "bmc type", "ip": blade.BmcAddress, "name": blade.Name, "serial": blade.Serial, "type": "chassis"}).Warning(err)
				}

				blade.Processor, blade.ProcessorCount, blade.ProcessorCoreCount, blade.ProcessorThreadCount, err = b.CPU()
				if err != nil {
					log.WithFields(log.Fields{"operation": "reading cpu data", "ip": blade.BmcAddress, "name": blade.Name, "serial": blade.Serial, "type": "chassis"}).Warning(err)
				}

				blade.Memory, err = b.Memory()
				if err != nil {
					log.WithFields(log.Fields{"operation": "reading memory data", "ip": blade.BmcAddress, "serial": blade.Serial, "type": "chassis"}).Warning(err)
				}

				blade.BmcLicenceType, blade.BmcLicenceStatus, err = b.License()
				if err != nil {
					log.WithFields(log.Fields{"operation": "reading license data", "ip": blade.BmcAddress, "serial": blade.Serial, "type": "chassis"}).Warning(err)
				}

				if len(blade.Nics) == 0 {
					nics, err := b.Nics()
					if err != nil {
						log.WithFields(log.Fields{"operation": "reading nice", "ip": blade.BmcAddress, "serial": blade.Serial, "type": "chassis"}).Warning(err)
					} else {
						for _, nic := range nics {
							blade.Nics = append(blade.Nics, &model.Nic{
								MacAddress:  nic.MacAddress,
								Name:        nic.Name,
								BladeSerial: blade.Serial,
							})
						}
					}
				}

				if len(blade.Disks) == 0 {
					disks, err := b.Disks()
					if err != nil {
						log.WithFields(log.Fields{"operation": "reading disks", "ip": blade.BmcAddress, "serial": blade.Serial, "type": "chassis"}).Warning(err)
					} else {
						for _, disk := range disks {
							blade.Disks = append(blade.Disks, &model.Disk{
								Serial:      disk.Serial,
								Size:        disk.Size,
								Status:      disk.Status,
								Model:       disk.Model,
								Type:        disk.Type,
								FwVersion:   disk.FwVersion,
								BladeSerial: blade.Serial,
							})
						}
					}
				}
			}
		}
	}

	chassisStorage := storage.NewChassisStorage(db)
	existingData, err := chassisStorage.GetOne(chassis.Serial)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	_, err = chassisStorage.UpdateOrCreate(chassis)
	if err != nil {
		return err
	}

	if len(chassis.Diff(&existingData)) != 0 {
		notifyChange <- fmt.Sprintf("%s/%s/%s", viper.GetString("url"), "chassis", chassis.Serial)
		for _, blade := range existingData.Blades {
			if !chassis.HasBlade(blade.Serial) {
				db.Delete(&blade)
			}
		}

		for _, storageBlade := range existingData.StorageBlades {
			if !chassis.HasStorageBlade(storageBlade.Serial) {
				db.Delete(&storageBlade)
			}
		}

		for _, nic := range existingData.Nics {
			if !chassis.HasNic(nic.MacAddress) {
				db.Delete(&nic)
			}
		}

		for _, psu := range existingData.Psus {
			if !chassis.HasNic(psu.Serial) {
				db.Delete(&psu)
			}
		}
	}

	return nil
}
