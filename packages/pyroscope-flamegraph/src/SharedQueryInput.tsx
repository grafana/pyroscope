import React, { useEffect, useMemo, useRef } from 'react';
import { ProfileHeaderProps, useSizeMode } from './Toolbar';
import Input from '@webapp/ui/Input';
import usePreviousProps from '@webapp/hooks/previousProps.hook';
import styles from './SharedQueryInput.module.scss';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLink } from '@fortawesome/free-solid-svg-icons/faLink';

interface SharedQueryProps {
  showMode: ReturnType<typeof useSizeMode>;
  onHighlightChange: ProfileHeaderProps['handleSearchChange'];
  highlightQuery: ProfileHeaderProps['highlightQuery'];
  sharedQuery: ProfileHeaderProps['sharedQuery'];
}

const SharedQueryInput = ({
  onHighlightChange,
  showMode,
  highlightQuery,
  sharedQuery,
}: SharedQueryProps) => {
  const prevProps = usePreviousProps(sharedQuery);

  const onQueryChange = (e) => {
    onHighlightChange(e.target.value);

    if (sharedQuery && sharedQuery.syncEnabled) {
      sharedQuery.onQueryChange(e.target.value);
    }
  };

  useEffect(() => {
    if (typeof sharedQuery?.query === 'string') {
      if (sharedQuery.syncEnabled) {
        onHighlightChange(sharedQuery.query);
      }

      if (
        !sharedQuery.syncEnabled &&
        prevProps?.syncEnabled &&
        prevProps?.syncEnabled !== sharedQuery?.id
      ) {
        onHighlightChange('');
      }
    }
  }, [sharedQuery?.query, sharedQuery?.syncEnabled]);

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
        ? sharedQuery.query || ''
        : highlightQuery,
    [sharedQuery, highlightQuery]
  );

  return (
    <div className={styles.wrapper}>
      <Input
        testId="flamegraph-search"
        className={`${
          styles[sharedQuery?.syncEnabled ? 'search-synced' : 'search']
        } ${showMode === 'small' ? styles['search-small'] : ''}`}
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
        </button>
      ) : null}
    </div>
  );
};

export default SharedQueryInput;
