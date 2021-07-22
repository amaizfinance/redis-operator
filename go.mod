module github.com/amaizfinance/redis-operator

go 1.16

require (
	github.com/cenkalti/backoff/v3 v3.0.0
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/go-redis/redis v6.15.5+incompatible
	github.com/operator-framework/operator-sdk v0.19.4
	github.com/spf13/cast v1.3.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	k8s.io/api v0.19.13
	k8s.io/apiextensions-apiserver v0.19.13 // indirect
	k8s.io/apimachinery v0.19.13
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20210527164424-3c818078ee3d
	sigs.k8s.io/controller-runtime v0.6.5
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.4.0
	k8s.io/client-go => k8s.io/client-go v0.19.13 // Required by prometheus-operator
)
