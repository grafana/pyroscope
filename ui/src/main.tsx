import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { GlobalStyles, ThemeContext, useTheme2 } from '@grafana/ui';

import App from './App.tsx';

function ThemeProvider({ children }: { children: React.ReactNode }) {
  const newTheme = useTheme2();

  return (
    <ThemeContext.Provider value={newTheme}>
      <GlobalStyles />
      {children}
    </ThemeContext.Provider>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <App />
    </ThemeProvider>
  </StrictMode>,
);
