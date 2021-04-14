module github.com/weaveworks/libgitops

go 1.15

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible

require (
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/fluxcd/go-git-providers v0.0.3
	github.com/fluxcd/pkg/ssh v0.0.5
	github.com/go-git/go-git/v5 v5.2.0
	github.com/go-openapi/spec v0.20.0
	github.com/google/btree v1.0.0
	github.com/google/go-github/v32 v32.1.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/afero v1.2.2
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	golang.org/x/sys v0.0.0-20210108172913-0df2131ae363
	k8s.io/apimachinery v0.19.6
	k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6
	k8s.io/utils v0.0.0-20200912215256-4140de9c8800
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/kustomize/kyaml v0.10.5
)
