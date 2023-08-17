import React, { useState } from 'react';
import type { ClickEvent } from '@pyroscope/ui/Menu';
import { formatRelative } from 'date-fns';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { faToggleOff } from '@fortawesome/free-solid-svg-icons/faToggleOff';
import { faToggleOn } from '@fortawesome/free-solid-svg-icons/faToggleOn';

import Button from '@pyroscope/ui/Button';
import Icon from '@pyroscope/ui/Icon';
import Dropdown, { MenuItem } from '@pyroscope/ui/Dropdown';
import {
  reloadUsers,
  changeUserRole,
} from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import confirmDelete from '@pyroscope/components/Modals/ConfirmDelete';
import type { User, Users } from '@pyroscope/models/users';
import type { BodyRow } from '@pyroscope/ui/Table';
import styles from './UserTableItem.module.css';

function DisableButton(props: { onDisable: (user: User) => void; user: User }) {
  const { user, onDisable } = props;
  const icon = user.isDisabled ? faToggleOff : faToggleOn;

  return (
    <Button type="button" kind="secondary" onClick={() => onDisable(user)}>
      <Icon icon={icon} />
    </Button>
  );
}

function EditRoleDropdown({ user }: { user: User }) {
  const { role } = user;
  const dispatch = useAppDispatch();
  const [status, setStatus] = useState(false);

  const handleEdit = (e: ClickEvent) => {
    if (e.value !== user.role) {
      dispatch(changeUserRole({ id: user.id, role: e.value }))
        .unwrap()
        .then(() => dispatch(reloadUsers()))
        .then(() => setStatus(true));
    }
  };

  return (
    <div className={styles.role}>
      <Dropdown label={`Role ${role}`} value={role} onItemClick={handleEdit}>
        <MenuItem value="Admin">Admin</MenuItem>
        <MenuItem value="ReadOnly">Readonly</MenuItem>
      </Dropdown>
      {status ? <Icon icon={faCheck} /> : null}
    </div>
  );
}

function DeleteButton(props: { onDelete: (user: User) => void; user: User }) {
  const { onDelete, user } = props;

  const handleDeleteClick = () => {
    confirmDelete({
      objectName: user.name,
      objectType: 'user',
      onConfirm: () => onDelete(user),
    });
  };

  return (
    <Button type="button" kind="danger" onClick={handleDeleteClick}>
      <Icon icon={faTimes} />
    </Button>
  );
}

export function getUserTableRows(
  currentUserId: number,
  displayUsers: Users,
  handleDisableUser: (user: User) => void,
  handleDeleteUser: (user: User) => void
): BodyRow[] {
  const bodyRows = displayUsers.reduce((acc, user) => {
    const { id, isDisabled, fullName, role, updatedAt, email, name } = user;
    const isCurrent = id === currentUserId;

    const row = {
      isRowDisabled: isDisabled,
      cells: [
        { value: id },
        { value: name },
        { value: email },
        { value: fullName },
        { value: isCurrent ? role : <EditRoleDropdown user={user} /> },
        { value: formatRelative(new Date(updatedAt), new Date()) },
        {
          value: !isCurrent ? (
            <div className={styles.actions}>
              <DisableButton user={user} onDisable={handleDisableUser} />
              <DeleteButton user={user} onDelete={handleDeleteUser} />
            </div>
          ) : null,
          align: 'center',
        },
      ],
    };

    acc.push(row);
    return acc;
  }, [] as BodyRow[]);

  return bodyRows;
}
