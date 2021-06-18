module github.com/bergerx/kubectl-status

go 1.12

replace github.com/gorilla/rpc v1.2.0+incompatible => github.com/gorilla/rpc v1.2.0 // https://github.com/gorilla/rpc/issues/65#issuecomment-518834577

require (
	github.com/Masterminds/sprig/v3 v3.0.2
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/rakyll/statik v0.1.7
	github.com/spf13/cobra v0.0.7
	github.com/spf13/viper v1.6.3
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/cli-runtime v0.17.4
	k8s.io/client-go v0.17.4
	k8s.io/kubectl v0.17.4
	k8s.io/metrics v0.17.4
	sigs.k8s.io/yaml v1.2.0
)
