import ContinuousComparisonView from '@webapp/pages/ContinuousComparisonView';
import { loadAppNames } from '../hooks/loadAppNames';

export function ComparisonView() {
  loadAppNames();

  return <ContinuousComparisonView />;
}
