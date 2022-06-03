import React, { useEffect, useMemo, ChangeEvent, useRef } from 'react';
import { ProfileHeaderProps, useSizeMode } from './Toolbar';
import Input from '@webapp/ui/Input';
import styles from './SharedQueryInput.module.scss';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLink } from '@fortawesome/free-solid-svg-icons/faLink';

interface SharedQueryProps {
  showMode: ReturnType<typeof useSizeMode>;
  onHighlightChange: ProfileHeaderProps['handleSearchChange'];
  highlightQuery: ProfileHeaderProps['highlightQuery'];
  sharedQuery: ProfileHeaderProps['sharedQuery'];
}

const usePreviousSyncEnabled = (syncEnabled) => {
  const ref = useRef();

  useEffect(() => {
    ref.current = syncEnabled;
  });

  return ref.current;
};

const Tooltip = ({ syncEnabled }: { syncEnabled: string | boolean }) => (
  <div
    onClick={(e) => e.stopPropagation()}
    className={styles[syncEnabled ? 'tooltip-sync-enabled' : 'tooltip']}
  >
    {syncEnabled ? 'Unsync search bars' : 'Sync search bars'}
  </div>
);

const SharedQueryInput = ({
  onHighlightChange,
  showMode,
  highlightQuery,
  sharedQuery,
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
  }, [sharedQuery?.searchQuery, sharedQuery?.syncEnabled]);

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

  return (
    <div className={styles.wrapper}>
      <Input
        testId="flamegraph-search"
        className={`${styles[sharedQuery ? 'search-with-sync' : 'search']} ${
          showMode === 'small' ? styles['search-small'] : ''
        } ${styles[sharedQuery?.syncEnabled ? 'search-synced' : '']}`}
        type="search"
        name="flamegraph-search"
        placeholder="Search…"
        minLength={2}
        debounceTimeout={100}
        onChange={onQueryChange}
        value={inputValue}
      />
      {sharedQuery ? (
        <button
          className={styles[sharedQuery.syncEnabled ? 'sync-selected' : 'sync']}
          onClick={onToggleSync}
        >
          <FontAwesomeIcon
            className={`${
              !!sharedQuery.syncEnabled ? styles.checked : styles.icon
            }`}
            icon={faLink}
          />
          <Tooltip syncEnabled={sharedQuery.syncEnabled} />
        </button>
      ) : null}
    </div>
  );
};

export default SharedQueryInput;
