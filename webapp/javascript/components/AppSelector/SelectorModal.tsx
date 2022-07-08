import React, { useEffect, useMemo, useState } from 'react';
import classNames from 'classnames/bind';
import Input from '@webapp/ui/Input';
import SelectButton from './SelectButton';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './SelectorModal.module.scss';

const cx = classNames.bind(styles);

interface SelectorModalProps {
  visible: boolean;
  appNames: string[];
  selectAppName: (name: string) => void;
  appName: string;
}

const DELIMITER = '.';
const APPS_LIST_ELEMENT_ID = 'apps_list';
export const APP_SEARCH_INPUT = 'application_search';

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

const SelectorModal = ({
  visible,
  appNames,
  selectAppName,
  appName,
}: SelectorModalProps) => {
  const [filter, setFilter] = useState('');

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

    if (visible && height && listRequiredHeight) {
      return height >= listRequiredHeight ? 'auto' : `${height}px`;
    }

    return 'auto';
  }, [groups, profileTypes, visible]);

  return (
    <div className={cx({ modalBody: true, visible })}>
      <div className={styles.selectorHeader}>
        <div>
          <div className={styles.sectionTitle}>SELECT APPLICATION</div>
          <Input
            name="application seach"
            type="text"
            placeholder="Type an app"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className={styles.search}
            testId={APP_SEARCH_INPUT}
          />
        </div>
      </div>
      {filteredAppNames?.length ? (
        <div
          style={{ height: listHeight }}
          id={APPS_LIST_ELEMENT_ID}
          className={styles.apps}
        >
          <div className={styles.section}>
            {groups.map((name) => (
              <SelectButton
                name={name}
                onClick={() => onSelect({ index: 0, name })}
                fullList={appNames}
                isSelected={selected?.[0] === name}
                key={name}
              />
            ))}
          </div>
          <div className={styles.section}>
            {profileTypes.map((name) => (
              <SelectButton
                name={name}
                onClick={() => onSelect({ index: 1, name })}
                fullList={appNames}
                isSelected={selected?.[1] === name}
                key={name}
              />
            ))}
          </div>
        </div>
      ) : (
        <div className={styles.noData}>No Data</div>
      )}
    </div>
  );
};

export default SelectorModal;
