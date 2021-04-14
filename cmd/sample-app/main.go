package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/weaveworks/libgitops/cmd/common"
	"github.com/weaveworks/libgitops/cmd/common/logs"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/scheme"
	"github.com/weaveworks/libgitops/cmd/sample-app/apis/sample/v1alpha1"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var manifestDirFlag = pflag.String("data-dir", "/tmp/libgitops/manifest", "Where to store the YAML files")

func main() {
	// Parse the version flag
	common.ParseVersionFlag()

	// Run the application
	if err := run(*manifestDirFlag); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(manifestDir string) error {
	ctx := context.Background()
	// Create the manifest directory
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return err
	}

	// Set the log level
	logs.Logger.SetLevel(logrus.InfoLevel)

	s, err := filesystem.NewSimpleStorage(
		manifestDir,
		storage.StaticNamespacer{NamespacedIsDefaultPolicy: false},
		filesystem.SimpleFileFinderOptions{
			DisableGroupDirectory: true,
			ContentType:           serializer.ContentTypeYAML,
		},
	)
	if err != nil {
		return err
	}

	// Just use default encoders and decoders
	encoder := scheme.Serializer.Encoder()
	decoder := scheme.Serializer.Decoder()

	// Use the version information in the scheme to determine the storage version
	versioner := backend.SchemePreferredVersioner{Scheme: scheme.Scheme}

	b, err := backend.NewGeneric(s, encoder, decoder, kube.NewNamespaceEnforcer(), versioner, nil)
	if err != nil {
		return err
	}

	plainClient, err := client.NewGeneric(b)
	if err != nil {
		return err
	}

	e := common.NewEcho()

	e.GET("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj := &v1alpha1.Car{}
		err := plainClient.Get(ctx, core.ObjectKey{Name: name}, obj)
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), obj); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.GET("/meta/", func(c echo.Context) error {
		list := &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind("CarList"))
		err := plainClient.List(ctx, list)
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), list); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.GET("/meta/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind("Car"))
		err := plainClient.Get(ctx, core.ObjectKey{
			Name: name,
		}, obj)
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), obj); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, content.Bytes())
	})

	e.GET("/unstructured/", func(c echo.Context) error {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind("CarList"))
		err := plainClient.List(ctx, list)
		if err != nil {
			return err
		}
		var content bytes.Buffer
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), list); err != nil {
			return err
		}
		var newcontent bytes.Buffer
		if err := json.Indent(&newcontent, content.Bytes(), "", "  "); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, newcontent.Bytes())
	})

	e.GET("/unstructured/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind("Car"))
		err := plainClient.Get(ctx, core.ObjectKey{
			Name: name,
		}, obj)
		if err != nil {
			return err
		}
		var content bytes.Buffer
		// This does for some reason not pretty-encode the output
		if err := scheme.Serializer.Encoder().Encode(serializer.NewJSONFrameWriter(&content), obj); err != nil {
			return err
		}
		var newcontent bytes.Buffer
		if err := json.Indent(&newcontent, content.Bytes(), "", "  "); err != nil {
			return err
		}
		return c.JSONBlob(http.StatusOK, newcontent.Bytes())
	})

	e.POST("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := plainClient.Create(ctx, common.NewCar(name)); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	e.PUT("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		if err := common.SetNewCarStatus(ctx, plainClient, name); err != nil {
			return err
		}
		return c.String(200, "OK!")
	})

	e.PATCH("/plain/:name", func(c echo.Context) error {
		name := c.Param("name")
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Please set name")
		}

		body, err := ioutil.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		c.Request().Body.Close()

		car := &v1alpha1.Car{}
		err = plainClient.Get(ctx, core.ObjectKey{
			Name: name,
		}, car)
		if err != nil {
			return err
		}

		if err := plainClient.Patch(ctx, car, ctrlclient.RawPatch(types.MergePatchType, body)); err != nil {
			return err
		}

		return c.JSON(200, car)
	})

	return common.StartEcho(e)
}

/*
type noNamespacesRESTMapper struct{}

func (noNamespacesRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{Scope: meta.RESTScopeRoot}, nil
}*/
