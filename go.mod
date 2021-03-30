module github.com/amaizfinance/redis-operator

go 1.16

require (
	github.com/cenkalti/backoff/v3 v3.0.0
	github.com/go-openapi/spec v0.19.4
	github.com/go-redis/redis v6.15.5+incompatible
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/spf13/cast v1.3.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20200414173820-0848c9571904
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20200121204235-bf4fb3bd569c
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
)
