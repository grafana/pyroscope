import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Button, useStyles2 } from '@grafana/ui';

function App() {
  const styles = useStyles2(getStyles);

  return (
    <main>
      <div className={styles.body}>
        <Button>
          Click me
        </Button>
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
