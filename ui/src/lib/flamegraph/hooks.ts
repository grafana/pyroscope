import { useState } from 'react';

import { type FlameGraphDataContainer } from './FlameGraph/dataTransform';
import { ColorScheme } from './types';

/**
 * Manages the color scheme state. Kept as a hook to mirror upstream's call
 * site even though the diff-aware reset logic has been removed.
 */
export function useColorScheme(_dataContainer: FlameGraphDataContainer | undefined) {
  return useState<ColorScheme>(ColorScheme.PackageBased);
}
