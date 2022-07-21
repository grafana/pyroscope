{
  local d = (import 'doc-util/main.libsonnet'),
  '#':: d.pkg(name='windowsSecurityContextOptions', url='', help='"WindowsSecurityContextOptions contain Windows-specific options and credentials."'),
  '#withGmsaCredentialSpec':: d.fn(help='"GMSACredentialSpec is where the GMSA admission webhook (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the GMSA credential spec named by the GMSACredentialSpecName field."', args=[d.arg(name='gmsaCredentialSpec', type=d.T.string)]),
  withGmsaCredentialSpec(gmsaCredentialSpec): { gmsaCredentialSpec: gmsaCredentialSpec },
  '#withGmsaCredentialSpecName':: d.fn(help='"GMSACredentialSpecName is the name of the GMSA credential spec to use."', args=[d.arg(name='gmsaCredentialSpecName', type=d.T.string)]),
  withGmsaCredentialSpecName(gmsaCredentialSpecName): { gmsaCredentialSpecName: gmsaCredentialSpecName },
  '#withRunAsUserName':: d.fn(help='"The UserName in Windows to run the entrypoint of the container process. Defaults to the user specified in image metadata if unspecified. May also be set in PodSecurityContext. If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence."', args=[d.arg(name='runAsUserName', type=d.T.string)]),
  withRunAsUserName(runAsUserName): { runAsUserName: runAsUserName },
  '#mixin': 'ignore',
  mixin: self,
}
