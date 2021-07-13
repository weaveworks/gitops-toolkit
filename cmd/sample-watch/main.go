package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/common"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/pkg/logs"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"github.com/weaveworks/libgitops/pkg/storage/watch/update"
)

var watchDirFlag = pflag.String("watch-dir", "/tmp/libgitops/watch", "Where to watch for YAML/JSON manifests")

func main() {
	// Parse the version flag
	common.ParseVersionFlag()

	// Run the application
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// Create the watch directory
	if err := os.MkdirAll(*watchDirFlag, 0755); err != nil {
		return err
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.InfoLevel)

	watchStorage, err := watch.NewManifestStorage(*watchDirFlag, scheme.Serializer)
	if err != nil {
		return err
	}
	defer func() { _ = watchStorage.Close() }()

	updates := make(chan update.Update, 4096)
	watchStorage.SetUpdateStream(updates)

	go func() {
		for upd := range updates {
			logrus.Infof("Got %s update for: %v %v", upd.Event, upd.PartialObject.GetObjectKind().GroupVersionKind(), upd.PartialObject.GetObjectMeta())
		}
	}()

	e := common.NewEcho()

	e.GET("/watch/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj, err := watchStorage.Get(common.CarKeyForName(name))
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(frame.NewJSONWriter(&content), obj); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.PUT("/watch/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := common.SetNewCarStatus(watchStorage, common.CarKeyForName(name)); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	return common.StartEcho(e)
}
