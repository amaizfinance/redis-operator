module github.com/amaizfinance/redis-operator

go 1.16

require (
	github.com/cenkalti/backoff/v3 v3.0.0
	github.com/go-redis/redis v6.15.5+incompatible
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/onsi/gomega v1.15.0 // indirect
	github.com/operator-framework/operator-lib v0.9.0
	github.com/spf13/cast v1.3.1
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20210527164424-3c818078ee3d
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/structured-merge-diff/v4 v4.1.0 // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.4.0
	github.com/operator-framework/operator-sdk => github.com/operator-framework/operator-sdk v1.13.0
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/apiserver => k8s.io/apiserver v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2 // Required by prometheus-operator
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.8.3
)
