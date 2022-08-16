import React, { useMemo } from 'react';

import { Maybe } from '@webapp/util/fp';
import { AllProfiles } from '@webapp/models/adhoc';
import TableUI, { useTable, BodyRow } from '@webapp/ui/Table';
// eslint-disable-next-line css-modules/no-unused-class
import styles from './FileList.module.scss';
import CheckIcon from './CheckIcon';

const dateModifiedColName = 'updatedAt';
const fileNameColName = 'name';
const headRow = [
  { name: fileNameColName, label: 'Filename', sortable: 1 },
  { name: dateModifiedColName, label: 'Date Modified', sortable: 1 },
];

const getBodyRows = (
  sortedProfilesIds: AllProfiles[0][],
  onProfileSelected: (id: string) => void,
  selectedProfileId: Maybe<string>
): BodyRow[] => {
  return sortedProfilesIds.reduce((acc, profile) => {
    const isRowSelected = selectedProfileId.mapOr(
      false,
      (profId) => profId === profile.id
    );

    acc.push({
      cells: [
        {
          value: (
            <>
              {profile.name}
              {isRowSelected && <CheckIcon className={styles.checkIcon} />}
            </>
          ),
        },
        { value: profile.updatedAt },
      ],
      onClick: () => {
        // Optimize to not reload the same one
        if (
          selectedProfileId.isJust &&
          selectedProfileId.value === profile.id
        ) {
          return;
        }
        onProfileSelected(profile.id);
      },
      isRowSelected,
    });

    return acc;
  }, [] as BodyRow[]);
};

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

  const tableProps = useTable(headRow);
  const sortedProfilesIds = useMemo(() => {
    const m = tableProps.sortByDirection === 'asc' ? 1 : -1;

    let sorted: AllProfiles[number][] = [];

    if (profiles) {
      const filesInfo = Object.values(profiles);

      switch (tableProps.sortBy) {
        case fileNameColName:
          sorted = filesInfo.sort(
            // add types depend on passed props type
            // @ts-ignore
            (a, b) =>
              m * a[tableProps.sortBy].localeCompare(b[tableProps.sortBy])
          );
          break;
        case dateModifiedColName:
          sorted = filesInfo.sort(
            (a, b) =>
              m *
              // add types depend on passed props type
              // @ts-ignore
              (new Date(a[tableProps.sortBy]).getTime() -
                new Date(b[tableProps.sortBy]).getTime())
          );
          break;
        default:
          sorted = filesInfo;
      }
    }

    return sorted;
  }, [profiles, tableProps.sortBy, tableProps.sortByDirection]);

  const tableBodyProps = profiles
    ? {
        bodyRows: getBodyRows(
          sortedProfilesIds,
          onProfileSelected,
          selectedProfileId
        ),
      }
    : { error: { value: '' } };

  return (
    <>
      <div className={`${styles.tableContainer} ${className}`}>
        <TableUI
          {...tableProps}
          // fix types
          // @ts-ignore
          table={{ headRow, ...tableBodyProps }}
        />
      </div>
    </>
  );
}

export default FileList;
