module github.com/gpsingh-1991/cluster-api-provider-metamorph

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/go-resty/resty/v2 v2.0.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	k8s.io/api v0.17.8
	k8s.io/apimachinery v0.17.8
	k8s.io/client-go v0.17.8
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/cluster-api v0.3.7
	sigs.k8s.io/controller-runtime v0.5.8
)
