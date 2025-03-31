import { ThemeContext } from '@grafana/data';
import { useTheme2, GlobalStyles } from '@grafana/ui';

export default function ThemeProvider({ children }: { children: React.ReactNode }) {
  const newTheme = useTheme2();

  return (
    <ThemeContext.Provider value={newTheme}>
      <GlobalStyles />
      {children}
    </ThemeContext.Provider>
  );
}
