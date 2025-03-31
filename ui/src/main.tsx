import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import App from './App';
import ThemeProvider from './provider/ThemeProvider';
import QueryProvider from './provider/QueryProvider';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryProvider>
        <App />
      </QueryProvider>
    </ThemeProvider>
  </StrictMode>,
);
