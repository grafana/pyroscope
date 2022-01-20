import React from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons';

function UserTableItem(props) {
  const { user } = props;
  const { login, email, name, lastActive } = user;

  return (
    <tr>
      <td align="center">{/* avatar */}</td>
      <td>{login}</td>
      <td>{email}</td>
      <td>{name}</td>
      <td>{lastActive}</td>
      <td>{/* role */}</td>
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
