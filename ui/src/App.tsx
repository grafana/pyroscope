import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { useStyles2 } from '@grafana/ui';

import { SinglePage } from './pages/single/page';

function App() {
  const styles = useStyles2(getStyles);

  return (
    <main>
      <div className={styles.body}>
        <SinglePage />
      </div>
    </main>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  body: css`
    padding: ${theme.spacing(1)}
  `,
});

export default App;
