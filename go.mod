module github.com/Fred78290/kubernetes-multipass-autoscaler

go 1.14

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.3
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20200501053045-e0ff5e5a1de5 // indirect
	golang.org/x/sys v0.0.0-20200501145240-bc7a7d42d5c3 // indirect
	google.golang.org/genproto v0.0.0-20200430143042-b979b6f78d84 // indirect
	google.golang.org/grpc v1.27.0
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.2
)

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

replace k8s.io/api => k8s.io/api v0.18.2
