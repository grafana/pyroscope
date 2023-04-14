import { loadAppNames } from '../hooks/loadAppNames';

// LoadAppNames loads all app names automatically
export function LoadAppNames(props: { children?: React.ReactNode }) {
  loadAppNames();

  return <>{props.children}</>;
}
