{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='scheduling', url='', help='"Scheduling specifies the scheduling constraints for nodes supporting a RuntimeClass."'),
  '#withNodeSelector':: d.fn(help="\"nodeSelector lists labels that must be present on nodes that support this RuntimeClass. Pods using this RuntimeClass can only be scheduled to a node matched by this selector. The RuntimeClass nodeSelector is merged with a pod's existing nodeSelector. Any conflicts will cause the pod to be rejected in admission.\"", args=[d.arg(name='nodeSelector', type=d.T.object)]),
  withNodeSelector(nodeSelector): { nodeSelector: nodeSelector },
  '#withNodeSelectorMixin':: d.fn(help="\"nodeSelector lists labels that must be present on nodes that support this RuntimeClass. Pods using this RuntimeClass can only be scheduled to a node matched by this selector. The RuntimeClass nodeSelector is merged with a pod's existing nodeSelector. Any conflicts will cause the pod to be rejected in admission.\"\n\n**Note:** This function appends passed data to existing values", args=[d.arg(name='nodeSelector', type=d.T.object)]),
  withNodeSelectorMixin(nodeSelector): { nodeSelector+: nodeSelector },
  '#withTolerations':: d.fn(help='"tolerations are appended (excluding duplicates) to pods running with this RuntimeClass during admission, effectively unioning the set of nodes tolerated by the pod and the RuntimeClass."', args=[d.arg(name='tolerations', type=d.T.array)]),
  withTolerations(tolerations): { tolerations: if std.isArray(v=tolerations) then tolerations else [tolerations] },
  '#withTolerationsMixin':: d.fn(help='"tolerations are appended (excluding duplicates) to pods running with this RuntimeClass during admission, effectively unioning the set of nodes tolerated by the pod and the RuntimeClass."\n\n**Note:** This function appends passed data to existing values', args=[d.arg(name='tolerations', type=d.T.array)]),
  withTolerationsMixin(tolerations): { tolerations+: if std.isArray(v=tolerations) then tolerations else [tolerations] },
  '#mixin': 'ignore',
  mixin: self,
}
