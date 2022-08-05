import React, { useState, useMemo } from 'react';
import { Maybe } from '@webapp/util/fp';
import { AllProfiles } from '@webapp/models/adhoc';
import clsx from 'clsx';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './FileList.module.scss';
import CheckIcon from './CheckIcon';

const dateModifiedColName = 'updatedAt';
const fileNameColName = 'name';
const tableFormat = [
  { name: fileNameColName, label: 'Filename' },
  { name: dateModifiedColName, label: 'Date Modified' },
];

interface FileListProps {
  className?: string;
  profilesList: AllProfiles;
  onProfileSelected: (id: string) => void;
  selectedProfileId: Maybe<string>;
}

function FileList(props: FileListProps) {
  const {
    profilesList: profiles,
    onProfileSelected,
    className,
    selectedProfileId,
  } = props;

  const [sortBy, updateSortBy] = useState(dateModifiedColName);
  const [sortByDirection, setSortByDirection] = useState<'desc' | 'asc'>(
    'desc'
  );

  const isRowSelected = (id: string) => {
    return selectedProfileId.mapOr(false, (profId) => profId === id);
  };

  const updateSortParams = (newSortBy: typeof tableFormat[number]['name']) => {
    let dir = sortByDirection;

    if (sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'asc';
    }

    updateSortBy(newSortBy);
    setSortByDirection(dir);
  };

  const sortedProfilesIds = useMemo(() => {
    const m = sortByDirection === 'asc' ? 1 : -1;

    let sorted: AllProfiles[number][] = [];

    if (profiles) {
      const filesInfo = Object.values(profiles);

      switch (sortBy) {
        case fileNameColName:
          sorted = filesInfo.sort(
            (a, b) => m * a[sortBy].localeCompare(b[sortBy])
          );
          break;
        case dateModifiedColName:
          sorted = filesInfo.sort(
            (a, b) =>
              m *
              (new Date(a[sortBy]).getTime() - new Date(b[sortBy]).getTime())
          );
          break;
        default:
          sorted = filesInfo;
      }
    }

    return sorted;
  }, [profiles, sortBy, sortByDirection]);

  return (
    <>
      <div className={`${styles.tableContainer} ${className}`}>
        <table className={styles.profilesTable} data-testid="table-view">
          <thead>
            <tr>
              {tableFormat.map(({ name, label }) => (
                <th
                  key={name}
                  className={styles.sortable}
                  onClick={() => updateSortParams(name)}
                >
                  {label}
                  <span
                    className={clsx(
                      styles.sortArrow,
                      sortBy === name && styles[sortByDirection]
                    )}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {profiles &&
              sortedProfilesIds.map((profile) => (
                <tr
                  key={profile.id}
                  onClick={() => {
                    // Optimize to not reload the same one
                    if (
                      selectedProfileId.isJust &&
                      selectedProfileId.value === profile.id
                    ) {
                      return;
                    }
                    onProfileSelected(profile.id);
                  }}
                  className={`${
                    isRowSelected(profile.id) && styles.rowSelected
                  }`}
                >
                  <td>
                    {profile.name}

                    {isRowSelected(profile.id) && (
                      <CheckIcon className={styles.checkIcon} />
                    )}
                  </td>
                  <td>{profile.updatedAt}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

export default FileList;
