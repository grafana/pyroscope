// Async to support override in Grafana's app plugin
export async function determineDefaultApp(apps: any[]) {
  const cpuApp = apps.find(
    (app) => app.__profile_type__.split(':')[1] === 'cpu'
  );

  if (cpuApp) {
    return cpuApp;
  }

  // `.itimer` type for Java
  const itimerApp = apps.find(
    (app) => app.__profile_type__.split(':')[1] === '.itimer'
  );

  if (itimerApp) {
    return itimerApp;
  }

  return apps[0];
}
