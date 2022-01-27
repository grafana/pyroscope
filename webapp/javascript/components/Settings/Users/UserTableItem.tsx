import React from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons';
import { format, formatRelative } from 'date-fns';
import { type User } from '../../../models/users';

function UserTableItem(props) {
  const { user } = props;
  const { id, fullName, passwordChangedAt, role, updatedAt, email, name } =
    user as User;

  return (
    <tr>
      <td>{id}</td>
      <td>{name}</td>
      <td>{email}</td>
      <td>{fullName}</td>
      <td>{role}</td>
      <td>{formatRelative(updatedAt, new Date())}</td>
      <td align="center">
        {' '}
        <Button type="submit" kind="default">
          <Icon icon={faTimes} />
        </Button>
      </td>
    </tr>
  );
}

export default UserTableItem;
