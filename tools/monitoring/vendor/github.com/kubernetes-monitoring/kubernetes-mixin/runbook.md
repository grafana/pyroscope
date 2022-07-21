# Kubernetes Alert Runbooks

As Rob Ewaschuk [puts it](https://docs.google.com/document/d/199PqyG3UsyXlwieHaqbGiWVa8eMWi8zzAn0YfcApr8Q/edit#):
> Playbooks (or runbooks) are an important part of an alerting system; it's best to have an entry for each alert or family of alerts that catch a symptom, which can further explain what the alert means and how it might be addressed.

It is a recommended practice that you add an annotation of "runbook" to every prometheus alert with a link to a clear description of it's meaning and suggested remediation or mitigation. While some problems will require private and custom solutions, most common problems have common solutions. In practice, you'll want to automate many of the procedures (rather than leaving them in a wiki), but even a self-correcting problem should provide an explanation as to what happened and why to observers.

Matthew Skelton & Rob Thatcher have an excellent [run book template](https://github.com/SkeltonThatcher/run-book-template). This template will help teams to fully consider most aspects of reliably operating most interesting software systems, if only to confirm that "this section definitely does not apply here" - a valuable realization.

This page collects this repositories alerts and begins the process of describing what they mean and how it might be addressed. Links from alerts to this page are added [automatically](https://github.com/kubernetes-monitoring/kubernetes-mixin/blob/master/lib/add-runbook-links.libsonnet).

### Group Name: "kubernetes-absent"
##### Alert Name: "KubeAPIDown"
+ *Message*: `KubeAPI has disappeared from Prometheus target discovery.`
+ *Severity*: critical
##### Alert Name: "KubeControllerManagerDown"
+ *Message*: `KubeControllerManager has disappeared from Prometheus target discovery.`
+ *Severity*: critical
+ *Runbook*: [Link](https://coreos.com/tectonic/docs/latest/troubleshooting/controller-recovery.html#recovering-a-controller-manager)
##### Alert Name: KubeSchedulerDown
+ *Message*: `KubeScheduler has disappeared from Prometheus target discovery`
+ *Severity*: critical
+ *Runbook*: [Link](https://coreos.com/tectonic/docs/latest/troubleshooting/controller-recovery.html#recovering-a-scheduler)
##### Alert Name: KubeletDown
+ *Message*: `Kubelet has disappeared from Prometheus target discovery.`
+ *Severity*: critical
### Group Name: kubernetes-apps
##### Alert Name: KubePodCrashLooping
+ *Message*: `{{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }}) is restarting {{ printf \"%.2f\" $value }} / second`
+ *Severity*: warning
##### Alert Name: "KubePodNotReady"
+ *Message*: `{{ $labels.namespace }}/{{ $labels.pod }} is not ready.`
+ *Severity*: warning
##### Alert Name: "KubeDeploymentGenerationMismatch"
+ *Message*: `Deployment {{ $labels.namespace }}/{{ $labels.deployment }} generation mismatch`
+ *Severity*: warning
##### Alert Name: "KubeDeploymentReplicasMismatch"
+ *Message*: `Deployment {{ $labels.namespace }}/{{ $labels.deployment }} replica mismatch`
+ *Severity*: warning
##### Alert Name: "KubeStatefulSetReplicasMismatch"
+ *Message*: `StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} replica mismatch`
+ *Severity*: warning
##### Alert Name: "KubeStatefulSetGenerationMismatch"
+ *Message*: `StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} generation mismatch`
+ *Severity*: warning
##### Alert Name: "KubeDaemonSetRolloutStuck"
+ *Message*: `Only {{$value | humanizePercentage }} of desired pods scheduled and ready for daemon set {{$labels.namespace}}/{{$labels.daemonset}}`
+ *Severity*: warning
##### Alert Name: "KubeContainerWaiting"
+ *Message*: `{{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }}) is in waiting state.`
+ *Severity*: warning
##### Alert Name: "KubeDaemonSetNotScheduled"
+ *Message*: `A number of pods of daemonset {{$labels.namespace}}/{{$labels.daemonset}} are not scheduled.`
+ *Severity*: warning

##### Alert Name: "KubeDaemonSetMisScheduled"
+ *Message*: `A number of pods of daemonset {{$labels.namespace}}/{{$labels.daemonset}} are running where they are not supposed to run.`
+ *Severity*: warning

##### Alert Name: "KubeJobNotCompleted"
+ *Message*: `Job {{ $labels.namespace }}/{{ $labels.job_name }} is taking more than {{ "%(kubeJobTimeoutDuration)s" | humanizeDuration }} to complete.`
+ *Severity*: warning
+ *Action*: Check the job using `kubectl describe job <job>` and look at the pod logs using `kubectl logs <pod>` for further information.

##### Alert Name: "KubeJobFailed"
+ *Message*: `Job {{ $labels.namespace }}/{{ $labels.job_name }} failed to complete.`
+ *Severity*: warning
+ *Action*: Check the job using `kubectl describe job <job>` and look at the pod logs using `kubectl logs <pod>` for further information.

### Group Name: "kubernetes-resources"
##### Alert Name: "KubeCPUOvercommit"
+ *Message*: `Cluster has overcommitted CPU resource requests for Pods and cannot tolerate node failure.`
+ *Severity*: warning
##### Alert Name: "KubeMemOvercommit"
+ *Message*: `Cluster has overcommitted memory resource requests for Pods and cannot tolerate node failure.`
+ *Severity*: warning
##### Alert Name: "KubeCPUQuotaOvercommit"
+ *Message*: `Cluster has overcommitted CPU resource requests for Namespaces.`
+ *Severity*: warning
##### Alert Name: "KubeMemQuotaOvercommit"
+ *Message*: `Cluster has overcommitted memory resource requests for Namespaces.`
+ *Severity*: warning
##### Alert Name: "KubeQuotaAlmostFull"
+ *Message*: `{{ $value | humanizePercentage }} usage of {{ $labels.resource }} in namespace {{ $labels.namespace }}.`
+ *Severity*: info
##### Alert Name: "KubeQuotaFullyUsed"
+ *Message*: `{{ $value | humanizePercentage }} usage of {{ $labels.resource }} in namespace {{ $labels.namespace }}.`
+ *Severity*: info
##### Alert Name: "KubeQuotaExceeded"
+ *Message*: `{{ $value | humanizePercentage }} usage of {{ $labels.resource }} in namespace {{ $labels.namespace }}.`
+ *Severity*: warning
### Group Name: "kubernetes-storage"
##### Alert Name: "KubePersistentVolumeFillingUp"
+ *Message*: `The persistent volume claimed by {{ $labels.persistentvolumeclaim }} in namespace {{ $labels.namespace }} has {{ $value | humanizePercentage }} free.`
+ *Severity*: critical
##### Alert Name: "KubePersistentVolumeFillingUp"
+ *Message*: `Based on recent sampling, the persistent volume claimed by {{ $labels.persistentvolumeclaim }} in namespace {{ $labels.namespace }} is expected to fill up within four days.`
+ *Severity*: warning
### Group Name: "kubernetes-system"
##### Alert Name: "KubeNodeNotReady"
+ *Message*: `{{ $labels.node }} has been unready for more than 15 minutes."`
+ *Severity*: warning
##### Alert Name: "KubeNodeUnreachable"
+ *Message*: `{{ $labels.node }} is unreachable and some workloads may be rescheduled.`
+ *Severity*: warning
##### Alert Name: "KubeletTooManyPods"
+ *Message*: `Kubelet '{{ $labels.node }}' is running at {{ $value | humanizePercentage }} of its Pod capacity.`
+ *Severity*: info
##### Alert Name: "KubeNodeReadinessFlapping"
+ *Message*: `The readiness status of node {{ $labels.node }} has changed {{ $value }} times in the last 15 minutes.`
+ *Severity*: warning
##### Alert Name: "KubeletPlegDurationHigh"
+ *Message*: `The Kubelet Pod Lifecycle Event Generator has a 99th percentile duration of {{ $value }} seconds on node {{ $labels.node }}.`
+ *Severity*: warning
##### Alert Name: "KubeletPodStartUpLatencyHigh"
+ *Message*: `Kubelet Pod startup 99th percentile latency is {{ $value }} seconds on node {{ $labels.node }}.`
+ *Severity*: warning
##### Alert Name: "KubeletClientCertificateExpiration"
+ *Message*: `Client certificate for Kubelet on node {{ $labels.node }} expires in 7 days.`
+ *Severity*: warning
##### Alert Name: "KubeletClientCertificateExpiration"
+ *Message*: `Client certificate for Kubelet on node {{ $labels.node }} expires in 1 day.`
+ *Severity*: critical
##### Alert Name: "KubeletServerCertificateExpiration"
+ *Message*: `Server certificate for Kubelet on node {{ $labels.node }} expires in 7 days.`
+ *Severity*: warning
##### Alert Name: "KubeletServerCertificateExpiration"
+ *Message*: `Server certificate for Kubelet on node {{ $labels.node }} expires in 1 day.`
+ *Severity*: critical
##### Alert Name: "KubeletClientCertificateRenewalErrors"
+ *Message*: `Kubelet on node {{ $labels.node }} has failed to renew its client certificate ({{ $value | humanize }} errors in the last 15 minutes).`
+ *Severity*: warning
##### Alert Name: "KubeletServerCertificateRenewalErrors"
+ *Message*: `Kubelet on node {{ $labels.node }} has failed to renew its server certificate ({{ $value | humanize }} errors in the last 5 minutes).`
+ *Severity*: warning
##### Alert Name: "KubeVersionMismatch"
+ *Message*: `There are {{ $value }} different versions of Kubernetes components running.`
+ *Severity*: warning
##### Alert Name: "KubeClientErrors"
+ *Message*: `Kubernetes API server client '{{ $labels.job }}/{{ $labels.instance }}' is experiencing {{ $value | humanizePercentage }} errors.'`
+ *Severity*: warning
##### Alert Name: "KubeClientCertificateExpiration"
+ *Message*: `A client certificate used to authenticate to the apiserver is expiring in less than 7 days.`
+ *Severity*: warning
##### Alert Name: "KubeClientCertificateExpiration"
+ *Message*: `A client certificate used to authenticate to the apiserver is expiring in less than 1 day.`
+ *Severity*: critical
##### Alert Name: "KubeAPITerminatedRequests"
+ *Message*: `The apiserver has terminated {{ $value | humanizePercentage }} of its incoming requests.`
+ *Severity*: warning
+ *Action*: Use the `apiserver_flowcontrol_rejected_requests_total` metric to determine which flow schema is throttling the traffic to the API Server. The flow schema also provides information on the affected resources and subjects.

## Other Kubernetes Runbooks and troubleshooting
+ [Troubleshoot Clusters ](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-cluster/)
+ [Cloud.gov Kubernetes Runbook ](https://landing.app.cloud.gov/docs/ops/runbook/troubleshooting-kubernetes/)
+ [Recover a Broken Cluster](https://codefresh.io/Kubernetes-Tutorial/recover-broken-kubernetes-cluster/)
