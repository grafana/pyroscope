import React, { useState, useEffect, useMemo } from 'react';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';

import { queryFromAppName, queryToAppName, Query } from '@webapp/models/query';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  actions,
  selectAppNames,
  reloadAppNames,
  selectQueries,
  selectAppNamesState,
} from '@webapp/redux/reducers/continuous';
import Button from '@webapp/ui/Button';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import Input from '@webapp/ui/Input';
import SelectButton from './SelectButton';
import styles from './AppSelector.module.scss';

interface AppSelectorProps {
  // Comparison/Diff View pages provide {onSelectedName} func which
  // handle propagating query to left/right flamegraphs
  onSelectedName?: (name: Query) => void;

  filterApp?: (names: string) => boolean;
}

const AppSelector = ({
  onSelectedName,
  filterApp = () => true,
}: AppSelectorProps) => {
  const dispatch = useAppDispatch();
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames).filter(filterApp);
  const { query } = useAppSelector(selectQueries);
  const appName = queryToAppName(query).mapOr('', (q) =>
    appNames.indexOf(q) !== -1 ? q : ''
  );

  const selectAppName = (name: string) => {
    const appNameQuery = queryFromAppName(name);
    if (onSelectedName) {
      onSelectedName(appNameQuery);
    } else {
      dispatch(actions.setQuery(appNameQuery));
    }
  };

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <SelectorModalWithToggler
        selectAppName={selectAppName}
        appNames={appNames}
        appName={appName}
      />
      <Button
        aria-label="Refresh Apps"
        icon={faSyncAlt}
        onClick={() => dispatch(reloadAppNames())}
        className={styles.refreshButton}
      />
      {appNamesState.type === 'reloading' && <LoadingSpinner />}
    </div>
  );
};

export default AppSelector;

const DELIMITER = '.';
const isGroupMember = (groupName: string, name: string) =>
  name.indexOf(groupName) === 0 &&
  (name[groupName.length] === DELIMITER || name.length === groupName.length);

const getGroupMembers = (names: string[], name: string) =>
  names.filter((a) => isGroupMember(name, a));

const getGroupNameFromAppName = (groups: string[], fullName: string) =>
  groups.filter((g) => isGroupMember(g, fullName))[0] || '';

const getGroups = (filteredAppNames: string[]) => {
  const allGroups = filteredAppNames.map((i) => {
    const arr = i.split(DELIMITER);
    const cutProfileType = arr.length > 1 ? arr.slice(0, -1) : arr;
    return cutProfileType.join(DELIMITER);
  });

  const uniqGroups = Array.from(new Set(allGroups));

  const dedupedUniqGroups = uniqGroups.filter((x) => {
    return !uniqGroups.find((y) => x !== y && isGroupMember(y, x));
  });

  const groupOrApp = dedupedUniqGroups.map((u) => {
    const appNamesEntries = getGroupMembers(filteredAppNames, u);

    return appNamesEntries.length > 1 ? u : appNamesEntries?.[0];
  });

  return groupOrApp;
};

interface SelectorModalWithTogglerProps {
  appNames: string[];
  selectAppName: (name: string) => void;
  appName: string;
}

const SelectorModalWithToggler = ({
  appNames,
  selectAppName,
  appName,
}: SelectorModalWithTogglerProps) => {
  const [filter, setFilter] = useState('');
  const [isModalOpen, setModalOpenStatus] = useState(false);

  // selected is an array of strings
  //  0 corresponds to string of group / app name selected in the left pane
  //  1 corresponds to string of app name selected in the right pane
  const [selected, setSelected] = useState<string[]>([]);
  const filteredAppNames = useMemo(
    // filtered names by search input
    () =>
      appNames.filter((n: string) =>
        n.toLowerCase().includes(filter.trim().toLowerCase())
      ),
    [filter, appNames]
  );

  const groups = useMemo(() => getGroups(filteredAppNames), [filteredAppNames]);

  const profileTypes = useMemo(() => {
    if (!selected?.[0]) {
      return [];
    }

    const filtered = getGroupMembers(filteredAppNames, selected?.[0]);

    if (filtered.length > 1) {
      return filtered;
    }

    return [];
  }, [selected, groups, filteredAppNames]);

  const onSelect = ({ index, name }: { index: number; name: string }) => {
    const filtered = getGroupMembers(filteredAppNames, name);

    if (filtered.length === 1 || index === 1) {
      selectAppName(filtered?.[0]);
      setModalOpenStatus(false);
    }

    const arr = Array.from(selected);

    if (index === 0 && arr?.length > 1) {
      arr.pop();
    }

    arr[index] = name;

    setSelected(arr);
  };

  useEffect(() => {
    if (appName && !selected.length && groups.length) {
      if (groups.indexOf(appName) !== -1) {
        setSelected([appName]);
        setModalOpenStatus(false);
      } else {
        setSelected([getGroupNameFromAppName(groups, appName), appName]);
      }
    }
  }, [appName, selected, groups]);

  const listHeight = useMemo(() => {
    const height = (window?.innerHeight || 0) - 160;

    const listRequiredHeight =
      // 35 is list item height
      Math.max(groups?.length || 0, profileTypes?.length || 0) * 35;

    if (height && listRequiredHeight) {
      return height >= listRequiredHeight ? 'auto' : `${height}px`;
    }

    return 'auto';
  }, [groups, profileTypes]);

  return (
    <ModalWithToggle
      isModalOpen={isModalOpen}
      setModalOpenStatus={setModalOpenStatus}
      modalClassName={styles.appSelectorModal}
      modalHeight={listHeight}
      noDataEl={
        !filteredAppNames?.length ? (
          <div data-testid="app-selector-no-data" className={styles.noData}>
            No Data
          </div>
        ) : null
      }
      toggleText={appName || 'Select application'}
      headerEl={
        <>
          <div className={styles.headerTitle}>SELECT APPLICATION</div>
          <Input
            name="application seach"
            type="text"
            placeholder="Type an app"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className={styles.search}
            testId="application-search"
          />
        </>
      }
      leftSideEl={groups.map((name) => (
        <SelectButton
          name={name}
          onClick={() => onSelect({ index: 0, name })}
          fullList={appNames}
          isSelected={selected?.[0] === name}
          key={name}
        />
      ))}
      rightSideEl={profileTypes.map((name) => (
        <SelectButton
          name={name}
          onClick={() => onSelect({ index: 1, name })}
          fullList={appNames}
          isSelected={selected?.[1] === name}
          key={name}
        />
      ))}
    />
  );
};
