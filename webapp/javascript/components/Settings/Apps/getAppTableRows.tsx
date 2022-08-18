import React from 'react';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';

import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';
import { App, Apps } from '@webapp/models/app';
import type { BodyRow } from '@webapp/ui/Table';

import confirmDelete from '@webapp/components/Modals/ConfirmDelete';
import styles from './AppTableItem.module.css';

function DeleteButton(props: { onDelete: (app: App) => void; app: App }) {
  const { onDelete, app } = props;

  const handleDeleteClick = () => {
    confirmDelete('this app', () => {
      onDelete(app);
    });
  };

  return (
    <Button type="button" kind="danger" onClick={handleDeleteClick}>
      <Icon icon={faTimes} />
    </Button>
  );
}

export function getAppTableRows(
  displayApps: Apps,
  handleDeleteApp: (app: App) => void
): BodyRow[] {
  const bodyRows = displayApps.reduce((acc, app) => {
    const { name } = app;

    const row = {
      cells: [
        { value: name },
        {
          value: (
            <div className={styles.actions}>
              <DeleteButton app={app} onDelete={handleDeleteApp} />
            </div>
          ),
          align: 'center',
        },
      ],
    };

    acc.push(row);
    return acc;
  }, [] as BodyRow[]);

  return bodyRows;
}
