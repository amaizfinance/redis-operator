module github.com/amaizfinance/redis-operator

go 1.15

require (
	github.com/cenkalti/backoff/v3 v3.0.0
	github.com/go-openapi/spec v0.19.4
	github.com/go-redis/redis v6.15.5+incompatible
	github.com/operator-framework/operator-sdk v0.17.1
	github.com/spf13/cast v1.3.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
