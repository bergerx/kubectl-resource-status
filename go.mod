module github.com/bergerx/kubectl-status

go 1.12

replace github.com/gorilla/rpc v1.2.0+incompatible => github.com/gorilla/rpc v1.2.0 // https://github.com/gorilla/rpc/issues/65#issuecomment-518834577

require (
	github.com/Masterminds/sprig/v3 v3.0.2
	github.com/coreos/go-etcd v2.0.0+incompatible // indirect
	github.com/cpuguy83/go-md2man v1.0.10 // indirect
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0
	github.com/gophercloud/gophercloud v0.1.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/rakyll/statik v0.1.7
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.0
	github.com/ugorji/go/codec v0.0.0-20181204163529-d75b2dcb6bc8 // indirect
	gonum.org/v1/netlib v0.0.0-20190331212654-76723241ea4e // indirect
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/cli-runtime v0.21.2
	k8s.io/client-go v0.21.2
	k8s.io/klog v1.0.0 // indirect
	k8s.io/kubectl v0.21.2
	k8s.io/metrics v0.21.2
	sigs.k8s.io/kustomize v2.0.3+incompatible // indirect
	sigs.k8s.io/structured-merge-diff v0.0.0-20190525122527-15d366b2352e // indirect
	sigs.k8s.io/yaml v1.2.0
)
