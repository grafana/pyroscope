/* eslint-disable no-unused-expressions */
import React, { useEffect, useMemo, ChangeEvent, useRef } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLink } from '@fortawesome/free-solid-svg-icons/faLink';
import Input from '@pyroscope/ui/Input';
import { Tooltip } from '@pyroscope/ui/Tooltip';
import styles from './SharedQueryInput.module.scss';
import type { ProfileHeaderProps } from './Toolbar';

interface SharedQueryProps {
  onHighlightChange: ProfileHeaderProps['handleSearchChange'];
  highlightQuery: ProfileHeaderProps['highlightQuery'];
  sharedQuery: ProfileHeaderProps['sharedQuery'];
  width: number;
}

const usePreviousSyncEnabled = (syncEnabled?: string | boolean) => {
  const ref = useRef();

  useEffect(() => {
    (ref.current as string | boolean | undefined) = syncEnabled;
  });

  return ref.current;
};

const SharedQueryInput = ({
  onHighlightChange,
  highlightQuery,
  sharedQuery,
  width,
}: SharedQueryProps) => {
  const prevSyncEnabled = usePreviousSyncEnabled(sharedQuery?.syncEnabled);

  const onQueryChange = (e: ChangeEvent<HTMLInputElement>) => {
    onHighlightChange(e.target.value);

    if (sharedQuery && sharedQuery.syncEnabled) {
      sharedQuery.onQueryChange(e.target.value);
    }
  };

  useEffect(() => {
    if (typeof sharedQuery?.searchQuery === 'string') {
      if (sharedQuery.syncEnabled) {
        onHighlightChange(sharedQuery.searchQuery);
      }

      if (
        !sharedQuery.syncEnabled &&
        prevSyncEnabled &&
        prevSyncEnabled !== sharedQuery?.id
      ) {
        onHighlightChange('');
      }
    }
  }, [
    sharedQuery?.searchQuery,
    sharedQuery?.syncEnabled,
    sharedQuery?.id,
    onHighlightChange,
    prevSyncEnabled,
  ]);

  const onToggleSync = () => {
    const newValue = sharedQuery?.syncEnabled ? false : sharedQuery?.id;
    sharedQuery?.toggleSync(newValue as string | false);

    if (newValue) {
      sharedQuery?.onQueryChange(highlightQuery);
    } else {
      onHighlightChange(highlightQuery);
      sharedQuery?.onQueryChange('');
    }
  };

  const inputValue = useMemo(
    () =>
      sharedQuery && sharedQuery.syncEnabled
        ? sharedQuery.searchQuery || ''
        : highlightQuery,
    [sharedQuery, highlightQuery]
  );

  const inputClassName = useMemo(
    () =>
      `${sharedQuery ? styles.searchWithSync : styles.search} ${
        sharedQuery?.syncEnabled ? styles['search-synced'] : ''
      }`,
    [sharedQuery]
  );

  return (
    <div className={styles.wrapper} style={{ width }}>
      <Input
        testId="flamegraph-search"
        className={inputClassName}
        type="search"
        name="flamegraph-search"
        placeholder="Search…"
        minLength={2}
        debounceTimeout={100}
        onChange={onQueryChange}
        value={inputValue}
      />
      {sharedQuery ? (
        <Tooltip
          placement="top"
          title={
            sharedQuery.syncEnabled ? 'Unsync search bars' : 'Sync search bars'
          }
        >
          <button
            className={
              sharedQuery.syncEnabled ? styles.syncSelected : styles.sync
            }
            onClick={onToggleSync}
          >
            <FontAwesomeIcon
              className={`${
                sharedQuery.syncEnabled ? styles.checked : styles.icon
              }`}
              icon={faLink}
            />
          </button>
        </Tooltip>
      ) : null}
    </div>
  );
};

export default SharedQueryInput;
