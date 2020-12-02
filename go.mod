module github.com/Fred78290/kubernetes-multipass-autoscaler

go 1.15

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.2
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.4.0
	google.golang.org/grpc v1.27.0
	google.golang.org/protobuf v1.24.0
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.19.0
)

replace google.golang.org/grpc => google.golang.org/grpc v1.27.0

replace k8s.io/api => k8s.io/api v0.19.0
