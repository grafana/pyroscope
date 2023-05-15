import React from 'react';
import { useAppNames } from '@phlare/hooks/useAppNames';

// LoadAppNames loads all app names automatically
export function LoadAppNames(props: { children?: React.ReactNode }) {
  useAppNames();

  return <>{props.children}</>;
}
