package web

import (
	"fmt"
	"html/template"
	"log"

	"github.com/GeertJohan/go.rice"
	"github.com/manyminds/api2go"
	"github.com/manyminds/api2go-adapter/gingonic"
	"gitlab.booking.com/go/dora/model"
	"gitlab.booking.com/go/dora/resource"
	"gitlab.booking.com/go/dora/storage"

	gin "gopkg.in/gin-gonic/gin.v1"
)

// RunGin is responsible to spin up the gin webservice
func RunGin(port int, debug bool) {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	templateBox, err := rice.FindBox("templates")
	if err != nil {
		log.Fatal(err)
	}

	staticBox, err := rice.FindBox("static")
	if err != nil {
		log.Fatal(err)
	}

	doc, err := template.New("doc.tmpl").Parse(templateBox.MustString("doc.tmpl"))
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	r.SetHTMLTemplate(doc)
	r.StaticFS("/static", staticBox.HTTPBox())
	api := api2go.NewAPIWithRouting(
		"v1",
		api2go.NewStaticResolver("/"),
		gingonic.New(r),
	)

	db := storage.InitDB()
	defer db.Close()

	chassisStorage := storage.NewChassisStorage(db)
	bladeStorage := storage.NewBladeStorage(db)
	discreteStorage := storage.NewDiscreteStorage(db)
	nicStorage := storage.NewNicStorage(db)
	storageBladeStorage := storage.NewStorageBladeStorage(db)
	scannedPortStorage := storage.NewScannedPortStorage(db)
	psuStorage := storage.NewPsuStorage(db)
	diskStorage := storage.NewDiskStorage(db)

	api.AddResource(model.Chassis{}, resource.ChassisResource{ChassisStorage: chassisStorage})
	api.AddResource(model.Blade{}, resource.BladeResource{BladeStorage: bladeStorage})
	api.AddResource(model.Discrete{}, resource.DiscreteResource{DiscreteStorage: discreteStorage})
	api.AddResource(model.StorageBlade{}, resource.StorageBladeResource{StorageBladeStorage: storageBladeStorage})
	api.AddResource(model.Nic{}, resource.NicResource{NicStorage: nicStorage})
	api.AddResource(model.ScannedPort{}, resource.ScannedPortResource{ScannedPortStorage: scannedPortStorage})
	api.AddResource(model.Psu{}, resource.PsuResource{PsuStorage: psuStorage})
	api.AddResource(model.Disk{}, resource.DiskResource{DiskStorage: diskStorage})

	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "doc.tmpl", gin.H{})
	})

	r.GET("/doc", func(c *gin.Context) {
		c.HTML(200, "doc.tmpl", gin.H{})
	})

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/ping_db", func(c *gin.Context) {
		err := db.DB().Ping()
		if err == nil {
			c.String(200, "pong")
		} else {
			c.String(451, "database has gone away")
		}
	})

	r.Run(fmt.Sprintf(":%d", port))
}
