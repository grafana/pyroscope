import React, { useState } from 'react';
import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { faToggleOff } from '@fortawesome/free-solid-svg-icons/faToggleOff';
import { faToggleOn } from '@fortawesome/free-solid-svg-icons/faToggleOn';

import { formatRelative } from 'date-fns';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import { reloadUsers, changeUserRole } from '@webapp/redux/reducers/settings';
import { useAppDispatch } from '@webapp/redux/hooks';
import confirmDelete from '@webapp/components/Modals/ConfirmDelete';
import type { User, Users } from '@webapp/models/users';
import type { BodyRow } from '@webapp/ui/Table';
import styles from './UserTableItem.module.css';

function DisableButton(props: ShamefulAny) {
  const { user, onDisable } = props;
  const icon = user.isDisabled ? faToggleOff : faToggleOn;
  return (
    <Button type="button" kind="secondary" onClick={() => onDisable(user)}>
      <Icon icon={icon} onClick={onDisable} />
    </Button>
  );
}

// TODO: type this correctly
function EditRoleDropdown(props: ShamefulAny) {
  const { user } = props;
  const { role } = user;
  const dispatch = useAppDispatch();
  const [status, setStatus] = useState(false);

  const handleEdit = (evt: ShamefulAny) => {
    if (evt.value !== user.role) {
      dispatch(changeUserRole({ id: user.id, role: evt.value }))
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

function DeleteButton(props: ShamefulAny) {
  const { onDelete, user } = props;

  const handleDeleteClick = () => {
    confirmDelete('this user', () => {
      onDelete(user);
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
): { bodyRows: BodyRow[] } {
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
          // todo <td align="center">
          value: !isCurrent ? (
            <div className={styles.actions}>
              <DisableButton user={user} onDisable={handleDisableUser} />
              <DeleteButton user={user} onDelete={handleDeleteUser} />
            </div>
          ) : null,
        },
      ],
    };

    acc.push(row);
    return acc;
  }, [] as BodyRow[]);

  return { bodyRows };
}
