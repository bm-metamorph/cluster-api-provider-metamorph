module github.com/metamorph/cluster-api-provider-metamorph

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/go-resty/resty/v2 v2.3.0
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/pkg/errors v0.9.1
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/cluster-api v0.3.6
	sigs.k8s.io/controller-runtime v0.5.2
)
