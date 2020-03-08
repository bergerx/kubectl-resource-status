package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
	_ "unsafe"

	"github.com/Masterminds/sprig/v3"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/resource"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var funcMap = template.FuncMap{
	"green":                         color.GreenString,
	"greenBold":                     color.New(color.FgGreen, color.Bold).SprintfFunc(),
	"yellow":                        color.YellowString,
	"yellowBold":                    color.New(color.FgYellow, color.Bold).SprintfFunc(),
	"red":                           color.RedString,
	"redBold":                       color.New(color.FgRed, color.Bold).SprintfFunc(),
	"cyan":                          color.CyanString,
	"cyanBold":                      color.HiCyanString,
	"bold":                          color.New(color.Bold).SprintfFunc(),
	"colorAgo":                      colorAgo,
	"colorDuration":                 colorDuration,
	"markRed":                       markRed,
	"markYellow":                    markYellow,
	"redIf":                         redIf,
	"dateSub":                       dateSub,
	"conditionStatusColor":          conditionStatusColor,
	"colorPodQos":                   colorPodQos,
	"colorPhase":                    colorPhase,
	"colorReason":                   colorReason,
	"colorContainerTerminateReason": colorContainerTerminateReason,
	"colorExitCode":                 colorExitCode,
	"signalName":                    signalName,
}

func conditionStatusColor(condition map[string]interface{}, str string) string {
	switch {
	/*
		From https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties:

		> Condition types should indicate state in the "abnormal-true" polarity. For example, if the condition indicates
		> when a policy is invalid, the "is valid" case is probably the norm, so the condition should be called
		> "Invalid".

		But apparently this is not common among most resources, so we have the list of cases that matches the expected
		behaviour rather than the exceptions.
	*/
	case strings.HasSuffix(fmt.Sprint(condition["type"]), "Pressure"), // Node Pressure conditions
		strings.HasSuffix(fmt.Sprint(condition["type"]), "Unavailable"), // Node NetworkUnavailable condition
		strings.HasPrefix(fmt.Sprint(condition["type"]), "Non"),         // CRD NonStructuralSchema condition
		condition["type"] == "Failed":                                   // Failed Jobs has this condition
		switch condition["status"] {
		case "False":
			return str
		case "True", "Unknown":
			return color.New(color.FgRed, color.Bold).Sprintf(str)
		default:
			return color.New(color.FgRed, color.Bold).Sprintf(str)
		}
	default:
		switch condition["status"] {
		case "True":
			return str
		case "False", "Unknown":
			return color.New(color.FgRed, color.Bold).Sprintf(str)
		default:
			return color.New(color.FgRed, color.Bold).Sprintf(str)
		}
	}
}

func colorContainerTerminateReason(reason string) string {
	switch reason {
	case "OOMKilled", "ContainerCannotRun", "Error":
		return color.New(color.FgRed, color.Bold).Sprint(reason)
	case "Completed":
		return color.GreenString(reason)
	default:
		return reason
	}
}

func dateSub(date1, date2 time.Time) time.Duration {
	return date2.Sub(date1)
}

//go:linkname signame runtime.signame
func signame(sig uint32) string

func signalName(signal int64) string {
	return signame(uint32(signal))
}

func redIf(cond bool, str string) string {
	if cond {
		return color.RedString(str)
	}
	return str
}

func markRed(substr, s string) string {
	return strings.ReplaceAll(s, substr, color.RedString(substr))
}

func markYellow(substr, s string) string {
	return strings.ReplaceAll(s, substr, color.YellowString(substr))
}

func colorExitCode(exitCode int) string {
	switch exitCode {
	case 0:
		return strconv.Itoa(exitCode)
	default:
		return color.RedString("%d", exitCode)
	}
}

func colorPodQos(qos string) string {
	switch qos {
	case "BestEffort":
		return color.RedString(qos)
	case "Burstable":
		return color.YellowString(qos)
	case "Guaranteed":
		return color.GreenString(qos)
	default:
		return color.RedString(qos)
	}
}

func colorPhase(phase string) string {
	/* covers ".status.phase" and ".status.state" for various types, e.g. pod, pv, pvc, svc, ns, etc ... */
	switch phase {
	case "Pending", "Released":
		return color.YellowString(phase)
	/* "valid" is for cert manager orders.status.state */
	case "Running", "Succeeded", "Active", "Available", "Bound", "valid":
		return color.GreenString(phase)
	case "Failed", "Unknown", "Terminating":
		return color.New(color.FgRed, color.Bold).Sprintf(phase)
	default:
		return phase
	}
}

func colorReason(reason string) string {
	/* covers ".status.reason" for various types, e.g. pod, pv, etc ... */
	switch reason {
	case "Evicted":
		return color.RedString(reason)
	default:
		return reason
	}
}

func colorAgo(kubeDate string) string {
	t, _ := time.ParseInLocation("2006-01-02T15:04:05Z", kubeDate, time.Local)
	duration := time.Since(t).Round(time.Second)
	return colorDuration(duration)
}

func colorDuration(duration time.Duration) string {
	durationRound := (sprig.GenericFuncMap()["durationRound"]).(func(duration interface{}) string)
	str := durationRound(duration.String())
	if duration < time.Minute*5 {
		return color.RedString(str)
	}
	if duration < time.Hour {
		return color.YellowString(str)
	}
	if duration < time.Hour*24 {
		return color.MagentaString(str)
	}
	return str
}

func RunPlugin(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	//log := logger.NewLogger()
	//log.Info(strings.Join(args, ","))
	clientConfig := f.ToRawKubeConfigLoader()
	namespace, enforceNamespace, err := clientConfig.Namespace()
	if err != nil {
		return err
	}
	filenames := cmdutil.GetFlagStringSlice(cmd, "filename")

	r := f.NewBuilder().
		Unstructured().
		NamespaceParam(namespace).DefaultNamespace().AllNamespaces(cmdutil.GetFlagBool(cmd, "all-namespaces")).
		FilenameParam(enforceNamespace, &resource.FilenameOptions{Filenames: filenames}).
		LabelSelectorParam(cmdutil.GetFlagString(cmd, "selector")).
		ResourceTypeOrNameArgs(true, args...).
		ContinueOnError().
		Flatten().
		Do()

	err = r.Err()
	if err != nil {
		return err
	}

	var allErrs []error
	infos, infosErr := r.Infos()
	if infosErr != nil {
		allErrs = append(allErrs, err)
	}

	errs := sets.NewString()
	for _, info := range infos {
		var data []byte
		var err error
		obj := info.Object
		data, err = json.Marshal(obj)
		if err != nil {
			if errs.Has(err.Error()) {
				continue
			}
			allErrs = append(allErrs, err)
			errs.Insert(err.Error())
			continue
		}

		out := map[string]interface{}{}
		if err := json.Unmarshal(data, &out); err != nil {
			if errs.Has(err.Error()) {
				continue
			}
			allErrs = append(allErrs, err)
			errs.Insert(err.Error())
			continue
		}
		tmpl := template.Must(template.
			New("templates").
			Funcs(sprig.TxtFuncMap()).
			Funcs(funcMap).
			Parse(`
{{- define "DefaultResource" }}
    {{- template "status_summary_line" . }}
    {{- template "observed_generation_summary" . }}
    {{- template "replicas_status" . }}
    {{- template "conditions_summary" . }}
{{- end }}

{{- define "Pod" }}
    {{- template "status_summary_line" . }}
    {{- with .status.qosClass }} {{ . | colorPodQos }}{{ end }}
    {{- with .status.message }}, message: {{ . }}{{ end }}
    {{- template "conditions_summary" . }}
    {{- with .status.initContainerStatuses }}
  InitContainers:
        {{- range . }}
            {{- template "container_status_summary" . }}
        {{- end }}
    {{- end }}
    {{- with .status.containerStatuses }}
  Containers:
        {{- range . }}
            {{- template "container_status_summary" . }}
        {{- end }}
    {{- end }}
{{- end }}

{{- define "DaemonSet" }}
    {{- template "status_summary_line" . }}
    {{- template "observed_generation_summary" . }}
    {{- template "daemonset_replicas_status" . }}
    {{- template "conditions_summary" . }}
{{- end -}}

{{- define "PersistentVolume" }}
    {{- template "status_summary_line" . }}
    {{- with .status.message }}
  {{ "message" | bold }}: {{ . }}
    {{- end }}
{{- end -}}

{{- define "PersistentVolumeClaim" }}
    {{- template "status_summary_line" . }}
    {{- with .status.capacity.storage }} {{ . }}{{ end }}
{{- end -}}

{{- define "CronJob" }}
    {{- template "status_summary_line" . }}, last ran at {{ .status.lastScheduleTime }} ({{ .status.lastScheduleTime | colorAgo }} ago)
{{- end -}}

{{- define "Job" }}
    {{- template "status_summary_line" . }}
    {{- /* See https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/#parallel-jobs */ -}}
    {{- if eq (coalesce .spec.completions .spec.parallelism 1 | toString) "1" }}
        {{- template "job_non_parallel" . }}
    {{- else if .spec.completions }}
        {{- /* TODO: handle "fixed completion count jobs" better */ -}}
        {{- template "job_parallel" . }}
    {{- else if .spec.parallelism }}
        {{- /* TODO: handle "work queue jobs" better */ -}}
        {{- template "job_parallel" . }}
    {{- end }}
    {{- template "conditions_summary" . }}
{{- end -}}

{{ define "job_non_parallel" }}
    {{- if .status.succeeded }}, {{ "Succeeded" | green }}{{ end }}
    {{- if .status.failed }}, {{ "Failed" | redBold }}{{ end }}
{{- end -}}

{{ define "job_parallel" }}
    TODO: handle parallel jobs  better
    {{- if .status.failed }}, {{ "failed" | redBold }} {{ .status.failed }}/{{ .spec.backoffLimit }} times{{ end }}
{{- end -}}

{{- define "Service" }}
    {{- template "status_summary_line" . }}
    {{- if eq .spec.type "LoadBalancer" }}
        {{- template "load_balancer_ingress" . }}
    {{- end }}
{{- end -}}

{{- define "Ingress" }}
    {{- template "status_summary_line" . }}
    {{- template "load_balancer_ingress" . }}
{{- end -}}

{{- define "HorizontalPodAutoscaler" }}
    {{- template "status_summary_line" . }} last scale was {{ .status.lastScaleTime | colorAgo }} ago
  {{ "current" | bold }} replicas:{{ .status.currentReplicas }}/({{ .spec.minReplicas | default "1" }}-{{ .spec.maxReplicas }})
    {{- if .status.currentCPUUtilizationPercentage }} CPUUtilisation: {{ .status.currentCPUUtilizationPercentage | toString | redIf (ge .status.currentCPUUtilizationPercentage .spec.targetCPUUtilizationPercentage) }}%/{{ .spec.targetCPUUtilizationPercentage }}%{{ end }}
    {{- if (ne .status.currentReplicas .status.desiredReplicas) }}, {{ "desired" | redBold}}: {{ .status.currentReplicas }} --> {{ .status.desiredReplicas }}{{ end }}
{{- end -}}

{{- define "load_balancer_ingress" }}
    {{- if .status.loadBalancer.ingress }}
	    {{- if or (index .status.loadBalancer.ingress 0).hostname (index .status.loadBalancer.ingress 0).ip }}
	        {{- with (index .status.loadBalancer.ingress 0).hostname }} {{ "LoadBalancer" | green }}:{{ . }}{{ end }}
	        {{- with (index .status.loadBalancer.ingress 0).ip }} {{ "LoadBalancer" | green }}:{{ . }}{{ end }}
	    {{- else }} {{ "Pending LoadBalancer" | redBold }}
	    {{- end }}
    {{- end }}
{{- end -}}

{{- define "daemonset_replicas_status" }}
    {{- if .status.desiredNumberScheduled }}{{ $desiredNumberScheduled := .status.desiredNumberScheduled }}
  {{ "desired" | bold}}:{{ .status.desiredNumberScheduled }}
        {{- with .status.currentNumberScheduled }}, current:{{ . | toString | redIf (not ( eq $desiredNumberScheduled . )) }}{{ end }}
        {{- with .status.numberAvailable }}, available:{{ . | toString | redIf (not ( eq $desiredNumberScheduled . )) }}{{ end }}
        {{- with .status.numberReady }}, ready:{{ . | toString | redIf (not ( eq $desiredNumberScheduled . )) }}{{ end }}
        {{- with .status.updatedNumberScheduled }}, updated:{{ . | toString | redIf (not ( eq $desiredNumberScheduled . )) }}{{ end }}
        {{- if gt (.status.numberMisscheduled | int) 0 }}{{ "numberMisscheduled" | redBold }}:{{ .status.numberMisscheduled }}{{- end }}
    {{- end }}
{{- end -}}

{{- define "replicas_status" }}
    {{- if .status.replicas }}{{ $spec_replicas := .spec.replicas }}
  {{ "desired" | bold }}:{{ .spec.replicas }}
        {{- with .status.replicas }}, existing:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.readyReplicas }}, ready:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.currentReplicas }}, current:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.updatedReplicas }}, updated:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.availableReplicas }}, available:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.fullyLabeledReplicas }}, fullyLabeled:{{ . | toString | redIf (not ( eq $spec_replicas . )) }}{{ end }}
        {{- with .status.unavailableReplicas }}, unavailable:{{ . | toString | redBold }}{{ end }}
        {{- with .status.collisionCount }}, collisions:{{ .status.collisionCount | toString | redBold }}{{ end }}
  {{- end }}
{{- end -}}

{{- define "status_summary_line" }}
{{.kind | cyanBold }}/{{ .metadata.name | cyan }}
    {{- with .metadata.namespace }} -n {{ . }}{{ end -}}
    , created {{ .metadata.creationTimestamp | colorAgo }} ago
    {{- if .status.startTime }}
	    {{- $created := .metadata.creationTimestamp | toDate "2006-01-02T15:04:05Z" }}
	    {{- $started := .status.startTime | toDate "2006-01-02T15:04:05Z" }}
	    {{- $startedIn := $created | dateSub $started}}
        {{- if gt ($startedIn.Seconds | int) 0 }}, started after {{ $startedIn.Seconds | ago }}{{ end }}
    {{- end }}
    {{- if .status.completionTime }}
        {{- $started := .status.startTime | toDate "2006-01-02T15:04:05Z" -}}
        {{- $completed := .status.completionTime | toDate "2006-01-02T15:04:05Z" -}}
        {{- $ranfor := $completed.Sub $started }} and {{ "completed" | green }} in {{ $ranfor | colorDuration }}
    {{- end }}
    {{- with .status.phase }} {{ . | colorPhase }}{{ end }}
    {{- /* .status.state is used by Ambassador */ -}}
    {{- with .status.state }} {{ . | colorPhase }}{{ end }}
    {{- with .status.reason }} {{ . | colorReason }}{{ end }}
{{- end -}}

{{- define "observed_generation_summary" }}
    {{- if and .metadata.generation .status.observedGeneration }}
        {{- if ne .metadata.generation .status.observedGeneration }}
  observedGeneration({{ .status.observedGeneration | redBold }}) doesn't match generation({{ .metadata.generation | redBold }})
    {{ "This usually means related controller has not yet reconciled this resource!" | yellow }}
        {{- end }}
    {{- end }}
{{- end -}}

{{- define "conditions_summary" }}
    {{- if .status.conditions }}
        {{- range .status.conditions }}{{ template "condition_summary" . }}{{ end }}
    {{- end }}
{{- end -}}

{{- define "condition_summary" }}
  {{ .type | bold }}:{{ .status | conditionStatusColor . }}{{ $condition := . }}
    {{- with .reason }} {{ .| conditionStatusColor $condition }}{{ end }}
    {{- with .message }}, {{ .| conditionStatusColor $condition }}{{ end }}
    {{- with .lastTransitionTime }} for {{ . | colorAgo }}{{ end }}
    {{- if .lastUpdateTime }}
        {{- if ne (.lastUpdateTime | colorAgo) (.lastTransitionTime | colorAgo) -}}
            , last update was {{ .lastUpdateTime | colorAgo }} ago
	    {{- end }}
    {{- end }}
    {{- if .lastProbeTime}}
        {{- if ne (.lastProbeTime | colorAgo) (.lastTransitionTime | colorAgo) -}}
            , last probe was {{ .lastProbeTime | colorAgo }} ago
        {{- end }}
    {{- end }}
{{- end -}}

{{- define "container_status_summary"}}
    {{ .name | bold }} ({{ .image | markYellow "latest" }}) {{ template "container_state_summary" .state }}
    {{- if .state.running }}{{ if .ready }} and {{ "Ready" | green }}{{ else }} but {{ "Not Ready" | redBold }}{{ end }}{{ end }}
    {{- if gt (.restartCount | int ) 0 }}, {{ printf "restarted %d times" (.restartCount | int) | yellowBold }}{{ end }}
    {{- with .lastState }}
      lastState: {{ template "container_state_summary" . }}
    {{- end }}
{{- end -}}

{{- define "container_state_summary" }}
    {{- /* https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status */}}
    {{- with .waiting }}
        {{- "Waiting" | redBold }} {{ .reason | redBold }}{{ with .message }}: {{ . | redBold }}{{ end }}
    {{- end }}
    {{- with .running }}
        {{- "Running" | green }} for {{ .startedAt | colorAgo }}
    {{- end }}
    {{- with .terminated }}
        {{- if .startedAt }}
            {{- $started := .startedAt | toDate "2006-01-02T15:04:05Z" -}}
            {{- $finished := .finishedAt | toDate "2006-01-02T15:04:05Z" -}}
            {{- $ranfor := $finished.Sub $started -}}
        Started {{ .startedAt | colorAgo }} ago and {{ if .reason }}{{ .reason | colorContainerTerminateReason }}{{ else }}terminated{{- end }} after {{ $ranfor | colorDuration }}
            {{- if .exitCode }} with exit code {{ template "exit_code_summary" . }}{{ end }}
        {{- else }}
            {{ template "exit_code_summary" . }}
        {{- end }}
    {{- end }}
{{- end -}}

{{- define "exit_code_summary" }}
    {{- .exitCode | toString | redIf (ne (.exitCode | toString) "0" ) }}
    {{- with .signal }} (signal: {{ . }}){{ end }}
    {{- if and (gt (.exitCode | int) 128) (le (.exitCode | int) 165) }} ({{ sub (.exitCode | int) 128 | signalName }}) {{ end }}
{{- end -}}

{{- define "Node" }}
    {{- template "status_summary_line" . }}
  {{ .status.nodeInfo.operatingSystem | bold }} {{ .status.nodeInfo.osImage }} ({{ .status.nodeInfo.architecture }}), kernel {{ .status.nodeInfo.kernelVersion }}, kubelet {{ .status.nodeInfo.kubeletVersion }}, kube-proxy {{ .status.nodeInfo.kubeProxyVersion }}
  cpu: {{ .status.allocatable.cpu }}, mem: {{ .status.allocatable.memory }} mem, ephemeral-storage: {{index .status.allocatable "ephemeral-storage"}}
    {{- if or (index .metadata.labels "node.kubernetes.io/instance") (index .metadata.labels "topology.kubernetes.io/region") (index .metadata.labels "failure-domain.beta.kubernetes.io/region") (index .metadata.labels "topology.kubernetes.io/zone") (index .metadata.labels "failure-domain.beta.kubernetes.io/region") }}
  {{ "cloudprovider" | bold }}
        {{- with index .metadata.labels "topology.kubernetes.io/region" | default (index .metadata.labels "failure-domain.beta.kubernetes.io/region") }} {{ . }}{{ end }}
        {{- with index .metadata.labels "topology.kubernetes.io/zone" | default (index .metadata.labels "failure-domain.beta.kubernetes.io/zone") }}{{ . }}{{ end }}
        {{- with index .metadata.labels "node.kubernetes.io/instance" | default (index .metadata.labels "beta.kubernetes.io/instance-type") }} {{ . }}{{ end }}
        {{- with .metadata.labels.agentpool }}, agentpool:{{ . }}{{ end }}
        {{- with index .metadata.labels "kubernetes.io/role" }}, role:{{ . }}{{ end }}
    {{- end }}
  {{ "images" | bold }} {{ .status.images | len }}.
    {{- if .status.volumesInUse }} {{ "volumes" | bold }} inuse={{ .status.volumesInUse | len }}
        {{- with index .status.allocatable "attachable-volumes-azure-disk" }}/{{ . }}{{ end }}, attached={{ .status.volumesAttached | len }}
    {{- end}}
    {{- template "conditions_summary" . }}
{{- end -}}
`))
		kind := info.ResourceMapping().GroupVersionKind.Kind
		var kindTemplateName string
		if t := tmpl.Lookup(kind); t != nil {
			kindTemplateName = kind
		} else {
			kindTemplateName = "DefaultResource"
		}
		err = tmpl.ExecuteTemplate(os.Stderr, kindTemplateName, out)
		if err != nil {
			if errs.Has(err.Error()) {
				continue
			}
			allErrs = append(allErrs, err)
			errs.Insert(err.Error())
			continue
		}
	}
	return utilerrors.NewAggregate(allErrs)
}
