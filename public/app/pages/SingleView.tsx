import ContinuousSingleView from '@webapp/pages/ContinuousSingleView';
import { loadAppNames } from '../hooks/loadAppNames';

export function SingleView() {
  loadAppNames();

  return <ContinuousSingleView />;
}
