module github.com/weaveworks/libgitops

go 1.16

replace (
	github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
	// Keep this version in sync with the Kubernetes version by looking at
	// https://github.com/kubernetes/apimachinery/blob/v0.21.2/go.mod#L17
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
)

require (
	github.com/fluxcd/go-git-providers v0.0.2
	github.com/fluxcd/toolkit v0.0.1-beta.2
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-openapi/spec v0.19.8
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v32 v32.1.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	// Keep all OTel imports the same version
	go.opentelemetry.io/otel v1.0.0-RC1
	go.opentelemetry.io/otel/exporters/jaeger v1.0.0-RC1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.0.0-RC1
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.0.0-RC1
	go.opentelemetry.io/otel/sdk v1.0.0-RC1
	go.opentelemetry.io/otel/trace v1.0.0-RC1
	go.uber.org/multierr v1.6.0
	golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
	// Use the latest available Kubernetes version.
	k8s.io/apimachinery v0.21.2
	// Keep this in sync with the Kubernetes version by checking
	// https://github.com/kubernetes/apimachinery/blob/v0.21.2/go.mod
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	// Keep this in sync with the Kubernetes version by checking
	// https://github.com/kubernetes/kubernetes/blob/v1.21.2/go.mod#L527
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	// Keep this in sync with Kubernetes by checking what controller-runtime
	// version uses the right Kubernetes version, e.g.
	// https://github.com/kubernetes-sigs/controller-runtime/blob/v0.9.2/go.mod
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/kustomize/kyaml v0.1.11
	// Keep this in sync with Kubernetes by checking
	// https://github.com/kubernetes/apimachinery/blob/v0.21.2/go.mod#L40
	sigs.k8s.io/yaml v1.2.0
)
