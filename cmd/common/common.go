package common

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/labstack/echo"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/v1alpha1"
	"github.com/weaveworks/libgitops/cmd/sample-app/version"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

var (
	CarGVK = v1alpha1.SchemeGroupVersion.WithKind("Car")
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewCar(name string) *v1alpha1.Car {
	obj := &v1alpha1.Car{}
	obj.Name = name
	obj.Namespace = "default"
	obj.Spec.Brand = fmt.Sprintf("Acura-%03d", rand.Intn(1000))

	return obj
}

func SetNewCarStatus(ctx context.Context, c client.Client, name string) error {
	car := &v1alpha1.Car{}
	err := c.Get(ctx, core.ObjectKey{Name: name}, car)
	if err != nil {
		return err
	}

	car.Status.Distance = rand.Uint64()
	car.Status.Speed = rand.Float64() * 100

	return c.Update(ctx, car)
}

func ParseVersionFlag() {
	var showVersion bool

	pflag.BoolVar(&showVersion, "version", showVersion, "Show version information and exit")
	pflag.Parse()
	if showVersion {
		fmt.Printf("sample-app version: %#v\n", version.Get())
		os.Exit(0)
	}
}

func NewEcho() *echo.Echo {
	// Set up the echo server
	e := echo.New()
	e.Debug = true
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Welcome!")
	})
	return e
}

func StartEcho(e *echo.Echo) error {
	// Start the server
	go func() {
		if err := e.Start(":8881"); err != nil {
			e.Logger.Info("shutting down the server", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	// Wait for interrupt signal to gracefully shutdown the application with a timeout of 10 seconds
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return e.Shutdown(ctx)
}
