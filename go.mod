module github.com/Fred78290/kubernetes-multipass-autoscaler

go 1.15

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20201202161906-c7110b5ffcbb // indirect
	golang.org/x/sys v0.0.0-20201202213521-69691e467435 // indirect
	golang.org/x/text v0.3.4 // indirect
	google.golang.org/genproto v0.0.0-20201203001206-6486ece9c497 // indirect
	google.golang.org/grpc v1.34.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4 // indirect
	k8s.io/klog/v2 v2.4.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.0.2 // indirect
)

replace google.golang.org/grpc => google.golang.org/grpc v1.27.0

replace k8s.io/api => k8s.io/api v0.18.12
