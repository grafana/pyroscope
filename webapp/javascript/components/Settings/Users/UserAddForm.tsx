import React, { useState } from 'react';
import Button from '@webapp/ui/Button';
import InputField from '@webapp/ui/InputField';
import { useHistory } from 'react-router-dom';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { createUser } from '@webapp/redux/reducers/settings';
import { useAppDispatch } from '@webapp/redux/hooks';
import { addNotification } from '@webapp/redux/reducers/notifications';
import { passwordEncode, type User } from '@webapp/models/users';

export type UserAddProps = User & { password?: string };

function UserAddForm() {
  const dispatch = useAppDispatch();
  const history = useHistory();

  const handleFormSubmit = (e: ShamefulAny) => {
    e.preventDefault();
    const formData = e.target as typeof e.target & {
      name: { value: string };
      password: { value: string };
      email: { value: string };
      fullName: { value: string };
    };

    const data = {
      name: formData.name.value,
      email: formData.email.value,
      fullName: formData.fullName.value,
      role: 'ReadOnly',
      password: passwordEncode(formData.password.value),
    };

    if (!data.password) {
      return;
    }

    dispatch(createUser(data as ShamefulAny as User))
      .unwrap()
      .then(() => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'User added',
            message: `User has been successfully added`,
          })
        );
        history.push('/settings/users');
      });
  };

  return (
    <>
      <h2>Add User</h2>
      <form onSubmit={handleFormSubmit}>
        <InputField label="Name" id="userAddName" name="name" />
        <InputField label="Email" id="userAddEmail" name="email" />
        <InputField label="Full name" id="userAddFullName" name="fullName" />
        <InputField
          label="Password"
          id="userAddPassword"
          name="password"
          type="password"
        />
        <div>
          <Button
            icon={faCheck}
            type="submit"
            data-testid="settings-useradd"
            kind="secondary"
          >
            Add user
          </Button>
        </div>
      </form>
    </>
  );
}

export default UserAddForm;
