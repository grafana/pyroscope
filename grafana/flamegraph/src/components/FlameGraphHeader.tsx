import { css } from '@emotion/css';
import React from 'react';

import { Button, Input, useStyles } from '@grafana/ui';

type Props = {
  query: string;
  setTopLevelIndex: (level: number) => void;
  setRangeMin: (range: number) => void;
  setRangeMax: (range: number) => void;
  setQuery: (query: string) => void;
};

const FlameGraphHeader = ({ query, setTopLevelIndex, setRangeMin, setRangeMax, setQuery }: Props) => {
  const styles = useStyles(getStyles);

  return (
    <div className={styles.header}>
      <div className={styles.search}>
        <Input
          value={query || ''}
          onChange={(v) => {
            setQuery(v.currentTarget.value);
          }}
          placeholder={'Search..'}
          width={24}
        />
      </div>
      <div className={styles.reset}>
        <Button
          type={'button'}
          variant={'secondary'}
          size={'md'}
          onClick={() => {
            setTopLevelIndex(0);
            setRangeMin(0);
            setRangeMax(1);
            setQuery('');
          }}
        >
          Reset
        </Button>
      </div>
    </div>
  );
};

const getStyles = () => ({
  header: css`
    display: flow-root;
    padding: 20px 0;
    width: 100%;
  `,
  search: css`
    float: left;
  `,
  reset: css`
    float: right;
  `,
});

export default FlameGraphHeader;
