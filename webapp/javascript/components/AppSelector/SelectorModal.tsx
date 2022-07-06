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

const getAppNames = (names: string[], name: string) =>
  names.filter((a) => a.indexOf(name) !== -1);

const getGroups = (filteredAppNames: string[]) => {
  const allGroups = filteredAppNames.map((i) => {
    const splitted = i.split(DELIMITER);
    const cutProfileType =
      splitted.length > 1 ? splitted.slice(0, -1) : splitted;
    const backToStr = cutProfileType.join(DELIMITER);

    return backToStr;
  });

  const uniqGroups = Array.from(new Set(allGroups));

  const groupOrProfileType = uniqGroups.map((u) => {
    const appNamesEntries = getAppNames(filteredAppNames, u);

    return appNamesEntries.length > 1 ? u : appNamesEntries?.[0];
  });

  return groupOrProfileType;
};

const SelectorModal = ({
  visible,
  appNames,
  selectAppName,
  appName,
}: SelectorModalProps) => {
  const [filter, setFilter] = useState('');
  const [selected, select] = useState<string[]>([]);
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

    const filtered = getAppNames(filteredAppNames, selected?.[0]);

    if (filtered.length > 1) {
      return filtered;
    }

    return [];
  }, [selected, groups, filteredAppNames]);

  const onSelect = ({ index, name }: { index: number; name: string }) => {
    const filtered = getAppNames(filteredAppNames, name);

    if (filtered.length === 1) {
      selectAppName(filtered?.[0]);
    }

    const arr = Array.from(selected);

    if (index === 0 && arr?.length > 1) {
      arr.pop();
    }

    arr[index] = name;

    select(arr);
  };

  useEffect(() => {
    if (appName && !selected.length && groups.length) {
      if (groups.indexOf(appName) !== -1) {
        select([appName]);
      } else {
        select([
          appName.split(DELIMITER).slice(0, -1).join(DELIMITER),
          appName,
        ]);
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
