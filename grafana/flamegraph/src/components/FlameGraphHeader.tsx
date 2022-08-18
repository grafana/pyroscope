import { css } from '@emotion/css';
import React from 'react';

import { Button, useStyles } from '@grafana/ui';

type Props = {
  setTopLevelIndex: (level: number) => void;
  setRangeMin: (range: number) => void;
  setRangeMax: (range: number) => void;
};

const FlameGraphHeader = ({setTopLevelIndex, setRangeMin, setRangeMax}: Props) => {
  const styles = useStyles(getStyles);
  
  return (
    <div className={styles.header}>
      <div className={styles.reset}>
        <Button
          type={'button'}
          variant={'secondary'}
          size={'md'}
          onClick={() => {
            setTopLevelIndex(0);
            setRangeMin(0);
            setRangeMax(1);
          }}
        >
          Reset
        </Button>
      </div>
    </div>
  )
}

const getStyles = () => ({
  header: css`
    display: flow-root;
    padding: 20px 0;
    width: 100%;
  `,
  reset: css`
    float: right;
  `,
});

export default FlameGraphHeader;
