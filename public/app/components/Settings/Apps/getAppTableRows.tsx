import React from 'react';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';

import Button from '@pyroscope/ui/Button';
import Icon from '@pyroscope/ui/Icon';
import { App } from '@pyroscope/models/app';
import type { BodyRow } from '@pyroscope/ui/Table';
import confirmDelete from '@pyroscope/components/Modals/ConfirmDelete';
import LoadingSpinner from '@pyroscope/ui/LoadingSpinner';

import styles from './AppTableItem.module.css';

interface DeleteButtorProps {
  onDelete: (app: App) => void;
  isLoading: boolean;
  app: App;
}

function DeleteButton(props: DeleteButtorProps) {
  const { onDelete, app, isLoading } = props;

  const handleDeleteClick = () => {
    confirmDelete({
      objectName: app.name,
      objectType: 'app',
      withConfirmationInput: true,
      warningMsg: `Note: This action can take up to ~15 minutes depending on the size of your application and wont' be reflected in the UI until it is complete.`,
      onConfirm: () => onDelete(app),
    });
  };

  return isLoading ? (
    <LoadingSpinner className={styles.loadingIcon} />
  ) : (
    <Button type="button" kind="danger" onClick={handleDeleteClick}>
      <Icon icon={faTimes} />
    </Button>
  );
}

export function getAppTableRows(
  displayApps: App[],
  appsInProcessing: string[],
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
              <DeleteButton
                app={app}
                onDelete={handleDeleteApp}
                isLoading={appsInProcessing.includes(name)}
              />
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
